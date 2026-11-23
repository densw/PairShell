package localui

import (
	"net/http/httptest"
	"pairshell/internal/executor"
	"pairshell/internal/session"
	"strings"
	"testing"
	"time"
)

func TestBridgeRejectsInvalidOutputLimitAndTrailingData(t *testing.T) {
	g := session.New("s", "p", time.Now().Add(time.Hour))
	g.Consent(session.Capabilities{Tasks: true}, true)
	m := executor.New(t.TempDir(), g.WithAuthorization, func(string, string, any) error { return nil })
	defer m.Close()
	b := Bridge{Token: "secret", Gate: g, Tasks: m}
	for _, raw := range []string{`{"id":"unknown","limit":-1}`, `{"id":"unknown","limit":65537}`, `{"id":"unknown","limit":10} {}`} {
		r := httptest.NewRequest("POST", "http://localhost/bridge/task_output", strings.NewReader(raw))
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("body %s: status %d", raw, w.Code)
		}
	}
}
