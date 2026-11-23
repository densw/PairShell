// Package transport implements a bounded migration-capable byte stream.
package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

const MaxPayload = 16 * 1024
const MaxBuffer = 1024 * 1024

type frame struct {
	Stream        uint32 `json:"stream_id"`
	Version       int    `json:"version"`
	Session       string `json:"session_id"`
	Epoch         uint64 `json:"authorization_epoch"`
	Seq           uint64 `json:"seq"`
	Ack           uint64 `json:"ack"`
	Type          string `json:"type"`
	Payload       []byte `json:"payload,omitempty"`
	PayloadLength int    `json:"payload_length"`
}
type Conn struct {
	routeMu           sync.RWMutex
	route             RouteStatus
	mu                sync.Mutex
	writeMu           sync.Mutex
	pathMu            sync.Mutex
	session           string
	epoch             uint64
	send              func([]byte) error
	next, received    uint64
	pending           map[uint64][]byte
	reorder           map[uint64][]byte
	buffer            bytes.Buffer
	queued            int
	closed            bool
	closeAck          chan struct{}
	closeAcknowledged bool
	changed           chan struct{}
	rd, wd            time.Time
}

func NewConn(session string, epoch uint64, send func([]byte) error) *Conn {
	return &Conn{session: session, epoch: epoch, send: send, next: 1, pending: map[uint64][]byte{}, reorder: map[uint64][]byte{}, changed: make(chan struct{}), closeAck: make(chan struct{})}
}
func (c *Conn) signal() { close(c.changed); c.changed = make(chan struct{}) }
func (c *Conn) Accept(p []byte) error {
	return c.AcceptFrom(p, nil)
}

// AcceptFrom returns acknowledgements on the ingress path, even before the reverse direction migrates.
func (c *Conn) AcceptFrom(p []byte, acknowledgementSender func([]byte) error) error {
	if len(p) > MaxPayload*2 {
		return errors.New("FRAME_TOO_LARGE")
	}
	var f frame
	if e := json.Unmarshal(p, &f); e != nil {
		return e
	}
	c.mu.Lock()
	if f.Version != 1 || f.Stream != 1 || f.PayloadLength != len(f.Payload) || f.Session != c.session || f.Epoch != c.epoch || len(f.Payload) > MaxPayload {
		c.mu.Unlock()
		return errors.New("FRAME_BINDING_MISMATCH")
	}
	if f.Type == "close_ack" {
		if !c.closeAcknowledged {
			c.closeAcknowledged = true
			close(c.closeAck)
		}
		c.mu.Unlock()
		return nil
	}
	if c.closed {
		// A final ACK/data frame may already be in flight while close_ack is
		// pending. Do not let it tear down the DataChannel before the close
		// handshake arrives; discard it without delivering any new bytes.
		if f.Type == "ack" || f.Type == "data" {
			c.mu.Unlock()
			return nil
		}
		// Simultaneous close still requires acknowledging the peer's close.
		if f.Type != "close" {
			c.mu.Unlock()
			return net.ErrClosed
		}
	}
	if f.Ack >= c.next {
		c.mu.Unlock()
		return errors.New("INVALID_ACK")
	}
	for seq := range c.pending {
		if seq <= f.Ack {
			delete(c.pending, seq)
		}
	}
	c.signal()
	if f.Type == "ack" {
		c.mu.Unlock()
		return nil
	}
	if f.Type == "close" {
		ack, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: c.session, Epoch: c.epoch, Type: "close_ack"})
		send := c.send
		if acknowledgementSender != nil {
			send = acknowledgementSender
		}
		c.mu.Unlock()
		_ = send(ack)
		c.mu.Lock()
		c.closed = true
		c.signal()
		c.mu.Unlock()
		return nil
	}

	if f.Type != "data" || f.Seq == 0 {
		c.mu.Unlock()
		return errors.New("INVALID_FRAME")
	}
	if f.Seq > c.received {
		if _, ok := c.reorder[f.Seq]; !ok {
			if c.queued+len(f.Payload) > MaxBuffer || len(c.reorder) >= MaxBuffer/MaxPayload {
				c.mu.Unlock()
				return errors.New("RECEIVE_BUFFER_EXHAUSTED")
			}
			c.reorder[f.Seq] = f.Payload
			c.queued += len(f.Payload)
		}
		for {
			b, ok := c.reorder[c.received+1]
			if !ok {
				break
			}
			c.buffer.Write(b)
			delete(c.reorder, c.received+1)
			c.received++
		}
		c.signal()
	}
	ack, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: c.session, Epoch: c.epoch, Ack: c.received, Type: "ack"})
	send := c.send
	if acknowledgementSender != nil {
		send = acknowledgementSender
	}
	c.mu.Unlock()
	return send(ack)
}
func wait(ch <-chan struct{}, deadline time.Time) error {
	if deadline.IsZero() {
		<-ch
		return nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		return os.ErrDeadlineExceeded
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
		return nil
	case <-timer.C:
		return os.ErrDeadlineExceeded
	}
}
func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		c.mu.Lock()
		if c.buffer.Len() > 0 {
			n, _ := c.buffer.Read(p)
			c.queued -= n
			c.signal()
			c.mu.Unlock()
			return n, nil
		}
		if c.closed {
			c.mu.Unlock()
			return 0, io.EOF
		}
		ch, d := c.changed, c.rd
		c.mu.Unlock()
		if e := wait(ch, d); e != nil {
			return 0, e
		}
	}
}
func (c *Conn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	written := 0
	for len(p) > 0 {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return written, net.ErrClosed
		}
		if !c.wd.IsZero() && !time.Now().Before(c.wd) {
			c.mu.Unlock()
			return written, os.ErrDeadlineExceeded
		}
		if len(c.pending) >= MaxBuffer/MaxPayload {
			ch, d := c.changed, c.wd
			c.mu.Unlock()
			if e := wait(ch, d); e != nil {
				return written, e
			}
			continue
		}
		n := len(p)
		if n > MaxPayload {
			n = MaxPayload
		}
		f := frame{Stream: 1, Version: 1, PayloadLength: n, Session: c.session, Epoch: c.epoch, Seq: c.next, Ack: c.received, Type: "data", Payload: append([]byte(nil), p[:n]...)}
		b, _ := json.Marshal(f)
		c.pending[c.next] = b
		c.next++
		send := c.send
		c.mu.Unlock()
		recoveryDeadline := time.Now().Add(30 * time.Second)
		for {
			if e := send(b); e == nil {
				break
			}
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				return written, net.ErrClosed
			}
			d := c.wd
			if d.IsZero() || d.After(recoveryDeadline) {
				d = recoveryDeadline
			}
			send = c.send
			_, unacked := c.pending[f.Seq]
			c.mu.Unlock()
			if !unacked {
				break
			}
			if !time.Now().Before(d) {
				return written, os.ErrDeadlineExceeded
			}
			time.Sleep(20 * time.Millisecond)
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

// Switch replaces the sending path and retransmits only unacknowledged frames.
func (c *Conn) Switch(send func([]byte) error) error {
	c.pathMu.Lock()
	defer c.pathMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return net.ErrClosed
	}
	c.send = send
	var frames [][]byte
	keys := make([]uint64, 0, len(c.pending))
	for seq := range c.pending {
		keys = append(keys, seq)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, seq := range keys {
		frames = append(frames, c.pending[seq])
	}
	c.mu.Unlock()
	for _, b := range frames {
		if e := send(b); e != nil {
			return e
		}
	}
	return nil
}
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.signal()
	b, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: c.session, Epoch: c.epoch, Type: "close"})
	send := c.send
	c.mu.Unlock()
	return send(b)
}

type addr string

func (a addr) Network() string       { return "pairshell" }
func (a addr) String() string        { return string(a) }
func (c *Conn) LocalAddr() net.Addr  { return addr("local") }
func (c *Conn) RemoteAddr() net.Addr { return addr(c.session) }
func (c *Conn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rd = t
	c.wd = t
	c.signal()
	return nil
}
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rd = t
	c.signal()
	return nil
}
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wd = t
	c.signal()
	return nil
}

type Selector struct {
	Current   string
	candidate string
	streak    int
	last      time.Time
}

func NewSelector(current string) *Selector { return &Selector{Current: current} }
func (s *Selector) Observe(path string, rtt, current time.Duration, now time.Time) bool {
	if path == s.Current || rtt <= 0 || rtt >= current {
		s.streak = 0
		return false
	}
	if path != s.candidate {
		s.candidate = path
		s.streak = 0
	}
	s.streak++
	if s.streak < 3 || (!s.last.IsZero() && now.Sub(s.last) < 15*time.Second) {
		return false
	}
	s.Current = path
	s.last = now
	s.streak = 0
	return true
}

// GracefulClose waits briefly for peer acknowledgement before the path is torn down.
func (c *Conn) GracefulClose() error {
	e := c.Close()
	if e != nil {
		return e
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-c.closeAck:
		return nil
	case <-timer.C:
		return nil
	}
}
