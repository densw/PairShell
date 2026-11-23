package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLegacyInviteMayReplacePendingDeviceCodeOnce(t *testing.T) {
	started := make(chan string, 1)
	release := make(chan struct{})
	defer close(release)
	canceled := false
	d := &desktopServer{host: "127.0.0.1:1234", secret: "cap", state: "正在生成配对码", listening: true, listenCancel: func() { canceled = true }, start: func(invite string) { started <- invite; <-release }}
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://"+d.host+"/start", strings.NewReader(`{"invite":"test invite"}`))
		r.Header.Set("X-PairShell", "cap")
		r.Header.Set("Origin", "http://"+d.host)
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		return w
	}
	if w := request(); w.Code != 200 {
		t.Fatal(w.Code)
	}
	select {
	case got := <-started:
		if got != "test invite" || !canceled {
			t.Fatal("not started or old listener not canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("start stuck")
	}
	if w := request(); w.Code != 409 {
		t.Fatal("second session accepted")
	}
	d.deviceError()
	if d.state != "正在核验协助人员" {
		t.Fatal("canceled listener overwrote current session")
	}
}
