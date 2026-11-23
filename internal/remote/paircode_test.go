package remote

import (
	"context"
	"net/http/httptest"
	"pairshell/internal/paircode"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestCustomerDeviceCodeConnectsWithoutDeploymentToken(t *testing.T) {
	s := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true, CreateToken: "private-admin"}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	code, result, err := paircode.Listen(ctx, s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		r := <-result
		if r.Err != nil {
			done <- r.Err
			return
		}
		done <- ServeCustomer(ctx, r.Invite, s.Client(), handler{}, func(tr pairing.Transcript, _ string) (*session.Gate, error) {
			g := session.New(tr.SessionID, "merchant", time.Now().Add(time.Minute))
			return g, g.Consent(session.Capabilities{Terminal: true, Tasks: true}, false)
		})
	}()
	m, err := ConnectCode(ctx, s.URL, code, "", s.Client())
	if err != nil {
		t.Fatal(err)
	}
	client, err := m.Connect(ctx, func(pairing.Transcript, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	shell, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	out, err := shell.Output("device-code")
	if err != nil || string(out) != "remote:device-code" {
		t.Fatal(string(out), err)
	}
	shell.Close()
	client.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("customer didn't close")
	}
}
