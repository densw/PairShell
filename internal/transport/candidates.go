package transport

import (
	"context"
	"errors"
	"github.com/pion/webrtc/v4"
	"sort"
	"strings"
	"sync"
	"time"
)

func directServers(servers []webrtc.ICEServer) []webrtc.ICEServer {
	var out []webrtc.ICEServer
	for _, server := range servers {
		var urls []string
		for _, u := range server.URLs {
			if strings.HasPrefix(u, "stun:") || strings.HasPrefix(u, "stuns:") {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			out = append(out, webrtc.ICEServer{URLs: urls})
		}
	}
	return out
}
func HasTURN(servers []webrtc.ICEServer) bool {
	for _, server := range servers {
		for _, u := range server.URLs {
			if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
				return true
			}
		}
	}
	return false
}

// NewDirectPeer has no TURN candidates, so measured direct RTT cannot secretly be relay RTT.
func NewDirectPeer(session string, epoch uint64, servers []webrtc.ICEServer) (*Peer, error) {
	return NewPeer(session, epoch, directServers(servers), false)
}

type PathStatus struct {
	Available bool  `json:"available"`
	RTTMillis int64 `json:"rtt_ms"`
	Timeouts  int   `json:"timeouts"`
}
type RouteStatus struct {
	Selected  string     `json:"selected"`
	RTTMillis int64      `json:"rtt_ms"`
	Direct    PathStatus `json:"direct"`
	Relay     PathStatus `json:"relay"`
}

func (c *Conn) RouteStatus() RouteStatus {
	c.routeMu.RLock()
	defer c.routeMu.RUnlock()
	return c.route
}
func (c *Conn) setRouteStatus(s RouteStatus) { c.routeMu.Lock(); c.route = s; c.routeMu.Unlock() }

// WaitCandidates starts every candidate together and admits the first working one.
// It does not wait for a preferred transport; MaintainCandidates then selects by measured RTT.
func WaitCandidates(ctx context.Context, conn *Conn, peers []*Peer) (int, error) {
	type result struct {
		i int
		e error
	}
	results := make(chan result, len(peers))
	for i, p := range peers {
		if p == nil {
			results <- result{i, errors.New("unavailable")}
			continue
		}
		go func(i int, p *Peer) { _, e := p.Wait(ctx); results <- result{i, e} }(i, p)
	}
	for range peers {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case result := <-results:
			if result.e == nil {
				if e := conn.Switch(peers[result.i].Send); e != nil {
					return -1, e
				}
				return result.i, nil
			}
		}
	}
	return -1, errors.New("NO_ROUTE")
}

// MaintainCandidates compares independently authenticated direct and relay paths.
// The initial path has no priority; lower recent median RTT wins subject to stability hysteresis.
func MaintainCandidates(ctx context.Context, conn *Conn, peers []*Peer, active int, exhausted func()) {
	names := []string{"direct", "relay"}
	selector := NewSelector(names[active])
	fail := make([]int, len(peers))
	samples := make([][]time.Duration, len(peers))
	var unavailable time.Time
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		rtt := make([]time.Duration, len(peers))
		var wg sync.WaitGroup
		for i, p := range peers {
			wg.Add(1)
			go func(i int, p *Peer) {
				defer wg.Done()
				if p == nil {
					fail[i]++
					return
				}
				probe, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
				defer cancel()
				d, e := p.Probe(probe)
				if e != nil {
					fail[i]++
					return
				}
				fail[i] = 0
				samples[i] = append(samples[i], d)
				if len(samples[i]) > 10 {
					samples[i] = samples[i][1:]
				}
				sorted := append([]time.Duration(nil), samples[i]...)
				sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
				rtt[i] = sorted[len(sorted)/2]
			}(i, p)
		}
		wg.Wait()
		if len(peers) > 1 {
			other := 1 - active
			switchPath := fail[active] >= 3 && fail[other] == 0
			if fail[active] == 0 && fail[other] == 0 {
				switchPath = selector.Observe(names[other], rtt[other], rtt[active], time.Now())
			}
			if switchPath && conn.Switch(peers[other].Send) == nil {
				active = other
				selector.Current = names[active]
				unavailable = time.Time{}
			}
		}
		status := RouteStatus{Selected: names[active], RTTMillis: rtt[active].Milliseconds()}
		status.Direct = PathStatus{fail[0] == 0, rtt[0].Milliseconds(), fail[0]}
		if len(peers) > 1 {
			status.Relay = PathStatus{fail[1] == 0, rtt[1].Milliseconds(), fail[1]}
		}
		conn.setRouteStatus(status)
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
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
