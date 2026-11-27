package localui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"pairshell/internal/audit"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func plainAuditFile(t *testing.T, chunks [][]byte, types []string) (string, []byte) {
	t.Helper()
	var source bytes.Buffer
	for i, chunk := range chunks {
		kind := "terminal.output"
		if types != nil {
			kind = types[i]
		}
		body, err := json.Marshal(map[string]any{"sequence": i + 1, "event_type": kind, "payload": map[string]string{"data_b64": base64.StdEncoding.EncodeToString(chunk)}})
		if err != nil {
			t.Fatal(err)
		}
		line, err := json.Marshal(map[string]string{"event_body_b64": base64.StdEncoding.EncodeToString(body)})
		if err != nil {
			t.Fatal(err)
		}
		source.Write(line)
		source.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, source.Bytes()
}

func TestPlainAuditPowerShellPreservesRawReaderAndSource(t *testing.T) {
	stream := "\x1b[?9001h\x1b]0;Administrator: PowerShell\a\x1b[2J\x1b[H\x1b[KPS C:\\Users\\Administrator> 中文\r\n\t好"
	path, source := plainAuditFile(t, [][]byte{[]byte(stream), []byte("\x1b[H\x1b[K")}, nil)
	plain, err := ReadPlainAuditEvents(path, 0)
	if err != nil || len(plain) != 2 || plain[0].Data != "PS C:\\Users\\Administrator> 中文\n\t好" || plain[1].Data != "" || plain[1].Sequence != 2 {
		t.Fatalf("plain=%+v err=%v", plain, err)
	}
	raw, err := ReadAuditEvents(path, 0)
	if err != nil || raw[0].Data != stream {
		t.Fatalf("raw changed: %+v %v", raw, err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(saved, source) {
		t.Fatal("source changed", err)
	}
}

func TestPlainAuditDoesNotChangeSignedAuditOrRawReplay(t *testing.T) {
	dir := t.TempDir()
	w, err := audit.NewWriter(dir, "readable")
	if err != nil {
		t.Fatal(err)
	}
	stream := "\x1b[?9001h\x1b]0;Administrator\a\x1b[2J中文\r\n"
	payload, err := json.Marshal(map[string]string{"data_b64": base64.StdEncoding.EncodeToString([]byte(stream))})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(audit.Event{EventType: "terminal.output", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close("complete"); err != nil {
		t.Fatal(err)
	}
	names := []string{"events.jsonl", "terminal.cast", "transcript.txt", "manifest.json"}
	originals := make(map[string][]byte)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		originals[name] = data
	}
	events, err := ReadPlainAuditEvents(filepath.Join(dir, "events.jsonl"), 0)
	if err != nil || len(events) != 2 || events[0].Data != "中文\n" || events[1].Type != "session.end" || events[1].Data != `{"reason":"complete"}` {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	verified, err := audit.Verify(dir)
	if err != nil || !verified.Complete {
		t.Fatalf("audit no longer verifies: %+v %v", verified, err)
	}
	var replay bytes.Buffer
	if err := audit.Replay(dir, &replay); err != nil || replay.String() != stream {
		t.Fatalf("raw replay changed: %q %v", replay.String(), err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(data, originals[name]) {
			t.Fatal("changed", name, err)
		}
	}
}

func TestPlainAuditInputCannotPolluteOutputDecoder(t *testing.T) {
	runeBytes := []byte("中")
	path, _ := plainAuditFile(t, [][]byte{runeBytes[:1], []byte("\x1b]input-title"), runeBytes[1:], []byte("\x1b]output-title"), []byte("echo hi\r\n"), []byte("\a完成")}, []string{"terminal.output", "terminal.input.intent", "terminal.output", "terminal.output", "terminal.input.intent", "terminal.output"})
	events, err := ReadPlainAuditEvents(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Data != "" || events[1].Data != "" || events[2].Data != "中" || events[3].Data != "" || events[4].Data != "echo hi\n" || events[5].Data != "完成" {
		t.Fatalf("events=%+v", events)
	}
}

func TestPlainAuditInvalidAndIncompleteUTF8AcrossPage(t *testing.T) {
	chunks := make([][]byte, 63)
	for i := range chunks {
		chunks[i] = []byte("a")
	}
	// Event 64 ends with the first byte of 中. The next page also contains
	// an invalid byte and an unfinished rune, which must wait for future output.
	chunks = append(chunks, []byte{0xe4}, []byte{0xb8, 0xad, 0xff, 'X', 0xe5})
	path, _ := plainAuditFile(t, chunks, nil)
	first, err := ReadPlainAuditEvents(path, 0)
	if err != nil || len(first) != 64 || first[63].Sequence != 64 || first[63].Data != "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := ReadPlainAuditEvents(path, 64)
	if err != nil || len(second) != 1 || second[0].Data != "中\ufffdX" || !utf8.ValidString(second[0].Data) {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestPlainAuditTruncatesOnUTF8Boundary(t *testing.T) {
	path, _ := plainAuditFile(t, [][]byte{[]byte(strings.Repeat("a", 65535) + "中文")}, nil)
	events, err := ReadPlainAuditEvents(path, 0)
	if err != nil || len(events) != 1 || !utf8.ValidString(events[0].Data) || !strings.HasPrefix(events[0].Data, strings.Repeat("a", 65535)+" [") {
		t.Fatalf("invalid truncation: %v", err)
	}
}

func TestPlainAuditRejectsMalformedTerminalData(t *testing.T) {
	for _, payload := range []string{`{"data_b64":"!"}`, `{"data_b64":42}`} {
		body := []byte(`{"sequence":1,"event_type":"terminal.output","payload":` + payload + `}`)
		line, _ := json.Marshal(map[string]string{"event_body_b64": base64.StdEncoding.EncodeToString(body)})
		path := filepath.Join(t.TempDir(), "events")
		if err := os.WriteFile(path, append(line, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPlainAuditEvents(path, 0); err == nil {
			t.Fatal("accepted malformed terminal data", payload)
		}
	}
}

func TestPlainTerminalSkipsBoundedLongCommands(t *testing.T) {
	var terminal plainTerminal
	if got := terminal.write([]byte("\x1b]0;")); got != "" {
		t.Fatal(got)
	}
	for i := 0; i < 2000; i++ {
		if got := terminal.write([]byte(strings.Repeat("title", 100))); got != "" {
			t.Fatal("title leaked")
		}
	}
	if got := terminal.write([]byte("\x1b\\中文\x1bPprivate\x1b\\\x1b(B\x1b[31m好\x1b[0m")); got != "中文好" {
		t.Fatal(got)
	}
}
