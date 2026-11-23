package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"pairshell/internal/audit"
	"pairshell/internal/executor"
	"pairshell/internal/localui"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoteRPCUsesCustomerGateAndAudit(t *testing.T) {
	relay := httptest.NewTLSServer(server.New(server.Config{CreateToken: "create"}))
	defer relay.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, credential, httpClient := relay.URL, "create", relay.Client()
	if external := os.Getenv("PAIRSHELL_TEST_RELAY"); external != "" {
		base = external
		credential = os.Getenv("PAIRSHELL_TEST_TOKEN")
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	m, e := remote.Create(ctx, base, credential, httpClient)
	if e != nil {
		t.Fatal(e)
	}
	handler := &customerHandler{}
	var gate *session.Gate
	var manager *executor.Manager
	dir := t.TempDir() + "/audit"
	writer, e := audit.NewWriter(dir, m.Invite.SessionID)
	if e != nil {
		t.Fatal(e)
	}
	record := func(kind, id string, v any) error {
		b, _ := json.Marshal(v)
		return writer.Append(audit.Event{Actor: "test", EventType: kind, RequestID: id, AuthorizationEpoch: 1, Payload: b})
	}
	customerDone := make(chan error, 1)
	go func() {
		customerDone <- remote.ServeCustomer(ctx, m.Invite.URL(), httpClient, handler, func(tr pairing.Transcript, _ string) (*session.Gate, error) {
			gate = session.New(tr.SessionID, "peer", time.Now().Add(time.Hour))
			gate.Consent(session.Capabilities{Tasks: true, FileRead: true, FileWrite: true}, true)
			manager = executor.New(t.TempDir(), gate.WithAuthorization, record)
			bridge := localui.Bridge{Token: "private", Gate: gate, Tasks: manager}
			handler.h = executor.SSHHandler{Manager: manager, Epoch: 1, ShellName: "sh", RPCHandler: func(ch io.ReadWriteCloser) error {
				defer ch.Close()
				req, e := http.ReadRequest(bufio.NewReader(ch))
				if e != nil {
					return e
				}
				req.Header.Set("Authorization", "Bearer private")
				w := httptest.NewRecorder()
				bridge.ServeHTTP(w, req)
				return w.Result().Write(ch)
			}}
			return gate, nil
		})
	}()
	client, e := m.Connect(ctx, func(pairing.Transcript, string) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	call := func(tool, body string) int {
		t.Helper()
		ch, requests, e := client.OpenChannel("pairshell-rpc", nil)
		if e != nil {
			t.Fatal(e)
		}
		defer ch.Close()
		go func() {
			for req := range requests {
				req.Reply(false, nil)
			}
		}()
		req, _ := http.NewRequest("POST", "http://remote/bridge/"+tool, strings.NewReader(body))
		if e = req.Write(ch); e != nil {
			t.Fatal(e)
		}
		res, e := http.ReadResponse(bufio.NewReader(ch), req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}
	argv := []string{"sh", "-c", "printf pairshell"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "echo pairshell"}
	}
	body, _ := json.Marshal(executor.Request{RequestID: "rpc-task", Epoch: 1, Argv: argv, TimeoutMS: 1000}) // adapter forces AI; AI is not an accepted wire argument
	var payload map[string]any
	json.Unmarshal(body, &payload)
	delete(payload, "ai")
	body, _ = json.Marshal(payload)
	if status := call("task_start", string(body)); status != 200 {
		t.Fatalf("start %d", status)
	}
	waitCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if e = manager.Wait(waitCtx); e != nil {
		t.Fatal(e)
	}
	if status := call("task_start", string(body)); status != 200 {
		t.Fatalf("dedup %d", status)
	}
	gate.Pause()
	if status := call("task_start", string(body)); status != 409 {
		t.Fatalf("paused %d", status)
	}
	gate.End()
	if status := call("file_read", `{"authorization_epoch":1,"path":"a","limit":10}`); status != 409 {
		t.Fatalf("revoked %d", status)
	}
	manager.Close()
	if e = writer.Close("test complete"); e != nil {
		t.Fatal(e)
	}
	v, e := audit.Verify(dir)
	if e != nil || !v.Complete {
		t.Fatalf("audit %v %v", v, e)
	}
	client.Close()
	cancel()
	select {
	case <-customerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("customer cleanup timeout")
	}
}
