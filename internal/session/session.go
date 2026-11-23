package session

import (
	"context"
	"errors"
	"sync"
	"time"
)

const DefaultIdleTimeout = 10 * time.Minute

type Capabilities struct {
	Terminal  bool `json:"terminal"`
	Tasks     bool `json:"tasks"`
	FileRead  bool `json:"file_read"`
	FileWrite bool `json:"file_write"`
}
type Snapshot struct {
	SessionID          string       `json:"session_id"`
	PeerKey            string       `json:"peer_key"`
	AuthorizationEpoch uint64       `json:"authorization_epoch"`
	State              string       `json:"state"`
	Capabilities       Capabilities `json:"capabilities"`
	AI                 bool         `json:"ai"`
	ExpiresAt          time.Time    `json:"expires_at"`
}
type Gate struct {
	mu            sync.Mutex
	s             Snapshot
	previous      string
	lease         time.Time
	auditFailed   bool
	idleTimeout   time.Duration
	lastActivity  time.Time
	stagedContext context.Context
	cancelStaged  context.CancelFunc
}

func New(id, peer string, expires time.Time) *Gate {
	return NewWithIdleTimeout(id, peer, expires, DefaultIdleTimeout)
}

func NewWithIdleTimeout(id, peer string, expires time.Time, timeout time.Duration) *Gate {
	if timeout <= 0 {
		timeout = DefaultIdleTimeout
	}
	return &Gate{
		s:           Snapshot{SessionID: id, PeerKey: peer, AuthorizationEpoch: 1, State: "AWAITING_CONSENT", ExpiresAt: expires},
		lease:       time.Now().Add(30 * time.Second),
		idleTimeout: timeout,
	}
}
func (g *Gate) Snapshot() Snapshot { g.mu.Lock(); defer g.mu.Unlock(); g.expire(); return g.s }
func (g *Gate) expire() {
	now := time.Now()
	if !now.Before(g.s.ExpiresAt) || !now.Before(g.lease) ||
		(!g.lastActivity.IsZero() && now.Sub(g.lastActivity) >= g.idleTimeout) {
		g.end()
	}
}
func (g *Gate) RenewLease(until time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.s.State == "ENDED" {
		return errors.New("SESSION_REVOKED")
	}
	if until.After(time.Now().Add(30 * time.Second)) {
		until = time.Now().Add(30 * time.Second)
	}
	g.lease = until
	return nil
}
func (g *Gate) Consent(c Capabilities, ai bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.auditFailed {
		return errors.New("AUDIT_IO_FAILED")
	}
	if g.s.State != "AWAITING_CONSENT" {
		return errors.New("CONSENT_REQUIRED")
	}
	g.s.Capabilities = c
	g.s.AI = ai
	g.s.State = "ACTIVE"
	g.lastActivity = time.Now()
	g.startStaged()
	return nil
}
func (g *Gate) Pause() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.s.State == "RECONNECTING" {
		g.previous = "PAUSED"
		return nil
	}
	if g.s.State != "ACTIVE" && g.s.State != "PAUSED" {
		return errors.New("CONSENT_REQUIRED")
	}
	g.s.State = "PAUSED"
	g.stopStaged()
	return nil
}
func (g *Gate) Resume() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.auditFailed {
		return errors.New("AUDIT_IO_FAILED")
	}
	if g.s.State != "PAUSED" {
		return errors.New("SESSION_REVOKED")
	}
	g.s.State = "ACTIVE"
	g.lastActivity = time.Now()
	g.startStaged()
	return nil
}
func (g *Gate) Reconnecting() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.s.State == "ACTIVE" || g.s.State == "PAUSED" {
		g.previous = g.s.State
		g.s.State = "RECONNECTING"
		g.stopStaged()
	}
}
func (g *Gate) Connected() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expire()
	if g.s.State == "RECONNECTING" {
		g.s.State = g.previous
		if g.s.State == "ACTIVE" && !g.auditFailed {
			g.startStaged()
		}
	}
}
func (g *Gate) End() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.end()
}
func (g *Gate) end() {
	if g.s.State != "ENDED" {
		g.s.AuthorizationEpoch++
		g.s.State = "ENDED"
		g.s.Capabilities = Capabilities{}
		g.s.AI = false
		g.stopStaged()
	}
}
func (g *Gate) AuditFailure() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auditFailed = true
	g.stopStaged()
	if g.s.State == "ACTIVE" {
		g.s.State = "PAUSED"
	}
	if g.s.State == "RECONNECTING" {
		g.previous = "PAUSED"
	}
}
func (g *Gate) check(epoch uint64, cap string, ai bool) error {
	g.expire()
	if g.auditFailed {
		return errors.New("AUDIT_IO_FAILED")
	}
	if epoch != g.s.AuthorizationEpoch || g.s.State == "ENDED" {
		return errors.New("SESSION_REVOKED")
	}
	if g.s.State == "PAUSED" {
		return errors.New("SESSION_PAUSED")
	}
	if g.s.State != "ACTIVE" {
		return errors.New("CONSENT_REQUIRED")
	}
	allowed := false
	switch cap {
	case "terminal":
		allowed = g.s.Capabilities.Terminal
	case "tasks":
		allowed = g.s.Capabilities.Tasks
	case "file_read":
		allowed = g.s.Capabilities.FileRead
	case "file_write":
		allowed = g.s.Capabilities.FileWrite
	}
	if !allowed || (ai && !g.s.AI) {
		return errors.New("CONSENT_REQUIRED")
	}
	return nil
}
func (g *Gate) Check(epoch uint64, cap string, ai bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.check(epoch, cap, ai)
}

// WithAuthorization serializes an atomic execution admission with revocation. The callback must only enqueue/start work and must not block waiting for completion.
func (g *Gate) WithAuthorization(epoch uint64, cap string, ai bool, fn func() error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e := g.check(epoch, cap, ai); e != nil {
		return e
	}
	if e := fn(); e != nil {
		return e
	}
	g.lastActivity = time.Now()
	return nil
}

// StagedAuthorization gives an operation a cancellation signal that
// remains canceled after pause/resume or reconnect. Authorization is checked
// again at completion because the operation performs I/O without the gate lock.
func (g *Gate) StagedAuthorization(epoch uint64, cap string, ai bool) (context.Context, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.check(epoch, cap, ai); err != nil {
		return nil, err
	}
	return g.stagedContext, nil
}

func (g *Gate) startStaged() {
	g.stagedContext, g.cancelStaged = context.WithCancel(context.Background())
}

func (g *Gate) stopStaged() {
	if g.cancelStaged != nil {
		g.cancelStaged()
		g.cancelStaged = nil
	}
}
