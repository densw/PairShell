package localui

import (
	"encoding/json"
	"net/http/httptest"
	"pairshell/internal/executor"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestTerminalViewerRequiresLocalAuthenticatedReadOnlyRequest(t *testing.T) {
	g := session.New("viewer-session", "peer", time.Now().Add(time.Hour))
	s := New(g, nil, nil)
	s.Host = "127.0.0.1:12345"
	for _, tc := range []struct {
		name, method, path, origin string
		authorized                 bool
		want                       int
	}{
		{"no control token", "GET", "/api/terminal", "", false, 403},
		{"foreign origin", "GET", "/api/terminal", "https://evil.example", true, 403},
		{"authenticated view", "GET", "/api/terminal?after=0", "", true, 200},
		{"invalid cursor", "GET", "/api/terminal?after=-1", "", true, 400},
		{"no input endpoint", "POST", "/api/terminal", "http://127.0.0.1:12345", true, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://"+s.Host+tc.path, nil)
			r.Header.Set("Origin", tc.origin)
			if tc.authorized {
				r.Header.Set("X-PairShell-Control", s.controlToken)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if tc.want == 200 {
				var view executor.TerminalView
				if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Events == nil || len(view.Events) != 0 {
					t.Fatalf("invalid empty terminal view: %s (%v)", w.Body.String(), err)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("terminal contents may be cached")
				}
			}
		})
	}
	if g.Snapshot().State != "AWAITING_CONSENT" {
		t.Fatal("view requests changed authorization")
	}
}
