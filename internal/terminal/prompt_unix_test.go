//go:build !windows

package terminal

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestInteractiveShellUsesNativePromptDefaults(t *testing.T) {
	for shell, prompt := range map[string]string{
		"/bin/zsh":  "%n@%m:%~ %# ",
		"/bin/bash": `\u@\h:\w\$ `,
		"/bin/sh":   "${PWD} $ ",
	} {
		args, env := interactiveShell(shell, []string{"PATH=/usr/bin:/bin"})
		if len(args) != 1 || args[0] != "-l" {
			t.Fatalf("%s not a login shell: %v", shell, args)
		}
		if !strings.Contains(strings.Join(env, "\n"), "PS1="+prompt) {
			t.Fatalf("%s missing directory prompt: %v", shell, env)
		}
	}
	_, env := interactiveShell("/bin/zsh", []string{"PS1=my-custom-prompt", "TERM=custom-term"})
	if strings.Count(strings.Join(env, "\n"), "PS1=") != 1 || !strings.Contains(strings.Join(env, "\n"), "PS1=my-custom-prompt") {
		t.Fatal("overwrote custom prompt")
	}
}

// This reads actual PTY output: echoing a cd command is not sufficient because
// the path must occur on a prompt line after the directory change has finished.
func TestPTYPromptTracksCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	child := "pairshell-prompt-child"
	if err := os.Mkdir(filepath.Join(root, child), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := Start(DefaultShell(), root, []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}, 120, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	changed := make(chan string, 1)
	go func() {
		var output strings.Builder
		buf := make([]byte, 4096)
		prompt := regexp.MustCompile(`(?m)[^\r\n]*pairshell-prompt-child[^\r\n]*[$%#] `)
		reported := false
		for {
			n, e := p.Read(buf)
			output.Write(buf[:n])
			if !reported && (prompt.MatchString(output.String()) || e != nil) {
				changed <- output.String()
				reported = true
			}
			if e != nil {
				return
			}
		}
	}()
	if _, err = p.Write([]byte("cd " + child + "\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case output := <-changed:
		if !regexp.MustCompile(`(?m)[^\r\n]*pairshell-prompt-child[^\r\n]*[$%#] `).MatchString(output) {
			t.Fatalf("no current-directory prompt: %q", output)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("directory prompt did not update after cd")
	}
	if _, err = p.Write([]byte("exit\r")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
