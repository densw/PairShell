package remote

import (
	"context"
	"errors"
	"github.com/pion/webrtc/v4"
	"pairshell/internal/transport"
	"sync"
	"time"
)

type candidateDescription struct {
	Kind string                     `json:"kind"`
	SDP  *webrtc.SessionDescription `json:"sdp,omitempty"`
}

// candidatePaths gathers both path types concurrently and exchanges one authenticated bundle.
func (c *coordinator) candidatePaths(ctx context.Context, stop context.CancelFunc) (*transport.Conn, []*transport.Peer, error) {
	servers := c.servers(ctx)
	count := 1
	if transport.HasTURN(servers) {
		count = 2
	}
	peers := make([]*transport.Peer, count)
	shared := transport.NewConn(c.id, 1, func([]byte) error { return errors.New("NO_ROUTE") })
	// Signaling includes the time a customer spends reviewing the consent UI.
	// Keep that budget separate from bounded ICE gathering on either endpoint.
	signaling, cancelSignaling := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelSignaling()
	newPeer := func(i int) (*transport.Peer, error) {
		if i == 0 {
			return transport.NewDirectPeer(c.id, 1, servers)
		}
		return transport.NewPeer(c.id, 1, servers, true)
	}
	descriptions := make([]candidateDescription, count)
	kinds := []string{"direct", "relay"}
	var wg sync.WaitGroup
	if c.role == "merchant" {
		for i := range peers {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				descriptions[i].Kind = kinds[i]
				p, e := newPeer(i)
				if e != nil {
					return
				}
				peers[i] = p
				p.UseConn(shared)
				gather, cancelGather := context.WithTimeout(ctx, 15*time.Second)
				defer cancelGather()
				offer, e := p.Offer(gather, c.id, 1)
				if e == nil {
					descriptions[i].SDP = &offer
				}
			}(i)
		}
		wg.Wait()
		if e := c.send(signaling, "candidate-offers", descriptions); e != nil {
			closeCandidates(peers)
			return nil, nil, e
		}
		var answers []candidateDescription
		if e := c.receive(signaling, "candidate-answers", &answers); e != nil {
			closeCandidates(peers)
			return nil, nil, e
		}
		if len(answers) != count {
			closeCandidates(peers)
			return nil, nil, errors.New("invalid candidate count")
		}
		for i, a := range answers {
			if a.Kind != kinds[i] {
				closeCandidates(peers)
				return nil, nil, errors.New("candidate binding mismatch")
			}
			if a.SDP != nil && peers[i] != nil {
				if e := peers[i].SetAnswer(*a.SDP); e != nil {
					peers[i].Close()
					peers[i] = nil
				}
			} else {
				if peers[i] != nil {
					peers[i].Close()
				}
				peers[i] = nil
			}
		}
	} else {
		var offers []candidateDescription
		if e := c.receive(signaling, "candidate-offers", &offers); e != nil {
			return nil, nil, e
		}
		if len(offers) < 1 || len(offers) > 2 {
			return nil, nil, errors.New("invalid candidate count")
		}
		count = len(offers)
		peers = make([]*transport.Peer, count)
		descriptions = make([]candidateDescription, count)
		for i, offer := range offers {
			if offer.Kind != kinds[i] {
				return nil, nil, errors.New("candidate binding mismatch")
			}
			wg.Add(1)
			go func(i int, offer candidateDescription) {
				defer wg.Done()
				descriptions[i].Kind = kinds[i]
				if offer.SDP == nil {
					return
				}
				p, e := newPeer(i)
				if e != nil {
					return
				}
				peers[i] = p
				p.UseConn(shared)
				gather, cancelGather := context.WithTimeout(ctx, 15*time.Second)
				defer cancelGather()
				answer, e := p.Answer(gather, c.id, 1, *offer.SDP)
				if e == nil {
					descriptions[i].SDP = &answer
				} else {
					p.Close()
					peers[i] = nil
				}
			}(i, offer)
		}
		wg.Wait()
		if e := c.send(signaling, "candidate-answers", descriptions); e != nil {
			closeCandidates(peers)
			return nil, nil, e
		}
	}
	// An unreachable pair must not inherit the two-hour session lifetime.
	connectCtx, stopConnect := context.WithTimeout(ctx, 30*time.Second)
	initial, e := transport.WaitCandidates(connectCtx, shared, peers)
	stopConnect()
	if e != nil {
		closeCandidates(peers)
		return nil, nil, e
	}
	c.route.Store(shared)
	go transport.MaintainCandidates(ctx, shared, peers, initial, stop)
	return shared, peers, nil
}
func closeCandidates(peers []*transport.Peer) {
	for _, p := range peers {
		if p != nil {
			p.Close()
		}
	}
}

// RouteStatus reports the local sending path; the opposite direction can choose independently.
func (m *Merchant) RouteStatus() transport.RouteStatus {
	if c := m.c.route.Load(); c != nil {
		return c.RouteStatus()
	}
	return transport.RouteStatus{Selected: "connecting"}
}
