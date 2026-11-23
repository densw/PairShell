package transport

import (
	"errors"
	"testing"
	"time"
)

func TestWriteWaitsForReplacementPath(t *testing.T) {
	started := make(chan struct{}, 1)
	c := NewConn("s", 1, func([]byte) error {
		select {
		case started <- struct{}{}:
		default:
		}
		return errors.New("route down")
	})
	defer c.Close()
	done := make(chan error, 1)
	go func() { _, e := c.Write([]byte("once")); done <- e }()
	<-started
	received := NewConn("s", 1, func([]byte) error { return nil })
	defer received.Close()
	if e := c.Switch(received.Accept); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("write did not resume")
	}
	b := make([]byte, 4)
	received.Read(b)
	if string(b) != "once" {
		t.Fatal(string(b))
	}
	received.SetReadDeadline(time.Now().Add(time.Millisecond))
	if _, e := received.Read(b); e == nil {
		t.Fatal("duplicate")
	}
}
