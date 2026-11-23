package remote

import (
	"context"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestEndedGateCancelsTransportWithoutWaitingForLeasePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := session.New("s", "p", time.Now().Add(time.Hour))
	go watchRevocation(ctx, cancel, g)
	g.End()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("revocation waited for control lease polling")
	}
}
