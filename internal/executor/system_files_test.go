package executor

import (
	"io"
	"os"
	"pairshell/internal/session"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSystemFilesUseRealPathsButStillRequireSessionConsent(t *testing.T) {
	cwd, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "source.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	g := session.New("full-session", "peer", time.Now().Add(time.Hour))
	m := NewSystem(cwd, g.WithAuthorization, func(string, string, any) error { return nil })
	m.SetStagedAuthorization(g.StagedAuthorization)
	defer m.Close()
	if _, err := m.ReadFile(1, true, path, 100); err == nil {
		t.Fatal("read before consent")
	}
	if err := g.Consent(session.Capabilities{Terminal: true, Tasks: true, FileRead: true, FileWrite: true}, true); err != nil {
		t.Fatal(err)
	}
	f, err := m.ReadFile(1, true, path, 100)
	if err != nil || string(f.Data) != "before" {
		t.Fatalf("absolute read outside CWD: %+v %v", f, err)
	}
	if err := m.WriteFile(1, true, path, []byte("after"), f.SHA256); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "after" {
		t.Fatal("real file was not updated")
	}
	if err := m.WriteFile(1, true, "relative.txt", []byte("relative"), ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(cwd, "relative.txt")); string(data) != "relative" {
		t.Fatal("relative path did not use remote CWD")
	}
	if err := g.Pause(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile(1, true, path, 100); err == nil {
		t.Fatal("paused read succeeded")
	}
	g.End()
	if err := m.WriteFile(1, true, path, []byte("revoked"), digest([]byte("after"))); err == nil {
		t.Fatal("write after end succeeded")
	}
}

func TestSystemToolsWorkWhileTerminalIsOpen(t *testing.T) {
	t.Setenv("PAIRSHELL_REMOTE_ENV_TEST", "remote-environment-visible")
	g := session.New("full-pty", "peer", time.Now().Add(time.Hour))
	if err := g.Consent(session.Capabilities{Terminal: true, Tasks: true, FileRead: true, FileWrite: true}, true); err != nil {
		t.Fatal(err)
	}
	m := NewSystem(t.TempDir(), g.WithAuthorization, func(string, string, any) error { return nil })
	m.SetStagedAuthorization(g.StagedAuthorization)
	defer m.Close()
	in, input := io.Pipe()
	defer in.Close()
	defer input.Close()
	shell := "/bin/sh"
	if runtime.GOOS == "windows" {
		shell = "powershell.exe -NoProfile -NoLogo"
	}
	done := make(chan error, 1)
	go func() { _, err := m.Terminal(1, shell, in, io.Discard, 80, 24, nil); done <- err }()
	deadline := time.Now().Add(15 * time.Second)
	for m.TerminalView(0).Width == 0 {
		select {
		case err := <-done:
			t.Fatalf("PTY failed: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("PTY startup timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	outside := t.TempDir()
	if err := m.WriteFile(1, true, filepath.Join(outside, "parallel.txt"), []byte("from-ai"), ""); err != nil {
		t.Fatalf("file tool blocked by open terminal: %v", err)
	}
	command := `printf '%s\n' "$PAIRSHELL_REMOTE_ENV_TEST"; pwd`
	if runtime.GOOS == "windows" {
		command = `Write-Output $env:PAIRSHELL_REMOTE_ENV_TEST; (Get-Location).Path`
	}
	task, err := m.Start(Request{RequestID: "with-terminal", Epoch: 1, AI: true, Command: command, Cwd: outside, TimeoutMS: 10000})
	if err != nil {
		t.Fatalf("MCP task blocked by open terminal: %v", err)
	}
	for {
		v, err := m.Status(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if v.State != "ACCEPTED" && v.State != "RUNNING" {
			if v.State != "SUCCEEDED" || !strings.Contains(string(v.Output), "remote-environment-visible") || !strings.Contains(strings.ToLower(string(v.Output)), strings.ToLower(outside)) {
				t.Fatalf("remote task environment/CWD mismatch: %+v output=%q", v, string(v.Output))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parallel task timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	g.End()
	m.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("terminal did not stop after revocation")
	}
}

func TestSystemTaskAcceptsDirectoryOutsideInitialCWD(t *testing.T) {
	m := NewSystem(t.TempDir(), func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	// Admission itself proves the path policy; the nonexistent executable exits safely.
	_, err := m.Start(Request{RequestID: "outside", Epoch: 1, AI: true, Argv: []string{"pairshell-nonexistent-test-tool"}, Cwd: t.TempDir(), TimeoutMS: 1000})
	if err != nil {
		t.Fatalf("outside CWD rejected: %v", err)
	}
}
