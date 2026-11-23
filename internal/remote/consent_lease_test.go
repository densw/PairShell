package remote

import (
	"context"
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"net/http"
	"net/http/httptest"
	"os"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestLeaseRenewsImmediatelyWhileConsentPending(t *testing.T) {
	called := make(chan struct{}, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case called <- struct{}{}:
		default:
		}
		json.NewEncoder(w).Encode(struct {
			Expires time.Time `json:"expires_at"`
		}{time.Now().Add(30 * time.Second)})
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := session.New("s", "p", time.Now().Add(time.Hour))
	g.RenewLease(time.Now().Add(500 * time.Millisecond))
	c := &coordinator{base: srv.URL, id: "s", token: "test", client: srv.Client()}
	go c.lease(ctx, cancel, g)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("lease did not renew until after initial deadline")
	}
	if state := g.Snapshot().State; state != "AWAITING_CONSENT" {
		t.Fatal(state)
	}
	if g.Check(1, "tasks", false) == nil {
		t.Fatal("pending consent allowed execution")
	}
}

func TestCustomerCanReturnPendingGateBeforeConsent(t *testing.T) {
	srv := httptest.NewTLSServer(server.New(server.Config{CreateToken: "create"}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	m, e := Create(ctx, srv.URL, "create", srv.Client())
	if e != nil {
		t.Fatal(e)
	}
	pending := make(chan *session.Gate, 1)
	customerDone := make(chan error, 1)
	go func() {
		customerDone <- ServeCustomer(ctx, m.Invite.URL(), srv.Client(), handler{}, func(tr pairing.Transcript, _ string) (*session.Gate, error) {
			g := session.New(tr.SessionID, "peer", time.Now().Add(time.Hour))
			pending <- g
			return g, nil
		})
	}()
	clients := make(chan *ssh.Client, 1)
	merchantErrors := make(chan error, 1)
	go func() {
		c, e := m.Connect(ctx, func(pairing.Transcript, string) error { return nil })
		if e != nil {
			merchantErrors <- e
			return
		}
		clients <- c
	}()
	var g *session.Gate
	select {
	case g = <-pending:
	case <-ctx.Done():
		t.Fatal("pairing timeout")
	}
	delay := 200 * time.Millisecond
	if raw := os.Getenv("PAIRSHELL_TEST_CONSENT_DELAY"); raw != "" {
		var e error
		delay, e = time.ParseDuration(raw)
		if e != nil || delay > 45*time.Second {
			t.Fatal("invalid test delay")
		}
	}
	select {
	case e := <-customerDone:
		t.Fatal("customer exited while awaiting consent", e)
	case c := <-clients:
		c.Close()
		t.Fatal("SSH connected before customer authorization")
	case <-time.After(delay):
	}
	if state := g.Snapshot().State; state != "AWAITING_CONSENT" {
		t.Fatal("pending consent expired", state)
	}
	if g.Check(1, "tasks", false) == nil {
		t.Fatal("consent bypass")
	}
	if e = g.Consent(session.Capabilities{Tasks: true}, false); e != nil {
		t.Fatal(e)
	}
	select {
	case c := <-clients:
		c.Close()
	case e := <-merchantErrors:
		t.Fatal(e)
	case <-ctx.Done():
		t.Fatal("connection after consent timed out")
	}
	cancel()
	select {
	case <-customerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup timeout")
	}
}
