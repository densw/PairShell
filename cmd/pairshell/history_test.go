package main

import (
	"net/http/httptest"
	"pairshell/internal/history"
	"strings"
	"testing"
)

func TestHistoryAPIRequiresOwnerAndDoesNotAcceptTraversal(t *testing.T) {
	h, e := history.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	d := &desktopServer{host: "127.0.0.1:1234", secret: "owner", history: h}
	for _, tc := range []struct {
		path, method, body, token, origin string
		want                              int
	}{
		{"/history", "GET", "", "", "", 403},
		{"/history/create", "POST", `{"Title":"test"}`, "owner", "https://evil.example", 403},
		{"/history/create", "POST", `{"Title":"test","Note":"hello"}`, "owner", "http://127.0.0.1:1234", 200},
		{"/history/entry?id=../outside", "GET", "", "owner", "", 404},
	} {
		r := httptest.NewRequest(tc.method, "http://"+d.host+tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-PairShell", tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s => %d", tc.path, w.Code)
		}
	}
	list, _ := h.List("test", false)
	if len(list) != 1 {
		t.Fatal("owner create failed")
	}
}
