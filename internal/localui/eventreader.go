package localui

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
)

// ReadAuditEvents reads bounded pages from the authoritative event source. This viewer does not claim integrity verification; use pairshell verify for that.
func ReadAuditEvents(path string, after uint64) ([]Event, error) {
	out := []Event{}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 2<<20)
	total := 0
	for scan.Scan() {
		var r struct {
			Body string `json:"event_body_b64"`
		}
		if e = json.Unmarshal(scan.Bytes(), &r); e != nil {
			continue
		}
		raw, e := base64.StdEncoding.DecodeString(r.Body)
		if e != nil {
			return nil, errors.New("invalid audit event")
		}
		var body struct {
			Sequence uint64          `json:"sequence"`
			Type     string          `json:"event_type"`
			Payload  json.RawMessage `json:"payload"`
		}
		if e = json.Unmarshal(raw, &body); e != nil {
			return nil, e
		}
		if body.Sequence <= after {
			continue
		}
		data := string(body.Payload)
		var payload struct {
			DataB64 string `json:"data_b64"`
		}
		if body.Type == "terminal.output" || body.Type == "terminal.input.intent" {
			if json.Unmarshal(body.Payload, &payload) == nil && payload.DataB64 != "" {
				b, e := base64.StdEncoding.DecodeString(payload.DataB64)
				if e != nil {
					return nil, e
				}
				data = string(b)
			}
		}
		if len(data) > 65536 {
			data = data[:65536] + " [显示已截断；原始数据保留在审计文件]"
		}
		total += len(data)
		out = append(out, Event{body.Sequence, body.Type, data})
		if len(out) >= 64 || total >= 1<<20 {
			break
		}
	}
	return out, scan.Err()
}
