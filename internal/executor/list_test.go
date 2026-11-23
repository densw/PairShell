package executor

import (
	"strings"
	"testing"
)

func TestCustomerTaskListBounded(t *testing.T) {
	m := New(t.TempDir(), nil, nil)
	m.tasks["one"] = &Task{ID: "one", State: "SUCCEEDED", Output: []byte(strings.Repeat("x", 70000))}
	v := m.List()
	if len(v) != 1 || len(v[0].Output) != 65536 || v[0].OutputStart != 4464 {
		t.Fatal("bad output window", v)
	}
}
