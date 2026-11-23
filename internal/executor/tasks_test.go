package executor

import (
	"crypto/sha256"
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTasksRequireAuthorizationAndDeduplicate(t *testing.T) {
	m := New(t.TempDir(), func(uint64, string, bool, func() error) error { return ErrDenied }, func(string, string, any) error { return nil })
	r := Request{RequestID: "one", Epoch: 1, Argv: []string{"cmd", "/c", "echo hello"}, TimeoutMS: 1000}
	if runtime.GOOS != "windows" {
		r.Argv = []string{"sh", "-c", "printf hello"}
	}
	defer m.Close()
	if _, e := m.Start(r); e == nil {
		t.Fatal("unauthorized start")
	}
	m.authorize = func(_ uint64, _ string, _ bool, f func() error) error { return f() }
	a, e := m.Start(r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := m.Start(r)
	if e != nil || a.ID != b.ID {
		t.Fatal("idempotency")
	}
	r.Argv = []string{"different"}
	if _, e = m.Start(r); e == nil {
		t.Fatal("conflict accepted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := m.Status(a.ID)
		if s.State != "RUNNING" && s.State != "ACCEPTED" {
			if s.State != "SUCCEEDED" || s.ExitCode == nil || *s.ExitCode != 0 || !strings.Contains(string(s.Output), "hello") {
				t.Fatalf("task failed: %+v", s)
			}
			raw, _ := json.Marshal(s)
			t.Log(string(raw))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task stuck")
}

func TestSessionTaskCountLimitPreservesIdempotentRetries(t *testing.T) {
	m := New(t.TempDir(), func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	for i := 0; i < maxRetainedTasks; i++ {
		id := strconv.Itoa(i)
		m.tasks[id] = &Task{ID: id, State: "SUCCEEDED", cancel: func() {}}
	}
	request := Request{RequestID: "0", Epoch: 1, Argv: []string{"echo"}, TimeoutMS: 1000}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	m.tasks["0"].body = sha256.Sum256(raw)
	if got, err := m.Start(request); err != nil || got.State != "SUCCEEDED" {
		t.Fatalf("idempotent retry at quota: %#v, %v", got, err)
	}
	request.RequestID = "new"
	if _, err = m.Start(request); err == nil || err.Error() != "TASK_SESSION_LIMIT" {
		t.Fatalf("unexpected admission above quota: %v", err)
	}
}

func TestSessionTaskOutputLimit(t *testing.T) {
	m := New(t.TempDir(), nil, func(string, string, any) error { return nil })
	m.retainedTaskBytes = maxRetainedTaskOutput - 1
	canceled := false
	task := &Task{ID: "t", cancel: func() { canceled = true }}
	w := outputWriter{m, task, "stdout"}
	if n, err := w.Write([]byte("a")); n != 1 || err != nil {
		t.Fatalf("first write: %d, %v", n, err)
	}
	if _, err := w.Write([]byte("b")); !canceled || err == nil || string(task.Output) != "a" {
		t.Fatalf("over-budget output retained: %s, canceled %t, error %v", task.Output, canceled, err)
	}
}

func TestFilesRejectEscapeAndCAS(t *testing.T) {
	m := New(t.TempDir(), func(_ uint64, _ string, _ bool, f func() error) error { return f() }, func(string, string, any) error { return nil })
	defer m.Close()
	if _, e := m.ReadFile(1, false, "../escape", 100); e == nil {
		t.Fatal("escape")
	}
	if e := m.WriteFile(1, false, "a.txt", []byte("hello"), ""); e != nil {
		t.Fatal(e)
	}
	if e := m.WriteFile(1, false, "a.txt", []byte("bad"), ""); e == nil {
		t.Fatal("missing precondition")
	}
	f, e := m.ReadFile(1, false, "a.txt", 100)
	if e != nil || string(f.Data) != "hello" {
		t.Fatal(e)
	}
	if e = m.WriteFile(1, false, "a.txt", []byte("next"), f.SHA256); e != nil {
		t.Fatal(e)
	}
}
