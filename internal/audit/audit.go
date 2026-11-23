package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"pairshell/internal/securefile"
	"path/filepath"
	"sync"
	"time"
)

const MaxEventBytes = 1 << 20
const maxSessionLogBytes = 32 << 20

var ErrLogCapacity = errors.New("AUDIT_LOG_CAPACITY")

type Event struct {
	SchemaVersion      int             `json:"schema_version"`
	SessionID          string          `json:"session_id"`
	Sequence           uint64          `json:"sequence"`
	WallTimeUTC        string          `json:"wall_time_utc"`
	MonotonicElapsedNS int64           `json:"monotonic_elapsed_ns"`
	Actor              string          `json:"actor"`
	EventType          string          `json:"event_type"`
	RequestID          string          `json:"request_id"`
	AuthorizationEpoch uint64          `json:"authorization_epoch"`
	Payload            json.RawMessage `json:"payload"`
}
type record struct {
	Body     string `json:"event_body_b64"`
	Previous string `json:"previous_hash"`
	Hash     string `json:"event_hash"`
}
type Writer struct {
	mu      sync.Mutex
	file    *os.File
	dir, id string
	start   time.Time
	seq     uint64
	bytes   int64
	head    [32]byte
	failed  error
	closed  bool
}
type Verification struct {
	SessionID string `json:"session_id"`
	Events    uint64 `json:"events"`
	ChainHead string `json:"chain_head"`
	Complete  bool   `json:"complete"`
}
type Manifest struct {
	Version        int    `json:"version"`
	SessionID      string `json:"session_id"`
	Events         uint64 `json:"events"`
	ChainHead      string `json:"chain_head"`
	Reason         string `json:"reason"`
	EventsHash     string `json:"events_sha256"`
	CastHash       string `json:"cast_sha256"`
	TranscriptHash string `json:"transcript_sha256"`
}

func hashEvent(prev [32]byte, b []byte) string {
	h := sha256.New()
	h.Write([]byte("PairShell:event:v1\x00"))
	h.Write(prev[:])
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func validPayload(b []byte) bool {
	d := json.NewDecoder(bytesReader(b))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return false
	}
	var check func(any) bool
	check = func(x any) bool {
		switch z := x.(type) {
		case json.Number:
			_, e := z.Int64()
			return e == nil
		case []any:
			for _, a := range z {
				if !check(a) {
					return false
				}
			}
		case map[string]any:
			for _, a := range z {
				if !check(a) {
					return false
				}
			}
		}
		return true
	}
	return check(v)
}
func NewWriter(dir, id string) (*Writer, error) {
	if id == "" {
		return nil, errors.New("empty session")
	}
	if e := securefile.Mkdir(dir); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return nil, e
	}
	return &Writer{file: f, dir: dir, id: id, start: time.Now()}, nil
}
func (w *Writer) Append(e Event) error { w.mu.Lock(); defer w.mu.Unlock(); return w.append(e) }
func (w *Writer) append(e Event) error {
	if w.failed != nil {
		return w.failed
	}
	if w.closed {
		return errors.New("audit closed")
	}
	if len(e.Payload) == 0 {
		e.Payload = json.RawMessage(`{}`)
	}
	if !validPayload(e.Payload) {
		return errors.New("invalid payload: integers only")
	}
	e.SchemaVersion = 1
	e.SessionID = w.id
	e.Sequence = w.seq + 1
	e.WallTimeUTC = time.Now().UTC().Format(time.RFC3339Nano)
	e.MonotonicElapsedNS = time.Since(w.start).Nanoseconds()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(b) > MaxEventBytes {
		return errors.New("event too large")
	}
	hash := hashEvent(w.head, b)
	line, err := json.Marshal(record{base64.StdEncoding.EncodeToString(b), hex.EncodeToString(w.head[:]), hash})
	if err == nil {
		limit := int64(maxSessionLogBytes)
		// Keep room to seal the existing record after the session hits its quota.
		if e.EventType != "session.end" {
			limit -= MaxEventBytes * 2
		}
		if w.bytes+int64(len(line))+1 > limit {
			return ErrLogCapacity
		}
		_, err = w.file.Write(append(line, '\n'))
	}
	if err == nil {
		err = w.file.Sync()
	}
	if err != nil {
		w.failed = fmt.Errorf("AUDIT_IO_FAILED: %w", err)
		return w.failed
	}
	w.seq++
	w.bytes += int64(len(line)) + 1
	raw, _ := hex.DecodeString(hash)
	copy(w.head[:], raw)
	return nil
}
func (w *Writer) Close(reason string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.failed
	}
	b, _ := json.Marshal(struct {
		Reason string `json:"reason"`
	}{reason})
	e := w.append(Event{Actor: "customer", EventType: "session.end", Payload: b})
	w.closed = true
	ce := w.file.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	v, events, e := readEvents(w.dir)
	if e != nil {
		return e
	}
	cast, txt, e := derive(events)
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(w.dir, "terminal.cast"), cast, 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(w.dir, "transcript.txt"), txt, 0600); e != nil {
		return e
	}
	src, e := os.ReadFile(filepath.Join(w.dir, "events.jsonl"))
	if e != nil {
		return e
	}
	m := Manifest{1, w.id, v.Events, v.ChainHead, reason, digest(src), digest(cast), digest(txt)}
	mb, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(w.dir, "manifest.json"), mb, 0600)
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func readEvents(dir string) (Verification, []Event, error) {
	var v Verification
	f, e := os.Open(filepath.Join(dir, "events.jsonl"))
	if e != nil {
		return v, nil, e
	}
	defer f.Close()
	info, statErr := f.Stat()
	if statErr != nil {
		return v, nil, statErr
	}
	if info.Size() > 256<<20 {
		return v, nil, errors.New("audit exceeds offline verification limit")
	}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), MaxEventBytes*2)
	var head [32]byte
	var events []Event
	var lastTime int64
	ended := false
	for s.Scan() {
		if ended {
			return v, nil, errors.New("events after end")
		}
		var r record
		if e = json.Unmarshal(s.Bytes(), &r); e != nil {
			return v, nil, e
		}
		b, e := base64.StdEncoding.DecodeString(r.Body)
		if e != nil || len(b) > MaxEventBytes {
			return v, nil, errors.New("invalid body")
		}
		if r.Previous != hex.EncodeToString(head[:]) || r.Hash != hashEvent(head, b) {
			return v, nil, errors.New("hash chain mismatch")
		}
		var ev Event
		if e = json.Unmarshal(b, &ev); e != nil {
			return v, nil, e
		}
		if ev.SchemaVersion != 1 || ev.Sequence != v.Events+1 || ev.SessionID == "" || !validPayload(ev.Payload) || ev.MonotonicElapsedNS < lastTime {
			return v, nil, errors.New("invalid event semantics")
		}
		if v.Events == 0 {
			v.SessionID = ev.SessionID
		}
		if ev.SessionID != v.SessionID {
			return v, nil, errors.New("session mismatch")
		}
		lastTime = ev.MonotonicElapsedNS
		v.Events++
		v.ChainHead = r.Hash
		hb, _ := hex.DecodeString(r.Hash)
		copy(head[:], hb)
		ended = ev.EventType == "session.end"
		events = append(events, ev)
	}
	if s.Err() != nil {
		return v, nil, s.Err()
	}
	return v, events, nil
}
func Verify(dir string) (Verification, error) {
	v, events, e := readEvents(dir)
	if e != nil {
		return v, e
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(e) {
		return v, nil
	}
	if e != nil {
		return v, e
	}
	var m Manifest
	if e = json.Unmarshal(b, &m); e != nil {
		return v, e
	}
	if len(events) == 0 || events[len(events)-1].EventType != "session.end" || m.Version != 1 || m.SessionID != v.SessionID || m.Events != v.Events || m.ChainHead != v.ChainHead {
		return v, errors.New("manifest mismatch")
	}
	var end struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(events[len(events)-1].Payload, &end) != nil || end.Reason != m.Reason {
		return v, errors.New("end reason mismatch")
	}
	cast, txt, e := derive(events)
	if e != nil {
		return v, e
	}
	for _, x := range []struct {
		name, hash string
		expected   []byte
	}{{"events.jsonl", m.EventsHash, nil}, {"terminal.cast", m.CastHash, cast}, {"transcript.txt", m.TranscriptHash, txt}} {
		data, e := os.ReadFile(filepath.Join(dir, x.name))
		if e != nil {
			return v, e
		}
		if digest(data) != x.hash || (x.expected != nil && string(data) != string(x.expected)) {
			return v, errors.New("derived file mismatch")
		}
	}
	v.Complete = true
	return v, nil
}
func derive(events []Event) ([]byte, []byte, error) {
	cast := []byte("{\"version\":2,\"width\":80,\"height\":24}\n")
	var txt []byte
	for _, ev := range events {
		if ev.EventType != "terminal.output" {
			continue
		}
		var p struct {
			Data string `json:"data_b64"`
		}
		if e := json.Unmarshal(ev.Payload, &p); e != nil {
			return nil, nil, e
		}
		b, e := base64.StdEncoding.DecodeString(p.Data)
		if e != nil {
			return nil, nil, e
		}
		line, _ := json.Marshal([]any{float64(ev.MonotonicElapsedNS) / 1e9, "o", string(b)})
		cast = append(cast, append(line, '\n')...)
		txt = append(txt, b...)
	}
	return cast, txt, nil
}
func Replay(dir string, out io.Writer) error {
	_, ev, e := readEvents(dir)
	if e != nil {
		return e
	}
	_, txt, e := derive(ev)
	if e != nil {
		return e
	}
	_, e = out.Write(txt)
	return e
}

type reader struct{ b []byte }

func bytesReader(b []byte) *reader { return &reader{b} }
func (r *reader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
