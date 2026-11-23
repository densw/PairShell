package localui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"pairshell/internal/executor"
	"pairshell/internal/session"
	"runtime"
	"strings"
)

// Bridge is a separate capability: it cannot invoke local consent/control routes.
type Bridge struct {
	Token string
	Gate  *session.Gate
	Tasks *executor.Manager
}

func (b Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if b.Token == "" || r.Method != "POST" || r.Header.Get("Origin") != "" || !equal(r.Header.Get("Authorization"), "Bearer "+b.Token) {
		http.Error(w, "forbidden", 403)
		return
	}
	var a struct {
		ID             string   `json:"id"`
		Expected       string   `json:"expected"`
		RequestID      string   `json:"request_id"`
		Epoch          uint64   `json:"authorization_epoch"`
		Shell          string   `json:"shell"`
		Command        string   `json:"command"`
		Argv           []string `json:"argv"`
		Cwd            string   `json:"cwd"`
		TimeoutMS      int64    `json:"timeout_ms"`
		TaskID         string   `json:"task_id"`
		Cursor         int      `json:"cursor"`
		Limit          int64    `json:"limit"`
		Path           string   `json:"path"`
		Data           []byte   `json:"data"`
		SHA256         string   `json:"sha256"`
		ExpectedSHA256 string   `json:"expected_sha256"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1500000))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	if r.URL.Path == "/bridge/task_output" && (a.Limit < 0 || a.Limit > 65536) {
		http.Error(w, "invalid output limit", 400)
		return
	}
	if a.TaskID == "" {
		a.TaskID = a.ID
	}
	if a.ExpectedSHA256 == "" {
		a.ExpectedSHA256 = a.Expected
	}
	s := b.Gate.Snapshot()
	if a.Epoch == 0 {
		a.Epoch = s.AuthorizationEpoch
	}
	tool := strings.TrimPrefix(r.URL.Path, "/bridge/")
	var out any
	var e error
	switch tool {
	case "session_info":
		out = struct {
			session.Snapshot
			OS               string `json:"os"`
			Arch             string `json:"arch"`
			WorkingDirectory string `json:"working_directory"`
			FileAccess       string `json:"file_access"`
		}{s, runtime.GOOS, runtime.GOARCH, b.Tasks.WorkingDirectory(), b.Tasks.FileAccessMode()}
	case "task_start":
		out, e = b.Tasks.Start(executor.Request{RequestID: a.RequestID, Epoch: a.Epoch, AI: true, Shell: a.Shell, Command: a.Command, Argv: a.Argv, Cwd: a.Cwd, TimeoutMS: a.TimeoutMS})
	case "task_cancel":
		e = b.Gate.WithAuthorization(a.Epoch, "tasks", true, func() error { return b.Tasks.Cancel(a.TaskID) })
		out = struct {
			Requested bool `json:"requested"`
		}{true}
	case "task_status", "task_output":
		if e = b.Gate.Check(a.Epoch, "tasks", true); e != nil {
			break
		}
		var t executor.Task
		t, e = b.Tasks.Status(a.TaskID)
		if tool == "task_output" && e == nil {
			if a.Cursor < 0 || a.Cursor > len(t.Output) {
				e = errors.New("invalid cursor")
				break
			}
			limit := int(a.Limit)
			if limit == 0 {
				limit = 65536
			}
			end := a.Cursor + limit
			if end > len(t.Output) {
				end = len(t.Output)
			}
			out = struct {
				Data  []byte `json:"data"`
				Next  int    `json:"next_cursor"`
				State string `json:"state"`
			}{t.Output[a.Cursor:end], end, t.State}
		} else {
			t.Output = nil
			out = t
		}
	case "file_read":
		if a.Limit == 0 {
			a.Limit = 65536
		}
		out, e = b.Tasks.ReadFile(a.Epoch, true, a.Path, a.Limit)
	case "file_write":
		expected := a.ExpectedSHA256
		if expected == "" {
			expected = a.SHA256
		}
		e = b.Tasks.WriteFile(a.Epoch, true, a.Path, a.Data, expected)
		out = struct {
			Written bool `json:"written"`
		}{e == nil}
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	json.NewEncoder(w).Encode(out)
}
