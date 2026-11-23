package transport

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestPionSharedStreamMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a, _ := NewPeer("s", 1, nil, false)
	defer a.Close()
	b, _ := NewPeer("s", 1, nil, false)
	defer b.Close()
	offer, e := a.Offer(ctx, "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	answer, e := b.Answer(ctx, "s", 1, offer)
	if e != nil {
		t.Fatal(e)
	}
	a.SetAnswer(answer)
	ac, e := a.Wait(ctx)
	if e != nil {
		t.Fatal(e)
	}
	bc, e := b.Wait(ctx)
	if e != nil {
		t.Fatal(e)
	}
	a2, _ := NewPeer("s", 1, nil, false)
	defer a2.Close()
	b2, _ := NewPeer("s", 1, nil, false)
	defer b2.Close()
	a2.UseConn(ac)
	b2.UseConn(bc)
	offer, e = a2.Offer(ctx, "s", 1)
	if e != nil {
		t.Fatal(e)
	}
	answer, e = b2.Answer(ctx, "s", 1, offer)
	if e != nil {
		t.Fatal(e)
	}
	a2.SetAnswer(answer)
	if _, e = a2.Wait(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = b2.Wait(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = a2.Probe(ctx); e != nil {
		t.Fatal(e)
	}
	ac.SetDeadline(time.Now().Add(12 * time.Second))
	bc.SetDeadline(time.Now().Add(12 * time.Second))
	ac.Write([]byte("before"))
	go MaintainPaths(ctx, ac, a, a2, cancel)
	a.Close()
	b.Close()
	ac.Write([]byte("after"))
	out := make([]byte, 11)
	if _, e = io.ReadFull(bc, out); e != nil || string(out) != "beforeafter" {
		t.Fatal(string(out), e)
	}
}
