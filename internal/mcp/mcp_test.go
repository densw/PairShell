package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFixedReference(t *testing.T) {
	var path, auth string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer s.Close()
	p := filepath.Join(t.TempDir(), "ref.json")
	os.WriteFile(p, []byte(`{"url":"`+s.URL+`","token":"secret"}`), 0600)
	b, e := load(p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.call(context.Background(), "session_info", struct{}{}); e != nil {
		t.Fatal(e)
	}
	if path != "/bridge/session_info" || auth != "Bearer secret" {
		t.Fatal(path, auth)
	}
}
func TestRemoteReferenceDenied(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ref")
	os.WriteFile(p, []byte(`{"url":"https://example.com","token":"secret"}`), 0600)
	if _, e := load(p); e == nil {
		t.Fatal("remote reference accepted")
	}
}
