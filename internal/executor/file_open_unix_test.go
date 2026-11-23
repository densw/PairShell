//go:build unix

package executor

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFileOperationsRejectSpecialFilesPromptly(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(root, func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	readResult := make(chan error, 1)
	go func() {
		_, err := m.ReadFile(1, false, "pipe", 100)
		readResult <- err
	}()
	select {
	case err := <-readResult:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("read waited on a FIFO")
	}
	if err := m.WriteFile(1, false, "pipe", []byte("data"), ""); err == nil {
		t.Fatal("special file replaced")
	}
}
