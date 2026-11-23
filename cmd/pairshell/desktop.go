package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"pairshell/internal/history"
	"pairshell/internal/uiassets"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type desktopServer struct {
	history              *history.Store
	httpClient           *http.Client
	started, listening   bool
	listenCancel         context.CancelFunc
	ctx                  context.Context
	pairCode             string
	merchant             *merchantWindow
	host, secret         string
	mu                   sync.Mutex
	state, control, code string
	start                func(string)
	run                  func(context.Context, string, func(string, string)) error
	announce             func(string)
	generation           uint64
	phase                string
	sessionCancel        context.CancelFunc
	sessionDone          chan struct{}
}

func (d *desktopServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != d.host {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/merchant" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	}
	if d.serveTerminalAsset(w, r) {
		return
	}
	if r.URL.Path == "/merchant/terminal" {
		d.serveTerminal(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, desktopPage)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/merchant" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, merchantPage())
		return
	}
	if r.Method == "GET" && r.URL.Path == "/merchant.js" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, uiassets.MerchantJS)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/ui.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		io.WriteString(w, uiassets.CSS)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/desktop.js" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, desktopScript)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/history.js" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, uiassets.HistoryJS)
		return
	}
	if r.Header.Get("X-PairShell") != d.secret || d.secret == "" {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+d.host {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/history" || strings.HasPrefix(r.URL.Path, "/history/") {
		d.serveHistory(w, r)
		return
	}
	if r.URL.Path == "/settings" {
		d.serveSettings(w, r)
		return
	}
	if r.URL.Path == "/listen" && r.Method == "POST" {
		if r.Header.Get("Origin") != "http://"+d.host {
			http.Error(w, "forbidden", 403)
			return
		}
		ctx, generation, ok := d.beginListener()
		if !ok {
			http.Error(w, "请先结束当前协助", 409)
			return
		}
		go d.listenGeneration(ctx, generation)
		io.WriteString(w, "{}")
		return
	}
	if r.URL.Path == "/reset" && r.Method == "POST" {
		if r.Header.Get("Origin") != "http://"+d.host {
			http.Error(w, "forbidden", 403)
			return
		}
		d.resetCustomer()
		io.WriteString(w, "{}")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/merchant/") {
		if r.Method == "POST" && r.Header.Get("Origin") != "http://"+d.host {
			http.Error(w, "forbidden", 403)
			return
		}
		if d.merchant == nil {
			http.Error(w, "unavailable", 503)
			return
		}
		d.merchant.serve(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/status" {
		d.mu.Lock()
		defer d.mu.Unlock()
		phase := d.phase
		if phase == "" {
			phase = "idle"
		}
		json.NewEncoder(w).Encode(struct {
			State, Control, Code, PairCode, Phase string
			CanGenerate, CanReset                 bool
			AttemptID                             uint64
		}{d.state, d.control, d.code, d.pairCode, phase, !d.started, d.started || d.listening, d.generation})
		return
	}
	if r.Method != "POST" || r.URL.Path != "/start" || r.Header.Get("Origin") != "http://"+d.host {
		http.Error(w, "forbidden", 403)
		return
	}
	var v struct {
		Invite string `json:"invite"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 5000))
	dec.DisallowUnknownFields()
	if dec.Decode(&v) != nil || len(v.Invite) > 4096 {
		http.Error(w, "邀请格式无效", 400)
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	if !d.launchCustomer(v.Invite) {
		http.Error(w, "本次协助已启动", 409)
		return
	}
	io.WriteString(w, "{}")
}
func desktop() error {
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	cache, e := os.UserConfigDir()
	if e != nil {
		return e
	}
	logs := filepath.Join(cache, "PairShell", "Sessions")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	embedded := os.Getenv("PAIRSHELL_DESKTOP_EMBEDDED") == "1"
	if embedded && os.Getenv("PAIRSHELL_PARENT_STDIN") == "1" {
		go func() { io.Copy(io.Discard, os.Stdin); cancel() }()
	}
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer ln.Close()
	d := &desktopServer{ctx: ctx, host: ln.Addr().String(), secret: token(), state: "等待邀请"}
	d.history, e = history.New(logs)
	if e != nil {
		return e
	}
	if embedded {
		d.announce = func(control string) { fmt.Println("PAIRSHELL_WINDOW_URL=" + control) }
	}
	d.merchant = &merchantWindow{ctx: ctx, logRoot: logs}
	defer func() {
		d.merchant.mu.Lock()
		defer d.merchant.mu.Unlock()
		if d.merchant.cancel != nil {
			d.merchant.cancel()
		}
		if d.merchant.client != nil {
			d.merchant.client.Close()
		}
	}()
	d.run = func(sessionCtx context.Context, invite string, ready func(string, string)) error {
		returnURL := "http://" + d.host + "/#token=" + d.secret + "&new=1"
		return customerWithUIWithReturn(sessionCtx, invite, home, logs, returnURL, ready)
	}
	srv := &http.Server{Handler: d, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	defer srv.Close()
	go srv.Serve(ln)
	if embedded {
		fmt.Println("PAIRSHELL_WINDOW_URL=http://" + d.host + "/#token=" + d.secret)
	} else {
		return errors.New("请使用 PairShell 独立客户端启动；命令行请使用 host/connect/shell/mcp")
	}
	<-ctx.Done()
	d.mu.Lock()
	done := d.sessionDone
	if d.listenCancel != nil {
		d.listenCancel()
	}
	if d.sessionCancel != nil {
		d.sessionCancel()
	}
	d.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(8 * time.Second):
		}
	}
	return nil
}

var desktopPage = uiassets.Invite

var desktopScript = uiassets.DesktopJS
