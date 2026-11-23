package executor

import (
	"context"
	"time"
)

func (m *Manager) Wait(ctx context.Context) error {
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		m.mu.Lock()
		busy := m.busy || m.fileBusy
		m.mu.Unlock()
		if !busy {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
