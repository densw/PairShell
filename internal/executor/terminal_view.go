package executor

import "sync"

const (
	terminalViewBytes  = 512 << 10
	terminalViewEvents = 1024
	terminalPageBytes  = 128 << 10
	terminalPageEvents = 256
	terminalChunkSize  = 32 << 10
)

// TerminalEvent contains only PTY display output and its display lifecycle.
// Input intents are excluded: PTY echo decides what is visible.
type TerminalEvent struct {
	Sequence uint64 `json:"sequence"`
	Type     string `json:"type"`
	Data     []byte `json:"data,omitempty"`
	Width    uint32 `json:"width,omitempty"`
	Height   uint32 `json:"height,omitempty"`
}

type TerminalView struct {
	Events    []TerminalEvent `json:"events"`
	Cursor    uint64          `json:"cursor"`
	Truncated bool            `json:"truncated"`
	Width     uint32          `json:"width"`
	Height    uint32          `json:"height"`
}

// terminalFeed is independent from Manager's execution lock. Slow viewers
// read bounded copies and cannot hold execution or resize admission locks.
type terminalFeed struct {
	mu            sync.Mutex
	events        [terminalViewEvents]TerminalEvent
	head, size    int
	bytes         int
	next          uint64
	width, height uint32
}

func (f *terminalFeed) appendLocked(event TerminalEvent) {
	for f.size > 0 && (f.size == terminalViewEvents || f.bytes+len(event.Data) > terminalViewBytes) {
		f.bytes -= len(f.events[f.head].Data)
		f.events[f.head] = TerminalEvent{}
		f.head = (f.head + 1) % terminalViewEvents
		f.size--
	}
	f.next++
	event.Sequence = f.next
	f.events[(f.head+f.size)%terminalViewEvents] = event
	f.size++
	f.bytes += len(event.Data)
}

func (f *terminalFeed) lifecycle(kind string, width, height uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if kind == "start" || kind == "resize" {
		f.width, f.height = width, height
	}
	f.appendLocked(TerminalEvent{Type: kind, Width: width, Height: height})
}

func (f *terminalFeed) output(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(data) > 0 {
		n := min(len(data), terminalChunkSize)
		f.appendLocked(TerminalEvent{Type: "output", Data: append([]byte(nil), data[:n]...)})
		data = data[n:]
	}
}

// TerminalView returns a read-only, bounded page of the local display stream.
// Cursor advances only through returned events. Truncated marks expired or
// invalid display cursors; complete output remains in the session audit.
func (m *Manager) TerminalView(after uint64) TerminalView {
	f := &m.terminalView
	f.mu.Lock()
	defer f.mu.Unlock()
	v := TerminalView{Events: []TerminalEvent{}, Cursor: after, Width: f.width, Height: f.height}
	if after > f.next {
		v.Truncated = true
		after = 0
		v.Cursor = 0
	}
	if f.size > 0 && f.events[f.head].Sequence > 1 && after < f.events[f.head].Sequence-1 {
		v.Truncated = true
	}
	bytes := 0
	for i := 0; i < f.size; i++ {
		event := f.events[(f.head+i)%terminalViewEvents]
		if event.Sequence <= after {
			continue
		}
		if len(v.Events) >= terminalPageEvents || bytes+len(event.Data) > terminalPageBytes {
			break
		}
		event.Data = append([]byte(nil), event.Data...)
		v.Events = append(v.Events, event)
		v.Cursor = event.Sequence
		bytes += len(event.Data)
	}
	return v
}
