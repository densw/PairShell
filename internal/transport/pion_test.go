package transport

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestPionLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a, e := NewPeer("s", 1, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := NewPeer("s", 1, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	offer, e := a.Offer(ctx, "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	answer, e := b.Answer(ctx, "s", 1, offer)
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
	ac.SetDeadline(time.Now().Add(time.Second))
	bc.SetDeadline(time.Now().Add(time.Second))
	if _, e := ac.Write([]byte("hello")); e != nil {
		t.Fatal(e)
	}
	p := make([]byte, 5)
	if _, e := io.ReadFull(bc, p); e != nil || string(p) != "hello" {
		t.Fatal(string(p), e)
	}
}
