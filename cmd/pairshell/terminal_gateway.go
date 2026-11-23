package main

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"pairshell/internal/audit"
	"pairshell/internal/uiassets"
)

// The browser authenticates in its first frame: no capability in URLs, history or logs.
func (d *desktopServer) serveTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || r.Host != d.host || r.Header.Get("Origin") != "http://"+d.host || r.URL.RawQuery != "" || d.secret == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	websocket.Server{Handshake: func(c *websocket.Config, request *http.Request) error {
		origin, err := websocket.Origin(c, request)
		if err != nil || origin == nil || origin.String() != "http://"+d.host {
			return errors.New("origin denied")
		}
		return nil
	}, Handler: websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		ws.MaxPayloadBytes = 65536
		ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		var auth struct {
			Token     string `json:"token"`
			Cols      int    `json:"cols"`
			Rows      int    `json:"rows"`
			AttemptID uint64 `json:"attempt_id"`
		}
		if websocket.JSON.Receive(ws, &auth) != nil || len(auth.Token) > 128 || subtle.ConstantTimeCompare([]byte(auth.Token), []byte(d.secret)) != 1 {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"窗口认证失败"})
			return
		}
		if !validTerminalSize(auth.Cols, auth.Rows) {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"终端尺寸无效"})
			return
		}
		m := d.merchant
		if m == nil {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"连接尚未就绪"})
			return
		}
		m.mu.Lock()
		client := m.client
		attempt := m.active
		attemptID := m.attemptID
		if client == nil || m.state != "已连接" || m.running || auth.AttemptID != attemptID {
			m.mu.Unlock()
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"连接尚未就绪，或终端正在使用"})
			return
		}
		m.running = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			if m.attemptID == attemptID && m.client == client {
				m.running = false
			}
			m.mu.Unlock()
		}()
		session, e := client.NewSession()
		if e != nil {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"无法打开远端终端"})
			return
		}
		defer session.Close()
		input, e := session.StdinPipe()
		if e != nil {
			return
		}
		output := &terminalSocketWriter{ws: ws}
		if attempt != nil {
			output.audit = attempt.audit
		}
		session.Stdout = output.stream()
		session.Stderr = output.stream()
		if e = session.RequestPty("xterm-256color", auth.Rows, auth.Cols, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}); e != nil {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"客户未开放交互终端"})
			return
		}
		if e = session.Shell(); e != nil {
			websocket.JSON.Send(ws, struct {
				Error string `json:"error"`
			}{"客户未开放交互终端"})
			return
		}
		ws.SetReadDeadline(time.Time{})
		if e = websocket.JSON.Send(ws, struct {
			Ready     bool   `json:"ready"`
			AttemptID uint64 `json:"attempt_id"`
		}{true, attemptID}); e != nil {
			return
		}
		go func() { session.Wait(); ws.Close() }()
		for {
			var message struct {
				Type string `json:"type"`
				Data string `json:"data"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if websocket.JSON.Receive(ws, &message) != nil {
				return
			}
			switch message.Type {
			case "input":
				if len(message.Data) > 16384 {
					return
				}
				if _, e = io.WriteString(input, message.Data); e != nil {
					return
				}
			case "resize":
				if !validTerminalSize(message.Cols, message.Rows) {
					return
				}
				if session.WindowChange(message.Rows, message.Cols) != nil {
					return
				}
			default:
				return
			}
		}
	})}.ServeHTTP(w, r)
}
func validTerminalSize(cols, rows int) bool {
	return cols >= 2 && cols <= 1000 && rows >= 1 && rows <= 1000
}

type terminalSocketWriter struct {
	mu    sync.Mutex
	ws    *websocket.Conn
	audit *audit.Writer
}

type terminalSocketStream struct {
	socket  *terminalSocketWriter
	display terminalDisplay
}

func (w *terminalSocketWriter) stream() io.Writer {
	return &terminalSocketStream{socket: w, display: terminalDisplay{output: w}}
}

func (s *terminalSocketStream) Write(p []byte) (int, error) {
	w := s.socket
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.audit != nil {
		payload, _ := json.Marshal(struct {
			Data string `json:"data_b64"`
		}{base64.StdEncoding.EncodeToString(p)})
		if e := w.audit.Append(audit.Event{Actor: "merchant", EventType: "terminal.output", AuthorizationEpoch: 1, Payload: payload}); e != nil {
			return 0, e
		}
	}
	return s.display.Write(p)
}

// Write emits the already-filtered bytes while the stream holds mu.
func (w *terminalSocketWriter) Write(p []byte) (int, error) {
	w.ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
	for start := 0; start < len(p); {
		end := start + 16384
		if end > len(p) {
			end = len(p)
		}
		if e := websocket.Message.Send(w.ws, p[start:end]); e != nil {
			return start, e
		}
		start = end
	}
	return len(p), nil
}

// Only compiled-in terminal resources are exposed; arbitrary disk paths are never served.
func (d *desktopServer) serveTerminalAsset(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/terminal-assets/") {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, "/terminal-assets/")
	if path.Base(name) != name || name == "." {
		http.NotFound(w, r)
		return true
	}
	data, e := uiassets.TerminalAssets.ReadFile("terminal-assets/" + name)
	if e != nil {
		http.NotFound(w, r)
		return true
	}
	switch path.Ext(name) {
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(data)
	return true
}
