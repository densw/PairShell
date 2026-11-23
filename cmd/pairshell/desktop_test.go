package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopRequiresLocalOriginAndToken(t *testing.T) {
	d := &desktopServer{host: "127.0.0.1:12345", secret: "secret", state: "等待邀请"}
	for _, path := range []string{"/status", "/start"} {
		r := httptest.NewRequest("POST", "http://"+d.host+path, strings.NewReader(`{"invite":"secret invite"}`))
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}
func TestDesktopDoesNotExposeControlWithoutToken(t *testing.T) {
	d := &desktopServer{host: "127.0.0.1:12345", secret: "secret", control: "sensitive"}
	r := httptest.NewRequest("GET", "http://"+d.host+"/status", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "sensitive") {
		t.Fatal("leaked control")
	}
}
