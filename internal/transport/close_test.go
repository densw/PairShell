package transport

import (
	"io"
	"testing"
	"time"
)

func TestGracefulCloseReachesPeerBeforePathTeardown(t *testing.T) {
	var a, b *Conn
	a = NewConn("close-test", 1, func(p []byte) error { return b.Accept(p) })
	b = NewConn("close-test", 1, func(p []byte) error { return a.Accept(p) })
	start := time.Now()
	if e := a.GracefulClose(); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("close acknowledgement not observed")
	}
	if _, e := b.Read(make([]byte, 1)); e != io.EOF {
		t.Fatal("peer not closed", e)
	}
}
