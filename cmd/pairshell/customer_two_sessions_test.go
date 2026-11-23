package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"pairshell/internal/audit"
	"pairshell/internal/executor"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDesktopPairCanCompleteTwoFreshSessionsWithoutReopening(t *testing.T) {
	relay := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true, CreateToken: "never-given-to-endpoints"}))
	defer relay.Close()
	t.Setenv("PAIRSHELL_SERVER", relay.URL)
	t.Setenv("PAIRSHELL_CREATE_TOKEN", "")
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	d := &desktopServer{ctx: ctx, httpClient: relay.Client(), host: "customer.local", secret: "customer-window"}
	m := &merchantWindow{ctx: ctx, httpClient: relay.Client()}
	merchant := &desktopServer{host: "merchant.local", secret: "merchant-window", merchant: m}
	root, logs := t.TempDir(), t.TempDir()
	ready := make(chan *session.Gate, 2)
	d.run = func(sessionCtx context.Context, invite string, publish func(string, string)) error {
		handler := &customerHandler{}
		var cleanup func()
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()
		return remote.ServeCustomer(sessionCtx, invite, relay.Client(), handler, func(tr pairing.Transcript, code string) (*session.Gate, error) {
			gate := session.New(tr.SessionID, "verified helper", time.Now().Add(time.Hour))
			writer, e := audit.NewWriter(filepath.Join(logs, tr.SessionID), tr.SessionID)
			if e != nil {
				return nil, e
			}
			manager := executor.New(root, gate.WithAuthorization, func(kind, id string, data any) error {
				payload, e := json.Marshal(data)
				if e != nil {
					return e
				}
				return writer.Append(audit.Event{Actor: "merchant", EventType: kind, RequestID: id, AuthorizationEpoch: 1, Payload: payload})
			})
			shell := "sh"
			if runtime.GOOS == "windows" {
				shell = "powershell"
			}
			handler.h = executor.SSHHandler{Manager: manager, Epoch: 1, ShellName: shell}
			publish("http://customer-control/", code)
			ready <- gate
			cleanup = func() {
				manager.Close()
				waitCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				manager.Wait(waitCtx)
				writer.Close("test session complete")
			}
			return gate, nil
		})
	}
	call := func(target *desktopServer, path string, body any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://"+target.host+path, bytes.NewReader(raw))
		r.Header.Set("Origin", "http://"+target.host)
		r.Header.Set("X-PairShell", target.secret)
		w := httptest.NewRecorder()
		target.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	wait := func(fn func() bool) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for !fn() {
			if time.Now().After(deadline) {
				d.mu.Lock()
				ds := fmt.Sprintf("customer phase=%s started=%v state=%s generation=%d", d.phase, d.started, d.state, d.generation)
				d.mu.Unlock()
				m.mu.Lock()
				ms := fmt.Sprintf("merchant phase=%s state=%s reference=%s", m.phase, m.state, m.reference)
				m.mu.Unlock()
				stacks := make([]byte, 1<<20)
				n := runtime.Stack(stacks, true)
				t.Fatalf("lifecycle did not settle: %s; %s\n%s", ds, ms, stacks[:n])
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	var previousGate *session.Gate
	var previousID string
	for round := 1; round <= 2; round++ {
		call(d, "/listen", struct{}{})
		var code string
		wait(func() bool { d.mu.Lock(); defer d.mu.Unlock(); code = d.pairCode; return code != "" })
		call(merchant, "/merchant/join", struct{ Code string }{code})
		var gate *session.Gate
		select {
		case gate = <-ready:
		case <-ctx.Done():
			t.Fatal("customer did not await consent")
		}
		if previousID != "" && gate.Snapshot().SessionID == previousID {
			t.Fatal("reused old session identity")
		}
		if previousGate != nil && previousGate.Check(1, "tasks", false) == nil {
			t.Fatal("previous consent remained authorized")
		}
		m.mu.Lock()
		early := m.client
		m.mu.Unlock()
		if early != nil {
			t.Fatal("new session skipped fresh customer consent")
		}
		var peerCode string
		var attemptID uint64
		wait(func() bool {
			d.mu.Lock()
			peerCode = d.code
			d.mu.Unlock()
			m.mu.Lock()
			defer m.mu.Unlock()
			attemptID = m.attemptID
			return peerCode != "" && m.state == "等待商家核对身份" && m.code == peerCode
		})
		call(merchant, "/merchant/confirm", map[string]any{"attempt_id": attemptID, "verification_code": peerCode})
		if e := gate.Consent(session.Capabilities{Tasks: true}, false); e != nil {
			t.Fatal(e)
		}
		var ref string
		wait(func() bool {
			m.mu.Lock()
			defer m.mu.Unlock()
			ref = m.reference
			return m.phase == "active" && ref != ""
		})
		cli, e := localSSHClient(ctx, ref)
		if e != nil {
			t.Fatal(e)
		}
		s, e := cli.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		marker := fmt.Sprintf("PairShell-Fresh-Round-%d", round)
		output, e := s.CombinedOutput("echo " + marker)
		s.Close()
		cli.Close()
		if e != nil || !strings.Contains(string(output), marker) {
			t.Fatalf("round%d execution %q %v", round, output, e)
		}
		call(merchant, "/merchant/end", struct{}{})
		wait(func() bool { d.mu.Lock(); defer d.mu.Unlock(); return !d.started })
		wait(func() bool { _, e := os.Stat(ref); return os.IsNotExist(e) })
		if gate.Check(1, "tasks", false) == nil {
			t.Fatal("finished customer remained authorized")
		}
		verified, e := audit.Verify(filepath.Join(logs, gate.Snapshot().SessionID))
		if e != nil || !verified.Complete {
			t.Fatalf("round %d audit not closed: %+v %v", round, verified, e)
		}
		previousGate = gate
		previousID = gate.Snapshot().SessionID
	}
	cancel()
}
