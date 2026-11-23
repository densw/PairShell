// Package mcp binds an official MCP stdio server to one protected local session reference.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Reference struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}
type bridge struct {
	ref    Reference
	client *http.Client
}

func load(path string) (*bridge, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var r Reference
	d := json.NewDecoder(io.LimitReader(f, 4097))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return nil, errors.New("invalid session reference")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid session reference")
	}
	u, e := url.Parse(r.URL)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Port() == "" || r.Token == "" || len(r.Token) > 1024 {
		return nil, errors.New("invalid session reference")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("session reference must be loopback")
	}
	return &bridge{r, &http.Client{Timeout: 65 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }}}, nil
}
func (b *bridge) call(ctx context.Context, name string, args any) (string, error) {
	raw, e := json.Marshal(args)
	if e != nil {
		return "", e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", b.ref.URL+"/bridge/"+name, bytes.NewReader(raw))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.ref.Token)
	res, e := b.client.Do(req)
	if e != nil {
		return "", errors.New("local session unavailable")
	}
	defer res.Body.Close()
	body, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if e != nil {
		return "", e
	}
	if len(body) >= 2<<20 {
		return "", errors.New("response too large")
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("session rejected call (%d): %s", res.StatusCode, string(body))
	}
	return string(body), nil
}

type Empty struct{}
type TaskStart struct {
	RequestID string   `json:"request_id"`
	Epoch     uint64   `json:"authorization_epoch"`
	Shell     string   `json:"shell,omitempty"`
	Command   string   `json:"command,omitempty"`
	Argv      []string `json:"argv,omitempty"`
	Cwd       string   `json:"cwd"`
	TimeoutMS int64    `json:"timeout_ms"`
}
type TaskID struct {
	ID string `json:"id"`
}
type Output struct {
	ID     string `json:"id"`
	Cursor int64  `json:"cursor"`
	Limit  int    `json:"limit"`
}
type FileRead struct {
	Epoch uint64 `json:"authorization_epoch"`
	Path  string `json:"path"`
	Limit int64  `json:"limit"`
}
type FileWrite struct {
	Epoch    uint64 `json:"authorization_epoch"`
	Path     string `json:"path"`
	Data     []byte `json:"data"`
	Expected string `json:"expected"`
}

func add[T any](s *sdk.Server, b *bridge, name, description string, validate func(T) error) {
	sdk.AddTool(s, &sdk.Tool{Name: name, Description: description}, func(ctx context.Context, req *sdk.CallToolRequest, args T) (*sdk.CallToolResult, any, error) {
		if validate != nil {
			if e := validate(args); e != nil {
				return nil, nil, e
			}
		}
		text, e := b.call(ctx, name, args)
		if e != nil {
			return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: e.Error()}}}, nil, nil
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, nil, nil
	})
}
func Run(ctx context.Context, referencePath string) error {
	b, e := load(referencePath)
	if e != nil {
		return e
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "pairshell", Version: "1.1.0"}, nil)
	add[Empty](s, b, "session_info", "Inspect this session's target, consent, connection and audit status", nil)
	add[TaskStart](s, b, "task_start", "Run on the remote machine using its account and environment. Omit shell to use the remote default shell. cwd may be an absolute remote directory; relative cwd uses session_info.working_directory. Never automatically retry after disconnect", nil)
	add[TaskID](s, b, "task_status", "Query an existing task's status", nil)
	add[TaskID](s, b, "task_cancel", "Request cancellation; query status to confirm termination", nil)
	add[Output](s, b, "task_output", "Read bounded output at a byte cursor", func(a Output) error {
		if a.Cursor < 0 || a.Limit < 1 || a.Limit > 65536 {
			return errors.New("output limit must be 1..65536 and cursor nonnegative")
		}
		return nil
	})
	add[FileRead](s, b, "file_read", "Read a remote file (up to 1 MiB). Absolute paths use the remote filesystem; relative paths use session_info.working_directory. Requires active customer authorization", func(a FileRead) error {
		if a.Limit < 1 || a.Limit > 1<<20 {
			return errors.New("file limit must be 1..1048576")
		}
		return nil
	})
	add[FileWrite](s, b, "file_write", "Write a remote file (up to 1 MiB) at an absolute path or relative to session_info.working_directory. Supply the previous-content SHA-256, or empty digest for a new file. Remote OS permissions apply", func(a FileWrite) error {
		if len(a.Data) > 1<<20 {
			return errors.New("file too large")
		}
		return nil
	})
	return s.Run(ctx, &sdk.StdioTransport{})
}
