package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMerchantRouteStatusIsExplicitBeforeConnection(t *testing.T) {
	m := &merchantWindow{ctx: context.Background()}
	w := httptest.NewRecorder()
	m.serve(w, httptest.NewRequest("GET", "http://localhost/merchant/state", nil))
	var state struct {
		Route struct {
			Selected string `json:"selected"`
		} `json:"Route"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &state); e != nil {
		t.Fatal(e)
	}
	if state.Route.Selected != "connecting" {
		t.Fatalf("route=%q, want connecting", state.Route.Selected)
	}
}

func TestMerchantConfirmationRequiresMatchingCodeForCurrentAttempt(t *testing.T) {
	m := &merchantWindow{ctx: context.Background()}
	attempt := m.beginAttemptLocked()
	defer retireMerchantAttempt(attempt)
	m.state = "等待商家核对身份"
	m.code = "0123456789abcdef01234567"
	post := func(attemptID uint64, code string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"attempt_id": attemptID, "verification_code": code})
		w := httptest.NewRecorder()
		m.serve(w, httptest.NewRequest("POST", "http://localhost/merchant/confirm", strings.NewReader(string(body))))
		return w.Code
	}
	for _, tc := range []struct {
		attemptID uint64
		code      string
	}{
		{attempt.id, ""},
		{attempt.id, "0123456789abcdef01234568"},
		{attempt.id + 1, m.code},
	} {
		if status := post(tc.attemptID, tc.code); status != http.StatusConflict {
			t.Fatalf("unverified attempt got HTTP %d", status)
		}
		select {
		case <-attempt.confirm:
			t.Fatal("unverified attempt released confirmation")
		default:
		}
	}
	if status := post(attempt.id, strings.ToUpper(m.code)); status != http.StatusOK {
		t.Fatalf("verified attempt got HTTP %d", status)
	}
	select {
	case <-attempt.confirm:
	default:
		t.Fatal("matching code did not release current confirmation")
	}
}

func TestMerchantRequiresWindowToken(t *testing.T) {
	d := &desktopServer{host: "127.0.0.1:1234", secret: "private", merchant: &merchantWindow{ctx: context.Background()}}
	for _, path := range []string{"/merchant/state", "/merchant/create", "/merchant/exec"} {
		r := httptest.NewRequest("POST", "http://"+d.host+path, strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(path, w.Code)
		}
	}
}
func TestMerchantCannotExecuteBeforeConnection(t *testing.T) {
	m := &merchantWindow{ctx: context.Background()}
	r := httptest.NewRequest("POST", "http://localhost/merchant/exec", strings.NewReader(`{"Command":"echo unexpected"}`))
	w := httptest.NewRecorder()
	m.serve(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
