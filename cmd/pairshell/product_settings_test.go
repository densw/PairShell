package main

import (
	"net/http/httptest"
	"testing"
)

func TestSettingsRequireWindowCapability(t *testing.T) {
	d := &desktopServer{host: "127.0.0.1:1234", secret: "secret"}
	r := httptest.NewRequest("GET", "http://"+d.host+"/settings", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
