package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func codeCall(t *testing.T, s *Server, path, token string, v any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(v)
	r := httptest.NewRequest("POST", path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}
func TestPairCodeSingleAttemptAndCreatorBinding(t *testing.T) {
	s := New(Config{CreateToken: "create"})
	c, e := s.Create("create", "0123456789abcdefghijkl", "key")
	if e != nil {
		t.Fatal(e)
	}
	req := map[string]any{"session_id": c.SessionID, "data": map[string]any{"message": []byte("public PAKE")}}
	if n, _ := codeCall(t, s, "/v1/code/create", "wrong", req); n == 200 {
		t.Fatal("creator bypass")
	}
	n, v := codeCall(t, s, "/v1/code/create", c.CreatorToken, req)
	if n != 200 {
		t.Fatal(n, v)
	}
	locator := v["locator"].(string)
	claim := map[string]any{"data": map[string]any{"locator": locator}}
	if n, _ = codeCall(t, s, "/v1/code/claim", "", claim); n != 200 {
		t.Fatal(n)
	}
	if n, _ = codeCall(t, s, "/v1/code/claim", "", claim); n == 200 {
		t.Fatal("second attempt accepted")
	}
	s.mu.Lock()
	s.rooms[c.SessionID].pairCode.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if n, _ = codeCall(t, s, "/v1/code/claim", "", claim); n == 200 {
		t.Fatal("expired code accepted")
	}
}
