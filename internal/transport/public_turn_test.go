package transport

import (
	"context"
	"github.com/pion/webrtc/v4"
	"io"
	"os"
	"testing"
	"time"
)

func TestPublicTURN(t *testing.T) {
	url := os.Getenv("PAIRSHELL_TEST_TURN_URL")
	if url == "" {
		t.Skip("public TURN environment not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cfg := []webrtc.ICEServer{{URLs: []string{url}, Username: os.Getenv("PAIRSHELL_TEST_TURN_USER"), Credential: os.Getenv("PAIRSHELL_TEST_TURN_PASS")}}
	a, e := NewPeer("turn-check", 1, cfg, true)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := NewPeer("turn-check", 1, cfg, true)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	offer, e := a.Offer(ctx, "turn-check", 1)
	if e != nil {
		t.Fatal(e)
	}
	answer, e := b.Answer(ctx, "turn-check", 1, offer)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SetAnswer(answer); e != nil {
		t.Fatal(e)
	}
	ac, e := a.Wait(ctx)
	if e != nil {
		t.Fatal(e)
	}
	bc, e := b.Wait(ctx)
	if e != nil {
		t.Fatal(e)
	}
	ac.SetDeadline(time.Now().Add(10 * time.Second))
	bc.SetDeadline(time.Now().Add(10 * time.Second))
	go ac.Write([]byte("verified-turn"))
	out := make([]byte, 13)
	if _, e = io.ReadFull(bc, out); e != nil || string(out) != "verified-turn" {
		t.Fatal(string(out), e)
	}
	t.Log("relay-only Pion byte stream passed")
}
