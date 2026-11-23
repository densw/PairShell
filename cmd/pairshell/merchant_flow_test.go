package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"pairshell/internal/executor"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMerchantWindowPairExecPauseEnd(t *testing.T) {
	srv := httptest.NewTLSServer(server.New(server.Config{CreateToken: "create"}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	m := &merchantWindow{ctx: ctx}
	base := os.Getenv("PAIRSHELL_TEST_RELAY")
	credential := os.Getenv("PAIRSHELL_TEST_TOKEN")
	httpClient := &http.Client{Timeout: 15 * time.Second}
	if base == "" {
		base = srv.URL
		credential = "create"
		httpClient = srv.Client()
	}
	m.httpClient = httpClient
	call := func(path string, value any) {
		t.Helper()
		b, _ := json.Marshal(value)
		r := httptest.NewRequest("POST", "http://local/merchant/"+path, bytes.NewReader(b))
		w := httptest.NewRecorder()
		m.serve(w, r)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	wait := func(f func() bool) {
		t.Helper()
		for i := 0; i < 400; i++ {
			if f() {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("flow timeout")
			case <-time.After(50 * time.Millisecond):
			}
		}
		t.Fatal("flow condition timeout")
	}
	call("create", map[string]string{"Server": base, "Token": credential})
	var invite string
	wait(func() bool { m.mu.Lock(); defer m.mu.Unlock(); invite = m.invite; return invite != "" })
	h := &customerHandler{}
	var g *session.Gate
	var tasks *executor.Manager
	codeReady := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- remote.ServeCustomer(ctx, invite, httpClient, h, func(tr pairing.Transcript, code string) (*session.Gate, error) {
			g = session.New(tr.SessionID, "peer", time.Now().Add(time.Hour))
			g.Consent(session.Capabilities{Tasks: true}, false)
			tasks = executor.New(t.TempDir(), g.WithAuthorization, func(string, string, any) error { return nil })
			shell := "sh"
			if runtime.GOOS == "windows" {
				shell = "powershell"
			}
			h.h = executor.SSHHandler{Manager: tasks, Epoch: 1, ShellName: shell}
			codeReady <- code
			return g, nil
		})
	}()
	var expected string
	select {
	case expected = <-codeReady:
	case <-ctx.Done():
		t.Fatal("pairing timeout")
	}
	wait(func() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.code != "" })
	m.mu.Lock()
	actual := m.code
	attemptID := m.attemptID
	m.mu.Unlock()
	if actual != expected {
		t.Fatal("verification mismatch")
	}
	call("confirm", map[string]any{"attempt_id": attemptID, "verification_code": expected})
	wait(func() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.state == "已连接" })
	call("exec", map[string]string{"Command": "echo PairShell-GUI-Check"})
	wait(func() bool { m.mu.Lock(); defer m.mu.Unlock(); return !m.running })
	m.mu.Lock()
	output := m.output
	m.mu.Unlock()
	if !strings.Contains(output, "PairShell-GUI-Check") || !strings.Contains(output, "[任务完成]") {
		t.Fatal(output)
	}
	g.Pause()
	call("exec", map[string]string{"Command": "echo MUST-NOT-EXECUTE"})
	wait(func() bool { m.mu.Lock(); defer m.mu.Unlock(); return !m.running })
	m.mu.Lock()
	output = m.output
	m.mu.Unlock()
	if strings.Contains(output, "MUST-NOT-EXECUTE") {
		t.Fatal("paused command executed")
	}
	call("end", struct{}{})
	tasks.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("customer did not close")
	}
}
