package localui

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditEventReader(t *testing.T) {
	body := []byte(`{"sequence":1,"event_type":"terminal.output","payload":{"data_b64":"aGVsbG8="}}`)
	line, _ := json.Marshal(struct {
		Body string `json:"event_body_b64"`
	}{base64.StdEncoding.EncodeToString(body)})
	p := filepath.Join(t.TempDir(), "events")
	os.WriteFile(p, append(line, '\n'), 0600)
	v, e := ReadAuditEvents(p, 0)
	if e != nil || len(v) != 1 || v[0].Data != "hello" {
		t.Fatal(v, e)
	}
	v, e = ReadAuditEvents(p, 1)
	if e != nil || len(v) != 0 {
		t.Fatal(v, e)
	}
}
