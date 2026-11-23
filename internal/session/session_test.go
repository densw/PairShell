package session

import (
	"testing"
	"time"
)

func TestGatePauseAndRevoke(t *testing.T) {
	g := New("s", "peer", time.Now().Add(time.Hour))
	if g.Check(1, "terminal", false) == nil {
		t.Fatal("consent bypass")
	}
	if err := g.Consent(Capabilities{Terminal: true}, false); err != nil {
		t.Fatal(err)
	}
	if err := g.Pause(); err != nil {
		t.Fatal(err)
	}
	g.Reconnecting()
	g.Connected()
	if g.Check(1, "terminal", false) == nil {
		t.Fatal("reconnect resumed")
	}
	g.End()
	if g.Check(1, "terminal", false) == nil {
		t.Fatal("revoked epoch accepted")
	}
}
