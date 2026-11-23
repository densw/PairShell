package terminal

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPTYOutputAndCleanup(t *testing.T) {
	shell := "/bin/sh"
	command := "printf pairshell-terminal; exit\n"
	if runtime.GOOS == "windows" {
		shell = "powershell.exe -NoProfile -NoLogo"
		command = "Write-Output 'pairshell-terminal'; exit\r\n"
	}
	p, e := Start(shell, "", os.Environ(), 80, 24)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	if e = p.Resize(90, 30); e != nil {
		t.Fatal(e)
	}
	done := make(chan string, 1)
	go func() {
		var text strings.Builder
		b := make([]byte, 4096)
		for {
			n, e := p.Read(b)
			text.Write(b[:n])
			if strings.Contains(text.String(), "pairshell-terminal") {
				done <- text.String()
				return
			}
			if e != nil {
				done <- text.String()
				return
			}
		}
	}()
	if _, e = p.Write([]byte(command)); e != nil {
		t.Fatal(e)
	}
	select {
	case text := <-done:
		if !strings.Contains(text, "pairshell-terminal") {
			t.Fatal(text)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("terminal output timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, e = p.Wait(ctx); e != nil {
		t.Fatal(e)
	}
}
