package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"pairshell/internal/history"
	"pairshell/internal/localui"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryReadsPlainOutputAcrossPagesWithoutChangingAudit(t *testing.T) {
	h, err := history.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.Root, "session-readable")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// A title sequence and a Chinese rune both cross the 64-event page boundary.
	stream := strings.Repeat("x", 63) + "中文" + "\x1b]0;" + strings.Repeat("private title", 10) + "\x07\x1b[31m完成\x1b[0m\r\n<script>alert(1)</script>\t好\x1bPignored\x1b\\!"
	var original bytes.Buffer
	for i, b := range []byte(stream) {
		body, _ := json.Marshal(map[string]any{"sequence": i + 1, "event_type": "terminal.output", "payload": map[string]string{"data_b64": base64.StdEncoding.EncodeToString([]byte{b})}})
		line, _ := json.Marshal(map[string]string{"event_body_b64": base64.StdEncoding.EncodeToString(body)})
		original.Write(line)
		original.WriteByte('\n')
	}
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	d := &desktopServer{host: "127.0.0.1:1234", secret: "owner", history: h}
	var after uint64
	var text strings.Builder
	for page := 0; page < 10; page++ {
		r := httptest.NewRequest("GET", fmt.Sprintf("http://%s/history/events?id=session-readable&after=%d", d.host, after), nil)
		r.Header.Set("X-PairShell", "owner")
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var events []localui.Event
		if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			break
		}
		if len(events) > 64 {
			t.Fatal("unbounded page", len(events))
		}
		for _, event := range events {
			if event.Sequence <= after {
				t.Fatal("cursor did not advance", event)
			}
			after = event.Sequence
			text.WriteString(event.Data)
		}
	}
	want := strings.Repeat("x", 63) + "中文完成\n<script>alert(1)</script>\t好!"
	if text.String() != want {
		t.Fatalf("plain history = %q, want %q", text.String(), want)
	}
	if after != uint64(len([]byte(stream))) {
		t.Fatal("empty control events lost cursor", after)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(saved, original.Bytes()) {
		t.Fatal("history changed original audit", err)
	}
}
