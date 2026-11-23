package mcp

import (
	"context"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSDKRejectsTargetOverride(t *testing.T) {
	var calls atomic.Int32
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte(`{}`)) }))
	defer h.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &bridge{Reference{h.URL, "token"}, h.Client()}
	s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	add[Empty](s, b, "session_info", "info", nil)
	ct, st := sdk.NewInMemoryTransports()
	ss, e := s.Connect(ctx, st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, e := c.Connect(ctx, ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	res, e := cs.CallTool(ctx, &sdk.CallToolParams{Name: "session_info", Arguments: map[string]any{"session_id": "another-client"}})
	if e == nil && !res.IsError {
		t.Fatal("target override accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid call reached bridge")
	}
	res, e = cs.CallTool(ctx, &sdk.CallToolParams{Name: "session_info", Arguments: map[string]any{}})
	if e != nil || res.IsError || calls.Load() != 1 {
		t.Fatalf("valid call failed error=%v result=%+v calls=%d", e,res,calls.Load())
	}
}

