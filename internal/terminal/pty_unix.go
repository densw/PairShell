//go:build !windows

package terminal

import (
	"context"
	"github.com/creack/pty"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type PTY struct {
	*os.File
	cmd *exec.Cmd
}

func Start(shell, cwd string, env []string, width, height int) (*PTY, error) {
	args, env := interactiveShell(shell, env)
	cmd := exec.Command(shell, args...)
	cmd.Dir = cwd
	cmd.Env = env
	f, e := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if e != nil {
		return nil, e
	}
	return &PTY{f, cmd}, nil
}

// Keep prompt expansion inside the real shell, so cd, jobs and interactive
// programs behave exactly as they do in a local terminal. Login profiles still
// load and can customize the prompt; no user files are changed.
func interactiveShell(shell string, env []string) ([]string, []string) {
	env = append([]string(nil), env...)
	setDefault := func(key, value string) {
		for _, entry := range env {
			if strings.HasPrefix(entry, key+"=") {
				return
			}
		}
		env = append(env, key+"="+value)
	}
	setDefault("TERM", "xterm-256color")
	setDefault("SHELL", shell)
	var args []string
	switch filepath.Base(shell) {
	case "zsh":
		args = []string{"-l"}
		setDefault("PS1", "%n@%m:%~ %# ")
	case "bash":
		args = []string{"-l"}
		setDefault("PS1", `\u@\h:\w\$ `)
	case "sh", "dash", "ksh":
		args = []string{"-l"}
		setDefault("PS1", "${PWD} $ ")
	case "fish":
		args = []string{"-l"}
	}
	return args, env
}
func (p *PTY) Resize(width, height int) error {
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
}
func (p *PTY) Close() error { syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); return p.File.Close() }
func (p *PTY) Wait(ctx context.Context) (int, error) {
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case e := <-done:
		if p.cmd.ProcessState != nil {
			return p.cmd.ProcessState.ExitCode(), e
		}
		return -1, e
	case <-ctx.Done():
		p.Close()
		e := <-done
		return -1, e
	}
}
