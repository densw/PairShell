package executor

import (
	"bytes"
	"errors"
	"io"
	"pairshell/internal/sshbridge"
	"runtime"
	"testing"
	"time"
)

func TestTerminalViewMirrorsAuditedBytesAndOwnsItsCopies(t *testing.T) {
	var remote bytes.Buffer
	m := New(t.TempDir(), nil, func(string, string, any) error { return nil })
	m.terminalView.lifecycle("start", 100, 30)
	data := []byte("\x1b[32muser:~/Documents$ ls\r\n中文\x1b[0m")
	if _, err := (&terminalOutput{m: m, out: &remote}).Write(data); err != nil {
		t.Fatal(err)
	}
	m.terminalView.lifecycle("resize", 120, 40)
	m.terminalView.lifecycle("end", 0, 0)
	data[0] = 'x'
	v := m.TerminalView(0)
	if v.Truncated || len(v.Events) != 4 || v.Width != 120 || v.Height != 40 {
		t.Fatalf("view=%+v", v)
	}
	if !bytes.Equal(v.Events[1].Data, remote.Bytes()) {
		t.Fatal("view differs from merchant PTY bytes")
	}
	v.Events[1].Data[0] = 'y'
	if !bytes.Equal(m.TerminalView(0).Events[1].Data, remote.Bytes()) {
		t.Fatal("viewer mutated stored output")
	}
	if got := m.TerminalView(v.Cursor); len(got.Events) != 0 || got.Cursor != v.Cursor {
		t.Fatalf("cursor replayed events: %+v", got)
	}
	if got := New(t.TempDir(), nil, nil).TerminalView(0); len(got.Events) != 0 || got.Cursor != 0 {
		t.Fatal("new session exposed previous session")
	}
}

func TestTerminalViewMatchesRealPTYOutput(t *testing.T) {
	m := New(t.TempDir(), func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	input, writeInput := io.Pipe()
	defer input.Close()
	defer writeInput.Close()
	resize := make(chan sshbridge.Size, 1)
	var remote bytes.Buffer
	done := make(chan error, 1)
	shell := "/bin/sh"
	if runtime.GOOS == "windows" {
		shell = "powershell.exe -NoProfile -NoLogo"
	}
	go func() {
		_, err := m.Terminal(1, shell, input, &remote, 80, 24, resize)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for m.TerminalView(0).Width == 0 {
		select {
		case err := <-done:
			t.Fatalf("PTY failed before start: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("PTY did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	resize <- sshbridge.Size{Width: 100, Height: 30}
	for m.TerminalView(0).Width != 100 {
		if time.Now().After(deadline) {
			t.Fatal("PTY resize not mirrored")
		}
		time.Sleep(10 * time.Millisecond)
	}
	command := "printf '%s%s\\n' MIRROR_ REAL_PTY\n"
	if runtime.GOOS == "windows" {
		command = "Write-Output ('MIRROR_' + 'REAL_PTY')\r\n"
	}
	if _, err := io.WriteString(writeInput, command); err != nil {
		t.Fatal(err)
	}
	for {
		var display []byte
		for _, event := range m.TerminalView(0).Events {
			display = append(display, event.Data...)
		}
		if bytes.Contains(display, []byte("MIRROR_REAL_PTY")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("PTY marker missing: %q", display)
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.Close()
	select {
	case err := <-done:
		if err != nil && !bytes.Contains(remote.Bytes(), []byte("MIRROR_REAL_PTY")) {
			t.Fatalf("PTY failed: %v, output=%q", err, remote.Bytes())
		}
	case <-time.After(12 * time.Second):
		m.Close()
		t.Fatal("PTY did not finish")
	}
	v := m.TerminalView(0)
	var mirrored []byte
	for _, event := range v.Events {
		if event.Type == "output" {
			mirrored = append(mirrored, event.Data...)
		}
	}
	if !bytes.Contains(mirrored, []byte("MIRROR_REAL_PTY")) || !bytes.Equal(mirrored, remote.Bytes()) {
		t.Fatalf("real PTY output mismatch: mirror=%q remote=%q", mirrored, remote.Bytes())
	}
	if len(v.Events) < 3 || v.Events[0].Type != "start" || v.Events[len(v.Events)-1].Type != "end" {
		t.Fatalf("incomplete PTY lifecycle: %+v", v.Events)
	}
}

func TestTerminalViewExcludesOutputAfterAuditFailure(t *testing.T) {
	var remote bytes.Buffer
	m := New(t.TempDir(), nil, func(string, string, any) error { return errors.New("audit failed") })
	if _, err := (&terminalOutput{m: m, out: &remote}).Write([]byte("secret")); err == nil {
		t.Fatal("audit failure accepted")
	}
	if remote.Len() != 0 || len(m.TerminalView(0).Events) != 0 {
		t.Fatal("unaudited output reached viewer or merchant")
	}
}

func TestTerminalViewBoundsHistoryAndPagesWithoutDroppingCursor(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	data := bytes.Repeat([]byte("x"), 700<<10)
	m.terminalView.output(data)
	var received []byte
	var cursor uint64
	for {
		v := m.TerminalView(cursor)
		if cursor == 0 && !v.Truncated {
			t.Fatal("expired history not marked truncated")
		}
		if cursor > 0 && v.Truncated {
			t.Fatal("valid cursor marked truncated")
		}
		if len(v.Events) == 0 {
			break
		}
		var page int
		for _, event := range v.Events {
			if event.Sequence <= cursor {
				t.Fatal("events not ordered")
			}
			page += len(event.Data)
			received = append(received, event.Data...)
			cursor = event.Sequence
		}
		if v.Cursor != cursor || page > 128<<10 {
			t.Fatal("unbounded or inaccurate page")
		}
	}
	if len(received) == 0 || len(received) > 512<<10 {
		t.Fatalf("retained bytes=%d", len(received))
	}
	for i := 0; i < 1100; i++ {
		m.terminalView.lifecycle("resize", 80, 24)
	}
	if m.terminalView.size > 1024 || m.terminalView.bytes != 0 {
		t.Fatal("event count did not bound history")
	}
}
