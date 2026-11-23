package localui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/session"
	"strings"
	"testing"
	"time"
)

func TestUIRejectsForeignHostAndUnauthenticatedControl(t *testing.T) {
	g := session.New("s", "peer", time.Now().Add(time.Hour))
	s := New(g, nil, nil)
	s.Host = "127.0.0.1:12345"
	for _, tc := range []struct{ host, path, method string }{{"evil.test", "/api/session", "GET"}, {s.Host, "/api/end", "POST"}} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			t.Fatal("accepted untrusted request")
		}
	}
}

func TestControlBootstrapUsesPerSessionHeader(t *testing.T) {
	g := session.New("s", "peer", time.Now().Add(time.Hour))
	s := New(g, nil, nil)
	s.Host = "127.0.0.1:12345"
	origin := "http://" + s.Host
	bootstrap := func(from string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", origin+"/api/bootstrap", nil)
		r.Header.Set("Origin", from)
		r.Header.Set("X-Bootstrap", s.Bootstrap)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := bootstrap("http://127.0.0.1:12346"); w.Code != http.StatusForbidden {
		t.Fatal("foreign origin could bootstrap control")
	}
	w := bootstrap(origin)
	var auth struct{ Token string }
	if w.Code != http.StatusOK || w.Header().Get("Set-Cookie") != "" || json.Unmarshal(w.Body.Bytes(), &auth) != nil || auth.Token == "" {
		t.Fatal("bootstrap did not issue a header-only control token")
	}
	if w := bootstrap(origin); w.Code != http.StatusForbidden {
		t.Fatal("bootstrap was reusable")
	}
	for _, useHeader := range []bool{false, true} {
		r := httptest.NewRequest("GET", origin+"/api/session", nil)
		r.AddCookie(&http.Cookie{Name: "pairshell", Value: auth.Token})
		want := http.StatusForbidden
		if useHeader {
			r.Header.Set("X-PairShell-Control", auth.Token)
			want = http.StatusOK
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != want || strings.Contains(w.Body.String(), auth.Token) {
			t.Fatalf("unexpected session response status %d", w.Code)
		}
	}
	other := New(g, nil, nil)
	other.Host = "127.0.0.1:12346"
	r := httptest.NewRequest("GET", "http://"+other.Host+"/api/session", nil)
	r.Header.Set("X-PairShell-Control", auth.Token)
	w = httptest.NewRecorder()
	other.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("control token worked in another session")
	}
}

func TestConsentRequiresExplicitDesktopShareAndAuditsEffectiveDecision(t *testing.T) {
	g := session.New("s", "peer", time.Now().Add(time.Hour))
	s := New(g, nil, nil)
	s.Host = "127.0.0.1:12345"
	s.VerificationCode = "0123456789abcdef01234567"
	origin := "http://" + s.Host
	s.SelectFileRoot = func(path string) (string, error) {
		if path == "selected" {
			return "/private/share", nil
		}
		return "", fmt.Errorf("invalid folder")
	}
	var events []string
	s.record = func(action string, snap session.Snapshot, root string) error {
		events = append(events, action)
		if root != "/private/share" || !snap.AI || !snap.Capabilities.FileRead {
			t.Fatalf("invalid audited consent state: %+v", snap)
		}
		return nil
	}
	consent := func(body string) int {
		r := httptest.NewRequest("POST", origin+"/api/consent", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-PairShell-Control", s.controlToken)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	if got := consent(`{"capabilities":{"file_read":true},"ai":true,"file_root":"selected"}`); got != http.StatusConflict || len(events) != 0 || g.Snapshot().State != "AWAITING_CONSENT" {
		t.Fatalf("missing verification code changed decision: HTTP %d, events %v", got, events)
	}
	if got := consent(`{"capabilities":{"file_read":true},"ai":true,"file_root":"selected","verification_code":"0123456789abcdef01234568"}`); got != http.StatusConflict || len(events) != 0 || g.Snapshot().State != "AWAITING_CONSENT" {
		t.Fatalf("wrong verification code changed decision: HTTP %d, events %v", got, events)
	}
	if got := consent(`{"capabilities":{"file_read":true},"ai":true,"verification_code":"0123456789abcdef01234567"}`); got != http.StatusBadRequest || len(events) != 0 || g.Snapshot().State != "AWAITING_CONSENT" {
		t.Fatalf("missing scope changed decision: HTTP %d, events %v", got, events)
	}
	if got := consent(`{"capabilities":{"file_read":true},"ai":true,"file_root":"selected","verification_code":"0123456789ABCDEF01234567"}`); got != http.StatusOK || strings.Join(events, ",") != "consent_requested,consent" || g.Snapshot().State != "ACTIVE" {
		t.Fatalf("selected scope could not be authorized: HTTP %d, events %v", got, events)
	}
}
