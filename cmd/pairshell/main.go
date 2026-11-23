package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"pairshell/internal/audit"
	"pairshell/internal/config"
	"pairshell/internal/executor"
	"pairshell/internal/localui"
	"pairshell/internal/mcp"
	"pairshell/internal/paircode"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/securefile"
	"pairshell/internal/session"
	"pairshell/internal/sshbridge"
	"pairshell/internal/terminal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "PairShell:", e)
		os.Exit(1)
	}
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func run() error {
	if len(os.Args) < 2 {
		if os.Getenv("PAIRSHELL_DESKTOP_EMBEDDED") == "1" {
			return desktop()
		}
		return errors.New("用法: pairshell merchant|customer|mcp|verify|replay")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	f := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	server := f.String("server", "", "协调服务 HTTPS URL")
	ref := f.String("session", "", "受保护本地会话引用")
	root := f.String("root", ".", "远程会话的初始工作目录；文件访问使用当前账号的系统权限")
	logs := f.String("logs", "./data", "审计目录")
	command := f.String("command", "", "连接后运行 SSH exec")
	headless := f.Bool("headless", false, "连接后只开放 CLI/MCP 适配器，不占用交互终端")
	allowAI := f.Bool("allow-ai", false, "客户明确允许本次 Agent 任务；与交互终端占用互斥")
	if e := f.Parse(os.Args[2:]); e != nil {
		return e
	}
	switch os.Args[1] {
	case "version", "--version":
		fmt.Fprintln(os.Stdout, "PairShell 1.1.2")
		return nil
	case "config":
		if *server != "" {
			return config.UpdateServerDefault(*server)
		}
		cfg, e := config.LoadDefault()
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(cfg)
	case "shell":
		if *ref == "" {
			return errors.New("--session required")
		}
		c, e := localSSHClient(ctx, *ref)
		if e != nil {
			return e
		}
		defer c.Close()
		return interactiveShell(ctx, c)
	case "host":
		base, e := config.ResolveServer(*server)
		if e != nil {
			return e
		}
		code, result, e := paircode.Listen(ctx, base, nil)
		if e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, "本次配对码：", code, "\n将它发送给协助人员；连接后仅需授权一次。")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r := <-result:
			if r.Err != nil {
				return r.Err
			}
			return customerWithConsent(ctx, r.Invite, *root, *logs, nil, func(g *session.Gate, verification string) error {
				fmt.Fprintln(os.Stderr, "通过核对后，你将允许本次终端和命令任务在当前系统账号权限下运行。")
				if e := verifyPeerFromInput(ctx, bufio.NewReader(os.Stdin), os.Stderr, verification); e != nil {
					return e
				}
				return g.Consent(session.Capabilities{Terminal: true, Tasks: true}, *allowAI)
			})
		}
	case "connect":
		base, e := config.ResolveServer(*server)
		if e != nil {
			return e
		}
		code := f.Arg(0)
		input := bufio.NewReaderSize(os.Stdin, 128)
		if code == "" {
			fmt.Fprintln(os.Stderr, "输入对方提供的配对码：")
			line, err := input.ReadSlice('\n')
			code, e = string(line), err
			if e != nil && e != io.EOF {
				return e
			}
		}
		return connectByCode(ctx, base, strings.TrimSpace(code), *ref, *headless, input)
	case "verify":
		if f.NArg() != 1 {
			return errors.New("verify <session-directory>")
		}
		v, e := audit.Verify(f.Arg(0))
		if e != nil {
			return e
		}
		json.NewEncoder(os.Stdout).Encode(v)
		if !v.Complete {
			return errors.New("日志未正常收尾")
		}
		return nil
	case "replay":
		if f.NArg() != 1 {
			return errors.New("replay <session-directory>")
		}
		return audit.Replay(f.Arg(0), safeOutput{os.Stdout})
	case "mcp":
		if *ref == "" {
			return errors.New("--session required")
		}
		return mcp.Run(ctx, *ref)
	case "proxy":
		return proxy(ctx, *ref)
	case "customer":
		fmt.Fprintln(os.Stderr, "粘贴邀请链接后按 Enter（不会写入普通日志）：")
		raw, e := bufio.NewReader(io.LimitReader(os.Stdin, 4097)).ReadString('\n')
		if e != nil && e != io.EOF {
			return e
		}
		return customer(ctx, strings.TrimSpace(raw), *root, *logs)
	case "merchant":
		base, e := config.ResolveServer(*server)
		if e != nil {
			return e
		}
		return merchant(ctx, base, *ref, *command)
	default:
		return errors.New("unknown command")
	}
}
func customer(ctx context.Context, invite, root, logs string) error {
	return customerWithUI(ctx, invite, root, logs, nil)
}
func customerWithUI(ctx context.Context, invite, root, logs string, ready func(string, string)) error {
	return customerWithConsent(ctx, invite, root, logs, ready, nil)
}
func customerWithConsent(ctx context.Context, invite, root, logs string, ready func(string, string), consent func(*session.Gate, string) error) error {
	return customerSession(ctx, invite, root, logs, ready, consent, "")
}
func customerWithUIWithReturn(ctx context.Context, invite, root, logs, returnURL string, ready func(string, string)) error {
	return customerSession(ctx, invite, root, logs, ready, nil, returnURL)
}
func customerSession(ctx context.Context, invite, root, logs string, ready func(string, string), consent func(*session.Gate, string) error, returnURL string) error {
	idleTimeout := session.DefaultIdleTimeout
	if value, configured := os.LookupEnv("PAIRSHELL_IDLE_TIMEOUT"); configured {
		var err error
		idleTimeout, err = time.ParseDuration(value)
		if err != nil || idleTimeout <= 0 {
			return errors.New("PAIRSHELL_IDLE_TIMEOUT 必须是正数时长，例如 10m")
		}
	}
	var gate *session.Gate
	var manager *executor.Manager
	var writer *audit.Writer
	var uiServer *http.Server
	handler := &customerHandler{}
	defer func() {
		if gate != nil {
			gate.End()
		}
		if manager != nil {
			manager.Close()
			waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if e := manager.Wait(waitCtx); e != nil {
				fmt.Fprintln(os.Stderr, "CLEANUP_INCOMPLETE:", e)
			}
		}
		if uiServer != nil {
			if returnURL != "" {
				// Revoke execution immediately above, but retain the authenticated
				// local viewer briefly so it can drain final output and show ENDED.
				time.AfterFunc(2*time.Minute, func() { uiServer.Close() })
			} else {
				uiServer.Close()
			}
		}
		if writer != nil {
			if e := writer.Close("session closed"); e != nil {
				fmt.Fprintln(os.Stderr, "AUDIT_CLOSE_FAILED:", e)
			}
		}
	}()
	return remote.ServeCustomer(ctx, invite, nil, handler, func(tr pairing.Transcript, code string) (*session.Gate, error) {
		gate = session.NewWithIdleTimeout(tr.SessionID, base64.RawURLEncoding.EncodeToString(tr.MerchantKey), time.Now().Add(2*time.Hour), idleTimeout)
		var e error
		dir := filepath.Join(logs, "session-"+tr.SessionID)
		writer, e = audit.NewWriter(dir, tr.SessionID)
		if e != nil {
			return nil, e
		}
		var auditFailed atomic.Bool
		var failOnce sync.Once
		appendEvent := func(actor, kind, id string, epoch uint64, p any) error {
			b, e := json.Marshal(p)
			if e != nil {
				return e
			}
			e = writer.Append(audit.Event{Actor: actor, EventType: kind, RequestID: id, AuthorizationEpoch: epoch, Payload: b})
			if e != nil {
				auditFailed.Store(true)
				failOnce.Do(func() {
					go func() {
						gate.AuditFailure()
						if manager != nil {
							manager.Close()
						}
					}()
				})
			}
			return e
		}
		record := func(kind, id string, p any) error { return appendEvent("merchant", kind, id, 1, p) }
		if e = appendEvent("customer", "pairing.verified", "", 1, tr); e != nil {
			return nil, e
		}
		// Audit failure is reported outside Gate's admission lock to avoid lock inversion.
		authorize := func(epoch uint64, cap string, ai bool, fn func() error) error {
			if auditFailed.Load() {
				return errors.New("AUDIT_IO_FAILED")
			}
			e := gate.WithAuthorization(epoch, cap, ai, fn)
			return e
		}
		manager = executor.NewSystem(root, authorize, record)
		manager.SetStagedAuthorization(gate.StagedAuthorization)
		shell := "sh"
		ptyShell := terminal.DefaultShell()
		if runtime.GOOS == "windows" {
			shell = "powershell"
			ptyShell = "powershell.exe -NoProfile -NoLogo"
		}
		rpcToken := token()
		bridge := localui.Bridge{Token: rpcToken, Gate: gate, Tasks: manager}
		handler.h = executor.SSHHandler{Manager: manager, Epoch: 1, ShellName: shell, Terminal: func(in io.Reader, out io.Writer, w, h uint32, r <-chan sshbridge.Size) (int, error) {
			return manager.Terminal(1, ptyShell, in, out, w, h, r)
		}, RPCHandler: func(ch io.ReadWriteCloser) error {
			defer ch.Close()
			req, e := http.ReadRequest(bufio.NewReader(io.LimitReader(ch, 1600000)))
			if e != nil {
				return e
			}
			req.Header.Set("Authorization", "Bearer "+rpcToken)
			rec := httptest.NewRecorder()
			bridge.ServeHTTP(rec, req)
			return rec.Result().Write(ch)
		}}
		ui := localui.New(gate, manager, func(action string, snapshot session.Snapshot, fileRoot string) error {
			return appendEvent("customer", "control."+action, "", snapshot.AuthorizationEpoch, struct {
				session.Snapshot
				FileRoot string `json:"file_root,omitempty"`
			}{snapshot, fileRoot})
		})
		ui.ReturnURL = returnURL
		ui.VerificationCode = code
		ui.ReadEvents = func(c uint64) ([]localui.Event, error) {
			return localui.ReadAuditEvents(filepath.Join(dir, "events.jsonl"), c)
		}
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return nil, e
		}
		ui.Host = ln.Addr().String()
		uiServer = &http.Server{Handler: ui, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
		go uiServer.Serve(ln)
		if ready != nil {
			controlURL := "http://" + ui.Host + "/#bootstrap=" + ui.Bootstrap
			if returnURL != "" {
				controlURL += "&return=" + url.QueryEscape(returnURL)
			}
			ready(controlURL, code)
		}
		if consent == nil {
			fmt.Fprintf(os.Stderr, "核对码：%s\n对端密钥：%s\n审计：%s\n客户控制：http://%s/#bootstrap=%s\n", code, gate.Snapshot().PeerKey, dir, ui.Host, ui.Bootstrap)
		} else {
			fmt.Fprintf(os.Stderr, "审计：%s\n", dir)
		}
		if consent != nil {
			if e := appendEvent("customer", "control.cli_consent_requested", "", 1, struct{}{}); e != nil {
				return nil, e
			}
			consentErr := consent(gate, code)
			if consentErr != nil {
				gate.End()
			}
			snapshot := gate.Snapshot()
			if e := appendEvent("customer", "control.cli_consent_result", "", snapshot.AuthorizationEpoch, snapshot); e != nil {
				gate.AuditFailure()
				return nil, e
			}
			if consentErr != nil {
				return nil, consentErr
			}
		} else {
			go ui.Watchdog(ctx.Done())
		}
		return gate, nil
	})
}

type customerHandler struct{ h executor.SSHHandler }

func (c *customerHandler) Run(s string, in io.Reader, out, err io.Writer) (int, error) {
	return c.h.Run(s, in, out, err)
}
func (c *customerHandler) Shell(in io.Reader, out io.Writer, w, h uint32, r <-chan sshbridge.Size) (int, error) {
	return c.h.Shell(in, out, w, h, r)
}
func (c *customerHandler) RPC(ch io.ReadWriteCloser) error { return c.h.RPC(ch) }
func merchant(ctx context.Context, base, ref, command string) error {
	m, e := remote.Create(ctx, base, os.Getenv("PAIRSHELL_CREATE_TOKEN"), nil)
	if e != nil {
		return e
	}
	fmt.Fprintln(os.Stdout, m.Invite.URL())
	fmt.Fprintln(os.Stderr, "将邀请发送给客户；等待配对。")
	var customerKey []byte
	client, e := m.Connect(ctx, func(tr pairing.Transcript, code string) error {
		if err := verifyPeerFromInput(ctx, bufio.NewReader(os.Stdin), os.Stderr, code); err != nil {
			return err
		}
		customerKey = append([]byte(nil), tr.CustomerKey...)
		return nil
	})
	if e != nil {
		return e
	}
	defer client.Close()
	if command != "" {
		s, e := client.NewSession()
		if e != nil {
			return e
		}
		defer s.Close()
		s.Stdout, s.Stderr = terminalDisplays(os.Stdout, os.Stderr)
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			s.Stdout = os.Stdout
		}
		if !term.IsTerminal(int(os.Stderr.Fd())) {
			s.Stderr = os.Stderr
		}
		return s.Run(command)
	}
	return serveMerchantAdapters(ctx, client, m, customerKey, ref, nil)
}
func serveMerchantAdapters(ctx context.Context, client *ssh.Client, m *remote.Merchant, customerKey []byte, ref string, ready func(string)) error {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer ln.Close()
	secret := token()
	if ref == "" {
		ref = filepath.Join(os.TempDir(), "pairshell-"+m.Invite.SessionID+".json")
	}
	b, _ := json.Marshal(struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}{"http://" + ln.Addr().String(), secret})
	if e = securefile.Write(ref, b); e != nil {
		return e
	}
	defer os.Remove(ref)
	defer os.Remove(ref + ".key")
	defer os.Remove(ref + ".known_hosts")
	defer os.Remove(ref + ".ssh_config")
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != ln.Addr().String() || r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == "CONNECT" && r.URL.Path == "/proxy" {
			ch, requests, e := client.OpenChannel("pairshell-proxy", nil)
			if e != nil {
				http.Error(w, "NO_ROUTE", 503)
				return
			}
			defer ch.Close()
			go func() {
				for req := range requests {
					req.Reply(false, nil)
				}
			}()
			h, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "proxy unavailable", 500)
				return
			}
			conn, rw, e := h.Hijack()
			if e != nil {
				return
			}
			defer conn.Close()
			rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
			rw.Flush()
			done := make(chan struct{})
			go func() { io.Copy(ch, rw); ch.CloseWrite(); close(done) }()
			io.Copy(conn, ch)
			return
		}
		if r.Method != "POST" || !strings.HasPrefix(r.URL.Path, "/bridge/") {
			http.Error(w, "forbidden", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1500000)
		request, e := http.NewRequest("POST", "http://customer"+r.URL.EscapedPath(), r.Body)
		if e != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.ContentLength = r.ContentLength
		status, body, e := merchantRPC(client, request, 2<<20, 20*time.Second)
		if e != nil {
			http.Error(w, "NO_ROUTE", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		w.Write(body)
	})}
	defer srv.Close()
	go srv.Serve(ln)
	if e = writeSSHConfig(m, customerKey, ref); e != nil {
		return e
	}
	if ready != nil {
		ready(ref)
	}
	fmt.Fprintf(os.Stderr, "已连接。MCP: pairshell mcp --session %s\nCtrl+C 结束。\n", ref)
	done := make(chan error, 1)
	go func() { done <- client.Wait() }()
	select {
	case <-ctx.Done():
		return nil
	case e := <-done:
		return e
	}
}

type safeOutput struct{ w io.Writer }

func (s safeOutput) Write(p []byte) (int, error) {
	var b strings.Builder
	for _, c := range string(p) {
		if c == '\n' || c == '\t' || !unicode.IsControl(c) {
			b.WriteRune(c)
		} else {
			fmt.Fprintf(&b, "\\x%02x", c)
		}
	}
	_, e := io.WriteString(s.w, b.String())
	return len(p), e
}
