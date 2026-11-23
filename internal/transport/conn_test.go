package transport

import (
	"bytes"
	"testing"
	"time"
)

func TestReliableDuplicateAndMigration(t *testing.T) {
	var frames [][]byte
	c := NewConn("test", 1, func(p []byte) error { frames = append(frames, append([]byte(nil), p...)); return nil })
	defer c.Close()
	r := NewConn("test", 1, func([]byte) error { return nil })
	defer r.Close()
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := r.Accept(frames[0]); err != nil {
		t.Fatal(err)
	}
	if err := r.Accept(frames[0]); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 5)
	if n, e := r.Read(b); e != nil || n != 5 || !bytes.Equal(b, []byte("hello")) {
		t.Fatalf("%d %v %q", n, e, b)
	}
	r.SetReadDeadline(time.Now().Add(time.Millisecond))
	if _, e := r.Read(b); e == nil {
		t.Fatal("duplicate injected")
	}
	count := 0
	if err := c.Switch(func([]byte) error { count++; return nil }); err != nil || count != 1 {
		t.Fatalf("replay unacked %d %v", count, err)
	}
}
func TestRejectEpoch(t *testing.T) {
	var f []byte
	c := NewConn("s", 1, func(p []byte) error { f = p; return nil })
	defer c.Close()
	c.Write([]byte("x"))
	r := NewConn("s", 2, func([]byte) error { return nil })
	defer r.Close()
	if r.Accept(f) == nil {
		t.Fatal("old epoch accepted")
	}
}
func TestPathHysteresis(t *testing.T) {
	s := NewSelector("a")
	now := time.Now()
	for i := 0; i < 2; i++ {
		if s.Observe("b", 20*time.Millisecond, 100*time.Millisecond, now) {
			t.Fatal("switched early")
		}
	}
	if !s.Observe("b", 20*time.Millisecond, 100*time.Millisecond, now) {
		t.Fatal("no switch")
	}
}
