package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http/httptest"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"pairshell/internal/sshbridge"
	"testing"
	"time"
)

type handler struct{}

func (handler) Run(cmd string, _ io.Reader, out, err io.Writer) (int, error) {
	io.WriteString(out, "remote:"+cmd)
	return 0, nil
}
func (handler) Shell(io.Reader, io.Writer, uint32, uint32, <-chan sshbridge.Size) (int, error) {
	return 0, nil
}
func TestPairPionSSH(t *testing.T) { testPairSSH(t, nil) }

func testPairSSH(t *testing.T, after func(*Merchant)) {
	t.Helper()
	srv := httptest.NewTLSServer(server.New(server.Config{CreateToken: "deploy", RoomTTL: time.Minute}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m, e := Create(ctx, srv.URL, "deploy", srv.Client())
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	customerCode := make(chan string, 1)
	go func() {
		done <- ServeCustomer(ctx, m.Invite.URL(), srv.Client(), handler{}, func(tr pairing.Transcript, code string) (*session.Gate, error) {
			customerCode <- code
			g := session.New(tr.SessionID, "merchant", time.Unix(tr.Expires, 0))
			return g, g.Consent(session.Capabilities{Terminal: true, Tasks: true}, false)
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("customer did not stop after cancellation")
		}
	})
	client, e := m.Connect(ctx, func(_ pairing.Transcript, code string) error {
		select {
		case peerCode := <-customerCode:
			if !pairing.MatchVerificationCode(code, peerCode) {
				return errors.New("endpoint verification codes do not match")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	s, e := client.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.Output("hello")
	if e != nil || string(out) != "remote:hello" {
		t.Fatalf("%q %v", out, e)
	}
	if after != nil {
		after(m)
		next, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		defer next.Close()
		output, err := next.Output("after-direct")
		if err != nil || string(output) != "remote:after-direct" {
			t.Fatalf("after path assertion: %q %v", output, err)
		}
	}
}

func TestHandshakeRejectsLegacyVerification(t *testing.T) {
	rooms := server.New(server.Config{CreateToken: "test-deploy"})
	srv := httptest.NewTLSServer(rooms)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := Create(ctx, srv.URL, "test-deploy", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(m.Invite.Join)
	if err := rooms.Claim(m.Invite.SessionID, token, base64.RawURLEncoding.EncodeToString(pub)); err != nil {
		t.Fatal(err)
	}
	legacy := coordinator{base: srv.URL, id: m.Invite.SessionID, role: "customer", token: token, secret: m.Invite.Secret, client: srv.Client()}
	if err := legacy.send(ctx, "hello", hello{Key: pub, Challenge: bytes.Repeat([]byte{7}, 32)}); err != nil {
		t.Fatal(err)
	}
	called := false
	_, _, _, err = m.ConnectStream(ctx, func(pairing.Transcript, string) error {
		called = true
		return nil
	})
	if !errors.Is(err, errVerificationVersion) || called {
		t.Fatalf("legacy verification reached approval: callback = %v, error = %v", called, err)
	}
}
