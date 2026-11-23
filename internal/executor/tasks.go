package executor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"pairshell/internal/terminal"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var ErrDenied = errors.New("CONSENT_REQUIRED")

const (
	maxRetainedTasks      = 256
	maxTaskOutput         = 4 << 20
	maxRetainedTaskOutput = 32 << 20
)

type Authorize func(uint64, string, bool, func() error) error
type Record func(string, string, any) error
type Request struct {
	RequestID string   `json:"request_id"`
	Epoch     uint64   `json:"authorization_epoch"`
	AI        bool     `json:"ai"`
	Shell     string   `json:"shell,omitempty"`
	Command   string   `json:"command,omitempty"`
	Argv      []string `json:"argv,omitempty"`
	Cwd       string   `json:"cwd"`
	TimeoutMS int64    `json:"timeout_ms"`
}
type Task struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	ExitCode *int   `json:"exit_code"`
	Error    string `json:"error,omitempty"`
	Output   []byte `json:"output"`
	body     [sha256.Size]byte
	cancel   context.CancelFunc
}
type Manager struct {
	mu                  sync.Mutex
	root                *os.Root
	rootPath            string
	systemFilesystem    bool
	terminalBusy        bool
	fileBusy            bool
	stagedAuthorization func(uint64, string, bool) (context.Context, error)
	closed              bool
	authorize           Authorize
	record              Record
	taskOutputMu        sync.Mutex
	tasks               map[string]*Task
	retainedTaskBytes   int
	busy                bool
	terminalClose       func() error
	terminalView        terminalFeed
}

func New(root string, a Authorize, r Record) *Manager {
	p, _ := filepath.Abs(root)
	return &Manager{rootPath: p, authorize: a, record: r, tasks: make(map[string]*Task)}
}

// NewSystem uses the remote account's filesystem and environment. Session
// authorization is still required for every operation; it never elevates OS rights.
func NewSystem(cwd string, a Authorize, r Record) *Manager {
	m := New(cwd, a, r)
	m.systemFilesystem = true
	return m
}

func (m *Manager) WorkingDirectory() string {
	if m == nil {
		return ""
	}
	return m.rootPath
}
func (m *Manager) FileAccessMode() string {
	if m == nil {
		return ""
	}
	if m.systemFilesystem {
		return "current_user"
	}
	return "shared_directory"
}

// SetStagedAuthorization installs the session's cancelable operation permit
// before the manager is used by the SSH or file interfaces.
func (m *Manager) SetStagedAuthorization(authorize func(uint64, string, bool) (context.Context, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stagedAuthorization = authorize
}

func (m *Manager) prepareOperation(epoch uint64, capability string, ai bool) (context.Context, error) {
	m.mu.Lock()
	authorize := m.stagedAuthorization
	m.mu.Unlock()
	if authorize != nil {
		return authorize(epoch, capability, ai)
	}
	return context.Background(), m.authorize(epoch, capability, ai, func() error { return nil })
}

func (m *Manager) checkOperation(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || ctx.Err() != nil {
		return errors.New("SESSION_REVOKED")
	}
	return nil
}

func (m *Manager) Start(r Request) (Task, error) {
	if r.RequestID == "" || len(r.RequestID) > 128 || r.TimeoutMS < 1 || r.TimeoutMS > 3600000 || len(r.Command) > 65536 || len(r.Argv) > 256 {
		return Task{}, errors.New("invalid task")
	}
	if (len(r.Argv) > 0) == (r.Command != "") {
		return Task{}, errors.New("choose argv or shell text")
	}
	raw, _ := json.Marshal(r)
	body := sha256.Sum256(raw)
	admission, err := m.prepareOperation(r.Epoch, "tasks", r.AI)
	if err != nil {
		return Task{}, err
	}
	var result Task
	m.mu.Lock()
	if m.closed || admission.Err() != nil {
		m.mu.Unlock()
		return Task{}, errors.New("SESSION_REVOKED")
	}
	if old := m.tasks[r.RequestID]; old != nil {
		if old.body != body {
			m.mu.Unlock()
			return Task{}, errors.New("request ID conflict")
		}
		result = copyTask(old)
		m.mu.Unlock()
		return result, nil
	}
	if m.busy || m.fileBusy {
		m.mu.Unlock()
		return Task{}, errors.New("EXECUTION_BUSY")
	}
	if len(m.tasks) >= maxRetainedTasks {
		m.mu.Unlock()
		return Task{}, errors.New("TASK_SESSION_LIMIT")
	}
	m.busy = true
	m.mu.Unlock()
	reserved := true
	// Resolve paths and persist the audit record without holding the gate or
	// manager lock. A concurrent end can revoke the task before admission.
	defer func() {
		if reserved {
			m.mu.Lock()
			m.busy = false
			m.mu.Unlock()
		}
	}()
	cwd := m.rootPath
	if r.Cwd != "" {
		requested := r.Cwd
		if m.systemFilesystem && !filepath.IsAbs(requested) {
			requested = filepath.Join(m.rootPath, requested)
		}
		cwd, err = filepath.Abs(requested)
		if err != nil {
			return Task{}, err
		}
		if !m.systemFilesystem {
			rel, err := filepath.Rel(m.rootPath, cwd)
			if err != nil || !filepath.IsLocal(rel) {
				return Task{}, errors.New("invalid cwd")
			}
			resolved, err := filepath.EvalSymlinks(cwd)
			if err != nil {
				return Task{}, err
			}
			if resolved != cwd {
				return Task{}, errors.New("linked cwd denied")
			}
		}
	}
	if err = m.record("task.accepted", r.RequestID, r); err != nil {
		return Task{}, err
	}
	err = m.authorize(r.Epoch, "tasks", r.AI, func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed || admission.Err() != nil {
			return errors.New("SESSION_REVOKED")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.TimeoutMS)*time.Millisecond)
		t := &Task{ID: r.RequestID, State: "ACCEPTED", body: body, cancel: cancel}
		m.tasks[t.ID] = t
		reserved = false
		result = copyTask(t)
		go m.run(ctx, admission, t, r, cwd)
		return nil
	})
	return result, err
}
func copyTask(t *Task) Task {
	v := *t
	v.Output = append([]byte(nil), t.Output...)
	v.cancel = nil
	return v
}
func (m *Manager) run(ctx, admission context.Context, t *Task, r Request, cwd string) {
	defer t.cancel()
	var cmd *exec.Cmd
	if len(r.Argv) > 0 {
		cmd = exec.CommandContext(ctx, r.Argv[0], r.Argv[1:]...)
	} else {
		switch r.Shell {
		case "":
			if !m.systemFilesystem {
				m.finish(t, "FAILED", nil, "unsupported shell")
				return
			}
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", r.Command)
			} else {
				cmd = exec.CommandContext(ctx, terminal.DefaultShell(), "-c", r.Command)
			}
		case "powershell":
			cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", r.Command)
		case "cmd":
			cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", r.Command)
		case "sh", "bash", "zsh":
			cmd = exec.CommandContext(ctx, r.Shell, "-c", r.Command)
		default:
			m.finish(t, "FAILED", nil, "unsupported shell")
			return
		}
	}
	cmd.Dir = cwd
	for _, k := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "HOME", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if m.systemFilesystem {
		cmd.Env = os.Environ()
	}
	setupProcess(cmd)
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout = &outputWriter{m, t, "stdout"}
	cmd.Stderr = &outputWriter{m, t, "stderr"}
	m.mu.Lock()
	t.State = "RUNNING"
	m.mu.Unlock()
	err := m.authorize(r.Epoch, "tasks", r.AI, func() error {
		if e := m.checkOperation(admission); e != nil {
			return e
		}
		return cmd.Start()
	})
	if err == nil {
		err = cmd.Wait()
	}
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	state := "SUCCEEDED"
	msg := ""
	if err != nil {
		state = "FAILED"
		msg = err.Error()
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state = "TIMED_OUT"
		} else {
			state = "CANCELED"
		}
	}
	m.finish(t, state, &code, msg)
}
func (m *Manager) finish(t *Task, state string, code *int, msg string) {
	if e := m.record("task.finished", t.ID, struct {
		State    string
		ExitCode *int
		Error    string
	}{state, code, msg}); e != nil {
		state = "UNKNOWN"
		msg = e.Error()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t.State = state
	t.ExitCode = code
	t.Error = msg
	m.busy = false
}

type outputWriter struct {
	m      *Manager
	t      *Task
	stream string
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.m.taskOutputMu.Lock()
	defer w.m.taskOutputMu.Unlock()
	w.m.mu.Lock()
	overLimit := len(w.t.Output)+len(p) > maxTaskOutput || w.m.retainedTaskBytes+len(p) > maxRetainedTaskOutput
	w.m.mu.Unlock()
	if overLimit {
		w.t.cancel()
		return 0, errors.New("OUTPUT_LIMIT")
	}
	if e := w.m.record("task.output", w.t.ID, struct {
		Stream string
		Data   []byte
	}{w.stream, p}); e != nil {
		w.t.cancel()
		return 0, e
	}
	w.m.mu.Lock()
	w.t.Output = append(w.t.Output, p...)
	w.m.retainedTaskBytes += len(p)
	w.m.mu.Unlock()
	return len(p), nil
}
func (m *Manager) Status(id string) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return Task{}, errors.New("TASK_UNKNOWN")
	}
	return copyTask(t), nil
}
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return errors.New("TASK_UNKNOWN")
	}
	t.cancel()
	return nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	for _, t := range m.tasks {
		t.cancel()
	}
	closeTerminal := m.terminalClose
	m.terminalClose = nil
	var root *os.Root
	if !m.fileBusy {
		root, m.root = m.root, nil
	}
	m.mu.Unlock()
	if closeTerminal != nil {
		go closeTerminal()
	}
	if root != nil {
		return root.Close()
	}
	return nil
}
