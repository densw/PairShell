package remote

import (
	"context"
	"pairshell/internal/session"
	"time"
)

// Local customer revocation must not wait for a slow control-server lease request.
func watchRevocation(ctx context.Context, cancel context.CancelFunc, g *session.Gate) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if g.Snapshot().State == "ENDED" {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
