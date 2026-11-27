package localui

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"unicode/utf8"
)

// ReadAuditEvents reads bounded pages from the authoritative event source. This viewer does not claim integrity verification; use pairshell verify for that.
func ReadAuditEvents(path string, after uint64) ([]Event, error) {
	return readAuditEvents(path, after, false)
}

// ReadPlainAuditEvents renders a read-only transcript for human-facing logs.
// It rebuilds terminal decoder state from the prefix before the cursor, because
// UTF-8 characters and terminal commands can span arbitrary events and pages.
// Raw audit, replay and terminal transport must continue using their raw paths.
func ReadPlainAuditEvents(path string, after uint64) ([]Event, error) {
	return readAuditEvents(path, after, true)
}

func readAuditEvents(path string, after uint64, plain bool) ([]Event, error) {
	out := []Event{}
	var terminal plainTerminal
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
		if !plain && body.Sequence <= after {
			continue
		}
		data := string(body.Payload)
		var payload struct {
			DataB64 string `json:"data_b64"`
		}
		if body.Type == "terminal.output" || body.Type == "terminal.input.intent" {
			payloadErr := json.Unmarshal(body.Payload, &payload)
			if plain && payloadErr != nil {
				return nil, payloadErr
			}
			if payloadErr == nil && (payload.DataB64 != "" || plain) {
				b, e := base64.StdEncoding.DecodeString(payload.DataB64)
				if e != nil {
					return nil, e
				}
				data = string(b)
				if plain {
					if body.Type == "terminal.output" {
						data = terminal.write(b)
					} else {
						// Input intent is a distinct stream and must not finish or
						// alter an output character or terminal command.
						var input plainTerminal
						data = input.write(b)
					}
				}
			}
		}
		if body.Sequence <= after {
			continue
		}
		if len(data) > 65536 {
			end := 65536
			if plain {
				for end > 0 && !utf8.RuneStart(data[end]) {
					end--
				}
			}
			data = data[:end] + " [显示已截断；原始数据保留在审计文件]"
		}
		total += len(data)
		out = append(out, Event{body.Sequence, body.Type, data})
		if len(out) >= 64 || total >= 1<<20 {
			break
		}
	}
	return out, scan.Err()
}
