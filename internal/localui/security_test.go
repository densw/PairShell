package localui

import (
	"errors"
	"net/http/httptest"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestEndRevokesEvenWhenAuditFails(t *testing.T) {
	g := session.New("s", "peer", time.Now().Add(time.Hour))
	g.Consent(session.Capabilities{Terminal: true}, false)
	s := New(g, nil, func(string, session.Snapshot, string) error { return errors.New("disk full") })
	s.Host = "127.0.0.1:12345"
	r := httptest.NewRequest("POST", "http://"+s.Host+"/api/end", nil)
	r.Header.Set("Origin", "http://"+s.Host)
	r.Header.Set("X-PairShell-Control", s.controlToken)
	s.ServeHTTP(httptest.NewRecorder(), r)
	if g.Snapshot().State != "ENDED" {
		t.Fatal("audit failure prevented revocation")
	}
}
