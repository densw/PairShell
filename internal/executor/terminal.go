package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"pairshell/internal/sshbridge"
	"pairshell/internal/terminal"
	"sync"

	"time"
)

func (m *Manager) Terminal(epoch uint64, shell string, in io.Reader, out io.Writer, w, h uint32, resize <-chan sshbridge.Size) (int, error) {
	var p *terminal.PTY
	var closeOnce sync.Once
	var closeErr error
	closePTY := func() error {
		closeOnce.Do(func() { closeErr = p.Close() })
		return closeErr
	}
	admission, e := m.prepareOperation(epoch, "terminal", false)
	if e != nil {
		return 255, e
	}
	m.mu.Lock()
	if m.closed || m.terminalBusy || (!m.systemFilesystem && (m.busy || m.fileBusy)) {
		m.mu.Unlock()
		return 255, ErrDenied
	}
	m.terminalBusy = true
	if !m.systemFilesystem {
		m.busy = true
	}
	m.mu.Unlock()
	defer func() {
		if p != nil {
			closePTY()
		}
		m.mu.Lock()
		m.terminalBusy = false
		if !m.systemFilesystem {
			m.busy = false
		}
		m.terminalClose = nil
		m.mu.Unlock()
	}()
	if e = m.record("terminal.start", "", struct{ Shell string }{shell}); e != nil {
		return 255, e
	}
	var env []string
	for _, k := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "HOME", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	if m.systemFilesystem {
		env = os.Environ()
	}
	e = m.authorize(epoch, "terminal", false, func() error {
		if e := m.checkOperation(admission); e != nil {
			return e
		}
		var e error
		p, e = terminal.Start(shell, m.rootPath, env, int(w), int(h))
		if e == nil {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.closed {
				return errors.New("SESSION_REVOKED")
			}
			m.terminalClose = closePTY
			m.terminalView.lifecycle("start", w, h)
		}
		return e
	})
	if e != nil {
		return 255, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, e := in.Read(buf)
			if n > 0 {
				data := append([]byte(nil), buf[:n]...)
				err := m.sendTerminalInput(epoch, p, closePTY, data)
				if err != nil {
					if err.Error() == "SESSION_PAUSED" {
						continue
					}
					closePTY()
					return
				}
			}
			if e != nil {
				closePTY()
				return
			}
		}
	}()
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-resize:
				if !ok {
					return
				}
				admission, e := m.prepareOperation(epoch, "terminal", false)
				if e != nil || m.record("terminal.resize", "", s) != nil {
					continue
				}
				if e := m.authorize(epoch, "terminal", false, func() error {
					if e := m.checkOperation(admission); e != nil {
						return e
					}
					if e := p.Resize(int(s.Width), int(s.Height)); e != nil {
						return e
					}
					m.terminalView.lifecycle("resize", s.Width, s.Height)
					return nil
				}); e != nil {
					continue
				}
			case <-tick.C: /* the enclosing runtime closes the PTY on revocation */
			}
		}
	}()
	_, e = io.Copy(&terminalOutput{m: m, out: out}, p)
	if e != nil {
		closePTY()
	}
	waitCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	code, waitErr := p.Wait(waitCtx)
	if m.record("terminal.end", "", struct{ Code int }{code}) == nil {
		m.terminalView.lifecycle("end", 0, 0)
	}
	if e != nil {
		return code, e
	}
	return code, waitErr
}

func (m *Manager) sendTerminalInput(epoch uint64, p io.Writer, closePTY func() error, data []byte) error {
	admission, err := m.prepareOperation(epoch, "terminal", false)
	if err != nil {
		return err
	}
	if err = m.record("terminal.input.intent", "", struct {
		Data string `json:"data_b64"`
	}{base64.StdEncoding.EncodeToString(data)}); err != nil {
		return err
	}
	var actual *int
	err = m.authorize(epoch, "terminal", false, func() error {
		if e := m.checkOperation(admission); e != nil {
			// Discard input staged before pause or reconnect without closing
			// the shell once an interrupted session has resumed.
			if admission.Err() != nil {
				return errors.New("SESSION_PAUSED")
			}
			return e
		}
		written, e := boundedWrite(p, data, 500*time.Millisecond)
		if !errors.Is(e, context.DeadlineExceeded) {
			actual = &written
		}
		return e
	})
	if errors.Is(err, context.DeadlineExceeded) {
		go closePTY()
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	if logErr := m.record("terminal.input.result", "", struct {
		Written *int
		Error   string
	}{actual, errText}); logErr != nil {
		return logErr
	}
	return err
}

type terminalOutput struct {
	m   *Manager
	out io.Writer
}

func (w *terminalOutput) Write(p []byte) (int, error) {
	if e := w.m.record("terminal.output", "", struct {
		Data string `json:"data_b64"`
	}{base64.StdEncoding.EncodeToString(p)}); e != nil {
		return 0, e
	}
	w.m.terminalView.output(p)
	return w.out.Write(p)
}
