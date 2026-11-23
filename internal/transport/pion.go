package transport

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pion/webrtc/v4"
	"sync"
	"time"
)

// Peer is one authenticated candidate path. SDP authentication is performed by pairing before Answer/SetAnswer.
type Peer struct {
	PC      *webrtc.PeerConnection
	Conn    *Conn
	ready   chan struct{}
	once    sync.Once
	bindMu  sync.Mutex
	dc      *webrtc.DataChannel
	session string
	epoch   uint64
	pong    chan uint64
	probeMu sync.Mutex
}

func NewPeer(session string, epoch uint64, servers []webrtc.ICEServer, relayOnly bool) (*Peer, error) {
	policy := webrtc.ICETransportPolicyAll
	if relayOnly {
		policy = webrtc.ICETransportPolicyRelay
	}
	pc, e := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: servers, ICETransportPolicy: policy})
	if e != nil {
		return nil, e
	}
	return &Peer{PC: pc, ready: make(chan struct{}), session: session, epoch: epoch, pong: make(chan uint64, 1)}, nil
}
func (p *Peer) bind(dc *webrtc.DataChannel, session string, epoch uint64) {
	p.bindMu.Lock()
	defer p.bindMu.Unlock()
	if p.dc != nil {
		dc.Close()
		return
	}
	if !dc.Ordered() || dc.MaxRetransmits() != nil || dc.MaxPacketLifeTime() != nil {
		dc.Close()
		return
	}
	p.dc = dc
	if p.Conn == nil {
		p.Conn = NewConn(session, epoch, func(b []byte) error {
			if dc.BufferedAmount() > MaxBuffer {
				return errors.New("DATACHANNEL_BACKPRESSURE")
			}
			return dc.Send(b)
		})
	}
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		var f frame
		if len(m.Data) <= MaxPayload*2 && json.Unmarshal(m.Data, &f) == nil && (f.Type == "ping" || f.Type == "pong") {
			if f.Version != 1 || f.Stream != 1 || len(f.Payload) != 0 || f.PayloadLength != 0 || f.Session != session || f.Epoch != epoch {
				p.Close()
				return
			}
			if f.Type == "ping" {
				f.Type = "pong"
				b, _ := json.Marshal(f)
				_ = dc.Send(b)
			} else {
				select {
				case p.pong <- f.Seq:
				default:
				}
			}
			return
		}
		p.bindMu.Lock()
		receiver := p.Conn
		p.bindMu.Unlock()
		if e := receiver.AcceptFrom(m.Data, dc.Send); e != nil {
			p.Close()
		}
	})
	dc.OnOpen(func() { p.once.Do(func() { close(p.ready) }) })
}

// UseConn attaches a candidate to the existing stream before the path is opened.
func (p *Peer) UseConn(conn *Conn) { p.bindMu.Lock(); defer p.bindMu.Unlock(); p.Conn = conn }
func (p *Peer) Send(b []byte) error {
	p.bindMu.Lock()
	dc := p.dc
	p.bindMu.Unlock()
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return errors.New("NO_ROUTE")
	}
	if dc.BufferedAmount() > MaxBuffer {
		return errors.New("DATACHANNEL_BACKPRESSURE")
	}
	return dc.Send(b)
}
func (p *Peer) Probe(ctx context.Context) (time.Duration, error) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	start := time.Now()
	seq := uint64(start.UnixNano())
	b, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: p.session, Epoch: p.epoch, Seq: seq, Type: "ping"})
	if e := p.Send(b); e != nil {
		return 0, e
	}
	for {
		select {
		case n := <-p.pong:
			if n == seq {
				return time.Since(start), nil
			}
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}
func (p *Peer) Offer(ctx context.Context, session string, epoch uint64) (webrtc.SessionDescription, error) {
	dc, e := p.PC.CreateDataChannel("pairshell-v1", nil)
	if e != nil {
		return webrtc.SessionDescription{}, e
	}
	p.bind(dc, session, epoch)
	s, e := p.PC.CreateOffer(nil)
	if e != nil {
		return s, e
	}
	return p.local(ctx, s)
}
func (p *Peer) Answer(ctx context.Context, session string, epoch uint64, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	p.PC.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "pairshell-v1" {
			dc.Close()
			return
		}
		p.bind(dc, session, epoch)
	})
	if e := p.PC.SetRemoteDescription(offer); e != nil {
		return webrtc.SessionDescription{}, e
	}
	s, e := p.PC.CreateAnswer(nil)
	if e != nil {
		return s, e
	}
	return p.local(ctx, s)
}
func (p *Peer) local(ctx context.Context, s webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	done := webrtc.GatheringCompletePromise(p.PC)
	if e := p.PC.SetLocalDescription(s); e != nil {
		return s, e
	}
	select {
	case <-done:
		return *p.PC.LocalDescription(), nil
	case <-ctx.Done():
		return s, ctx.Err()
	}
}
func (p *Peer) SetAnswer(answer webrtc.SessionDescription) error {
	return p.PC.SetRemoteDescription(answer)
}
func (p *Peer) Wait(ctx context.Context) (*Conn, error) {
	select {
	case <-p.ready:
		return p.Conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (p *Peer) Close() error { return p.PC.Close() }
