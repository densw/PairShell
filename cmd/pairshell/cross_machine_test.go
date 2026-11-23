package main

import (
	"context"
	"os"
	"pairshell/internal/executor"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
	"pairshell/internal/securefile"
	"pairshell/internal/session"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Opt-in integration probes only execute echo inside the customer's temporary directory.
// Their automatic confirmation is test-only; production windows always require consent.
func TestCrossMachineMerchant(t *testing.T) {
	path := os.Getenv("PAIRSHELL_TEST_INVITE_OUT")
	if path == "" {
		t.Skip("cross-machine probe disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	m, e := remote.Create(ctx, os.Getenv("PAIRSHELL_TEST_RELAY"), os.Getenv("PAIRSHELL_TEST_TOKEN"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = securefile.Write(path, []byte(m.Invite.URL())); e != nil {
		t.Fatal(e)
	}
	defer os.Remove(path)
	client, e := m.Connect(ctx, func(pairing.Transcript, string) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	s, e := client.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	out, e := s.CombinedOutput("echo PairShell-CrossMachine-Verified")
	if e != nil || !strings.Contains(string(out), "PairShell-CrossMachine-Verified") {
		t.Fatal("cross-machine execution failed", e)
	}
	t.Log("merchant OS", runtime.GOOS, "verified authenticated remote echo")
}
func TestCrossMachineCustomer(t *testing.T) {
	path := os.Getenv("PAIRSHELL_TEST_INVITE_IN")
	if path == "" {
		t.Skip("cross-machine probe disabled")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	h := &customerHandler{}
	var tasks *executor.Manager
	e = remote.ServeCustomer(ctx, string(raw), nil, h, func(tr pairing.Transcript, _ string) (*session.Gate, error) {
		g := session.New(tr.SessionID, "test-peer", time.Now().Add(time.Minute))
		g.Consent(session.Capabilities{Tasks: true}, false)
		tasks = executor.New(t.TempDir(), g.WithAuthorization, func(string, string, any) error { return nil })
		shell := "sh"
		if runtime.GOOS == "windows" {
			shell = "powershell"
		}
		h.h = executor.SSHHandler{Manager: tasks, Epoch: 1, ShellName: shell}
		return g, nil
	})
	if tasks != nil {
		tasks.Close()
	}
	if e != nil {
		t.Fatal(e)
	}
	t.Log("customer OS", runtime.GOOS, "completed controlled test session")
}
