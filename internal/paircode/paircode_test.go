package paircode

import (
	"context"
	"net/http/httptest"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"testing"
	"time"
)

func TestLegacyPublishedCodeStaysEndToEndEncrypted(t *testing.T) {
	s := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true, CreateToken: "admin"}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	invite, _, err := pairing.NewInvite(s.URL, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	serverHandler := server.New(server.Config{PublicDevicePairing: true, CreateToken: "admin"})
	credentials, err := serverHandler.Create("admin", invite.SessionID, "key")
	if err != nil {
		t.Fatal(err)
	}
	s.Config.Handler = serverHandler
	code, done, err := Publish(ctx, s.URL, invite.SessionID, credentials.CreatorToken, invite.URL(), s.Client())
	if err != nil {
		t.Fatal(err)
	}
	received, err := Receive(ctx, s.URL, code, s.Client())
	if err != nil || received != invite.URL() {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestMalformedPAKEAndCodeDoNotPanic(t *testing.T) {
	for _, code := range []string{"123456", "https://example/", "123456-12AA-5678"} {
		if _, _, err := parse(code); err == nil {
			t.Fatal("invalid code", code)
		}
	}
	p, err := newPAKE("12345678", 0, "https://example", "device")
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range [][]byte{[]byte("null"), []byte(`{"Role":1}`), []byte(`[]`)} {
		if err = update(p, wire); err == nil {
			t.Fatal("malformed PAKE accepted")
		}
	}
}

func TestDeviceCodeExchangesInviteAndRejectsReuse(t *testing.T) {
	s := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true, CreateToken: "admin"}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, done, e := Listen(ctx, s.URL, s.Client())
	if e != nil {
		t.Fatal(e)
	}
	invite, _, e := pairing.NewInvite(s.URL, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if e = Send(ctx, s.URL, code, invite.URL(), s.Client()); e != nil {
		t.Fatal(e)
	}
	result := <-done
	if result.Err != nil || result.Invite != invite.URL() {
		t.Fatal(result)
	}
	if e = Send(ctx, s.URL, code, invite.URL(), s.Client()); e == nil {
		t.Fatal("reused code")
	}
}
func TestWrongPasswordNeverRevealsInvite(t *testing.T) {
	s := httptest.NewTLSServer(server.New(server.Config{PublicDevicePairing: true}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	code, done, e := Listen(ctx, s.URL, s.Client())
	if e != nil {
		t.Fatal(e)
	}
	wrong := []byte(code)
	if wrong[len(wrong)-1] == '0' {
		wrong[len(wrong)-1] = '1'
	} else {
		wrong[len(wrong)-1] = '0'
	}
	invite, _, _ := pairing.NewInvite(s.URL, time.Minute)
	if e = Send(ctx, s.URL, string(wrong), invite.URL(), s.Client()); e == nil {
		t.Fatal("wrong password accepted")
	}
	result := <-done
	if result.Err == nil || result.Invite != "" {
		t.Fatal("invite leaked", result)
	}
}
