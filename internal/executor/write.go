package executor

import (
	"context"
	"io"
	"time"
)

// A blocked terminal must release the control lock so the customer can end.
func boundedWrite(w io.Writer, p []byte, limit time.Duration) (int, error) {
	type result struct {
		n int
		e error
	}
	done := make(chan result, 1)
	go func() { n, e := w.Write(p); done <- result{n, e} }()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case v := <-done:
		return v.n, v.e
	case <-timer.C:
		return 0, context.DeadlineExceeded
	}
}
