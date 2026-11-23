package executor

import (
	"bytes"
	"os"
	"pairshell/internal/session"
	"path/filepath"
	"testing"
	"time"
)

func TestTerminalInputAuditDoesNotHoldAuthorization(t *testing.T) {
	gate := session.New("s", "peer", time.Now().Add(time.Hour))
	if err := gate.Consent(session.Capabilities{Terminal: true}, false); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	manager := New(t.TempDir(), gate.WithAuthorization, func(kind, _ string, _ any) error {
		if kind == "terminal.input.intent" {
			close(started)
			<-release
		}
		return nil
	})
	defer manager.Close()
	manager.SetStagedAuthorization(gate.StagedAuthorization)
	var terminal bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- manager.sendTerminalInput(1, &terminal, func() error { return nil }, []byte("input"))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("terminal input did not reach the blocked audit write")
	}
	revoked := make(chan struct{})
	go func() { gate.End(); close(revoked) }()
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("terminal input blocked revocation")
	}
	release <- struct{}{}
	select {
	case err := <-done:
		if err == nil || terminal.Len() > 0 {
			t.Fatalf("revoked input reached terminal: %s (%v)", &terminal, err)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal input did not finish")
	}
}

func TestWriteRejectsOversizedExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large")
	os.WriteFile(path, make([]byte, (1<<20)+1), 0600)
	m := New(root, func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	if e := m.WriteFile(1, false, "large", []byte("new"), digest(make([]byte, (1<<20)+1))); e == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestFileReadDoesNotBlockRevocationOrReturnRevokedData(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("contents"), 0600); err != nil {
		t.Fatal(err)
	}
	gate := session.New("s", "peer", time.Now().Add(time.Hour))
	if err := gate.Consent(session.Capabilities{FileRead: true}, true); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	manager := New(root, gate.WithAuthorization, func(string, string, any) error {
		close(started)
		<-release
		return nil
	})
	manager.SetStagedAuthorization(gate.StagedAuthorization)
	type result struct {
		file File
		err  error
	}
	done := make(chan result, 1)
	go func() {
		f, err := manager.ReadFile(1, true, "source", 100)
		done <- result{f, err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("read did not reach the test's blocked audit write")
	}
	revoked := make(chan struct{})
	go func() {
		gate.End()
		manager.Close()
		close(revoked)
	}()
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("revocation waited on a blocked file operation")
	}
	release <- struct{}{}
	select {
	case r := <-done:
		if r.err == nil || len(r.file.Data) != 0 {
			t.Fatalf("revoked data returned: %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("file operation did not exit")
	}
}

func TestStagedWriteCannotCommitAfterPauseAndResume(t *testing.T) {
	root := t.TempDir()
	gate := session.New("s", "peer", time.Now().Add(time.Hour))
	if err := gate.Consent(session.Capabilities{FileWrite: true}, true); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), gate.WithAuthorization, func(kind, _ string, _ any) error {
		if kind == "file.write.intent" {
			if err := gate.Pause(); err != nil {
				return err
			}
			return gate.Resume()
		}
		return nil
	})
	defer manager.Close()
	manager.SetStagedAuthorization(gate.StagedAuthorization)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetFileRoot(root, info); err != nil {
		t.Fatal(err)
	}
	if err := manager.WriteFile(1, true, "output", []byte("new"), ""); err == nil {
		t.Fatal("staged write resumed after authorization was interrupted")
	}
	if _, err := os.Stat(filepath.Join(root, "output")); !os.IsNotExist(err) {
		t.Fatalf("revoked write committed: %v", err)
	}
}
