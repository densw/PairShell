package executor

import (
	"sort"
)

// VisibleTask provides a bounded output tail for the local customer viewer.
type VisibleTask struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	ExitCode    *int   `json:"exit_code"`
	Error       string `json:"error,omitempty"`
	Output      string `json:"output"`
	OutputStart int    `json:"output_start"`
	OutputEnd   int    `json:"output_end"`
}

func (m *Manager) List() []VisibleTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.tasks))
	for id := range m.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 32 {
		ids = ids[len(ids)-32:]
	}
	out := make([]VisibleTask, 0, len(ids))
	for _, id := range ids {
		t := m.tasks[id]
		start := len(t.Output) - 65536
		if start < 0 {
			start = 0
		}
		var code *int
		if t.ExitCode != nil {
			v := *t.ExitCode
			code = &v
		}
		out = append(out, VisibleTask{ID: id, State: t.State, ExitCode: code, Error: t.Error, Output: string(t.Output[start:]), OutputStart: start, OutputEnd: len(t.Output)})
	}
	return out
}
