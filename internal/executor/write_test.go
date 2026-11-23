package executor

import (
	"io"
	"testing"
	"time"
)

func TestTerminalWriteIsBounded(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	start := time.Now()
	if _, e := boundedWrite(w, []byte("blocked"), 10*time.Millisecond); e == nil {
		t.Fatal("blocked write accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("write did not release control")
	}
}
