package transport

import (
	"context"
	"sort"
	"time"
)

// MaintainPaths measures at most two authenticated candidates and migrates the same byte stream.
// Each direction chooses its own sending path; the shared stream deduplicates late old-path frames.
func MaintainPaths(ctx context.Context, conn *Conn, primary, backup *Peer, exhausted func()) {
	peers := []*Peer{primary, backup}
	selector := NewSelector("primary")
	names := []string{"primary", "relay"}
	active := 0
	fail := [2]int{}
	samples := [2][]time.Duration{}
	var unavailable time.Time
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var rtt [2]time.Duration
		for i, p := range peers {
			probe, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
			d, e := p.Probe(probe)
			cancel()
			if e != nil {
				fail[i]++
				continue
			}
			fail[i] = 0
			samples[i] = append(samples[i], d)
			if len(samples[i]) > 10 {
				samples[i] = samples[i][1:]
			}
			sorted := append([]time.Duration(nil), samples[i]...)
			sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
			rtt[i] = sorted[len(sorted)/2]
		}
		other := 1 - active
		switchPath := false
		if fail[active] >= 3 && fail[other] == 0 {
			switchPath = true
			selector.Current = names[other]
		} else if fail[active] == 0 && fail[other] == 0 && selector.Observe(names[other], rtt[other], rtt[active], time.Now()) {
			switchPath = true
		}
		if switchPath {
			if conn.Switch(peers[other].Send) == nil {
				active = other
				unavailable = time.Time{}
			}
		}
		if fail[active] >= 3 {
			if unavailable.IsZero() {
				unavailable = time.Now()
			}
			if time.Since(unavailable) >= 30*time.Second {
				exhausted()
				return
			}
		} else {
			unavailable = time.Time{}
		}
	}
}
