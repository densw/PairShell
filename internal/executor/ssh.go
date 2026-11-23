package executor

import (
	"errors"
	"fmt"
	"io"
	"pairshell/internal/sshbridge"
	"time"
)

// SSHHandler deliberately routes exec into the same idempotent audited manager.
type SSHHandler struct {
	Manager    *Manager
	Epoch      uint64
	ShellName  string
	Terminal   func(io.Reader, io.Writer, uint32, uint32, <-chan sshbridge.Size) (int, error)
	RPCHandler func(io.ReadWriteCloser) error
}

func (h SSHHandler) RPC(c io.ReadWriteCloser) error {
	if h.RPCHandler == nil {
		return ErrDenied
	}
	return h.RPCHandler(c)
}
func (h SSHHandler) Run(command string, input io.Reader, out, stderr io.Writer) (int, error) {
	t, e := h.Manager.Start(Request{RequestID: fmt.Sprintf("ssh-%d", time.Now().UnixNano()), Epoch: h.Epoch, Shell: h.ShellName, Command: command, TimeoutMS: 3600000})
	if e != nil {
		return 255, e
	}
	cursor := 0
	for {
		v, e := h.Manager.Status(t.ID)
		if e != nil {
			return 255, e
		}
		if len(v.Output) > cursor {
			if _, e = out.Write(v.Output[cursor:]); e != nil {
				h.Manager.Cancel(t.ID)
				return 255, e
			}
			cursor = len(v.Output)
		}
		if v.State != "RUNNING" && v.State != "ACCEPTED" {
			if v.ExitCode != nil {
				return *v.ExitCode, nil
			}
			return 255, errors.New(v.Error)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
func (h SSHHandler) Shell(in io.Reader, out io.Writer, w, height uint32, r <-chan sshbridge.Size) (int, error) {
	if h.Terminal == nil {
		return 255, errors.New("terminal unavailable")
	}
	return h.Terminal(in, out, w, height, r)
}
