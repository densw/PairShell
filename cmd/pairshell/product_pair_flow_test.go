package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/audit"
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

// This crosses the production desktop listener, code entry, PAKE exchange, peer
// handshake, Pion channel, SSH adapter and real customer execution gate.
func TestDeviceCodeFlowWaitsForConsentAndUsesSameGateForCLI(t *testing.T) {
	relay := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true, CreateToken: "not-distributed-to-clients"}))
	defer relay.Close()
	t.Setenv("PAIRSHELL_SERVER", relay.URL)
	t.Setenv("PAIRSHELL_CREATE_TOKEN", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := &desktopServer{ctx: ctx, httpClient: relay.Client(), host: "customer.local", secret: "customer-window", state: "等待邀请"}
	m := &merchantWindow{ctx: ctx, httpClient: relay.Client()}
	merchant := &desktopServer{ctx: ctx, host: "merchant.local", secret: "merchant-window", merchant: m}
	root := t.TempDir()
	logDir := t.TempDir() + "/session"
	type customerState struct {
		gate    *session.Gate
		manager *executor.Manager
		writer  *audit.Writer
		code    string
	}
	customerReady := make(chan customerState, 1)
	customerDone := make(chan error, 1)
	d.start = func(invite string) {
		h := &customerHandler{}
		customerDone <- remote.ServeCustomer(ctx, invite, relay.Client(), h, func(tr pairing.Transcript, code string) (*session.Gate, error) {
			g := session.New(tr.SessionID, "verified helper", time.Now().Add(time.Hour))
			writer, e := audit.NewWriter(logDir, tr.SessionID)
			if e != nil {
				return nil, e
			}
			manager := executor.New(root, g.WithAuthorization, func(kind, id string, p any) error {
				b, e := json.Marshal(p)
				if e != nil {
					return e
				}
				return writer.Append(audit.Event{Actor: "merchant", EventType: kind, RequestID: id, AuthorizationEpoch: 1, Payload: b})
			})
			shell := "sh"
			if runtime.GOOS == "windows" {
				shell = "powershell"
			}
			h.h = executor.SSHHandler{Manager: manager, Epoch: 1, ShellName: shell}
			customerReady <- customerState{g, manager, writer, code}
			return g, nil
		})
	}
	call := func(target *desktopServer, path string, body any) int {
		t.Helper()
		b, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest(http.MethodPost, "http://"+target.host+path, bytes.NewReader(b))
		r.Header.Set("Origin", "http://"+target.host)
		r.Header.Set("X-PairShell", target.secret)
		w := httptest.NewRecorder()
		target.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Log(path, w.Code, w.Body.String())
		}
		return w.Code
	}
	wait := func(ready func() bool) {
		t.Helper()
		for {
			if ready() {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("product flow timeout")
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if status := call(d, "/listen", struct{}{}); status != http.StatusOK {
		t.Fatal("customer listener rejected", status)
	}
	var code string
	wait(func() bool { d.mu.Lock(); defer d.mu.Unlock(); code = d.pairCode; return code != "" })
	if status := call(merchant, "/merchant/join", struct{ Code string }{code}); status != http.StatusOK {
		t.Fatal("merchant join rejected", status)
	}
	var state customerState
	select {
	case state = <-customerReady:
	case <-ctx.Done():
		t.Fatal("customer did not reach consent")
	}
	defer state.manager.Close()
	defer state.writer.Close("test cleanup")
	var attemptID uint64
	wait(func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		attemptID = m.attemptID
		return m.state == "等待商家核对身份" && m.code == state.code
	})
	if status := call(merchant, "/merchant/confirm", map[string]any{"attempt_id": attemptID, "verification_code": state.code}); status != http.StatusOK {
		t.Fatal("merchant did not accept the code provided by the customer", status)
	}
	m.mu.Lock()
	earlyClient := m.client
	m.mu.Unlock()
	if earlyClient != nil {
		t.Fatal("SSH opened before customer consent")
	}
	if state.gate.Snapshot().State != "AWAITING_CONSENT" {
		t.Fatal("consent was bypassed")
	}
	if e := state.gate.Consent(session.Capabilities{Terminal: true, Tasks: true}, false); e != nil {
		t.Fatal(e)
	}
	var reference string
	wait(func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		reference = m.reference
		return m.state == "已连接" && reference != ""
	})
	cli, e := localSSHClient(ctx, reference)
	if e != nil {
		t.Fatal("native CLI adapter failed:", e)
	}
	run := func(command string) ([]byte, error) {
		s, e := cli.NewSession()
		if e != nil {
			return nil, e
		}
		defer s.Close()
		return s.CombinedOutput(command)
	}
	output, e := run("echo PairShell-DeviceCode-CLI")
	if e != nil || !strings.Contains(string(output), "PairShell-DeviceCode-CLI") {
		t.Fatalf("real execution: %q %v", output, e)
	}
	if e = state.gate.Pause(); e != nil {
		t.Fatal(e)
	}
	output, e = run("echo MUST_NOT_EXECUTE_PAUSED")
	if e == nil || strings.Contains(string(output), "MUST_NOT_EXECUTE_PAUSED") {
		t.Fatalf("pause bypass: %q %v", output, e)
	}
	state.gate.End()
	output, e = run("echo MUST_NOT_EXECUTE_ENDED")
	if e == nil || strings.Contains(string(output), "MUST_NOT_EXECUTE_ENDED") {
		t.Fatalf("revoke bypass: %q %v", output, e)
	}
	cli.Close()
	if status := call(merchant, "/merchant/end", struct{}{}); status != http.StatusOK {
		t.Fatal("merchant end rejected", status)
	}
	select {
	case <-customerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("customer did not close")
	}
	state.manager.Close()
	if e = state.writer.Close("complete"); e != nil {
		t.Fatal(e)
	}
	verification, e := audit.Verify(logDir)
	if e != nil || !verification.Complete {
		t.Fatalf("audit verification: %+v %v", verification, e)
	}
}
