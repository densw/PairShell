package executor

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPauseRejectsInputWithoutKillingTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY interactive input requires a real terminal; covered by manual matrix")
	}
	var admissions atomic.Int32
	m := New(t.TempDir(), func(_ uint64, _ string, _ bool, f func() error) error {
		if admissions.Add(1) == 3 {
			return errors.New("SESSION_PAUSED")
		}
		return f()
	}, func(string, string, any) error { return nil })
	defer m.Close()
	shell := "/bin/sh"
	command := "echo survived\nexit\n"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		command = "echo survived\r\nexit\r\n"
	}
	var out bytes.Buffer
	seen := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, e := m.Terminal(1, shell, io.MultiReader(strings.NewReader("ignored"), strings.NewReader(command)), pauseOutput{&out, seen}, 80, 24, nil)
		done <- e
	}()
	select {
	case <-done:
		if !strings.Contains(out.String(), "survived") {
			t.Fatalf("paused input killed terminal: %s", out.String())
		}
	case <-time.After(10 * time.Second):
		m.Close()
		t.Fatal("terminal stuck")
	}
}

type pauseOutput struct {
	b    *bytes.Buffer
	seen chan struct{}
}

func (w pauseOutput) Write(p []byte) (int, error) {
	n, e := w.b.Write(p)
	if strings.Contains(w.b.String(), "survived") {
		select {
		case w.seen <- struct{}{}:
		default:
		}
	}
	return n, e
}
