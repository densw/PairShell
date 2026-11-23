package transport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCandidateWaitHonorsConnectDeadlineWithoutSelectingPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	p, e := NewDirectPeer("timeout", 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	c := NewConn("timeout", 1, func([]byte) error { return errors.New("NO_ROUTE") })
	defer c.Close()
	selected, e := WaitCandidates(ctx, c, []*Peer{p})
	if !errors.Is(e, context.DeadlineExceeded) || selected != -1 {
		t.Fatalf("selected=%d err=%v", selected, e)
	}
}
