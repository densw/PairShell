// Package remote wires authenticated pairing, coordination, Pion and pinned SSH.
package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/pion/webrtc/v4"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"net/http"
	"os"
	"pairshell/internal/config"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"pairshell/internal/sshbridge"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type coordinator struct {
	clock                 pairing.ServerClock
	route                 atomic.Pointer[routeSource]
	useTailcat            bool
	base, token, id, role string
	client                *http.Client
	secret                []byte
	sequence              uint64
	auth                  pairing.SignalAuthenticator
	queue                 []envelope
	mu                    sync.Mutex
}
type envelope struct {
	Signal pairing.Signal `json:"signal"`
	MAC    []byte         `json:"mac"`
}
type hello struct {
	Key                 ed25519.PublicKey `json:"key"`
	Challenge           []byte            `json:"challenge"`
	VerificationVersion int               `json:"verification_version"`
	Transport           string            `json:"transport,omitempty"`
}

const verificationVersion = 2

var errVerificationVersion = errors.New("对端版本不支持安全会话校验，请更新两端客户端")

type Merchant struct {
	Invite pairing.Invite
	key    ed25519.PrivateKey
	c      *coordinator
}

func (c *coordinator) call(ctx context.Context, path string, data any, out any) error {
	raw, e := json.Marshal(struct {
		ID   string `json:"session_id"`
		Key  string `json:"key,omitempty"`
		Data any    `json:"data,omitempty"`
	}{c.id, "", data})
	if e != nil {
		return e
	}
	return c.request(ctx, path, raw, out)
}
func (c *coordinator) request(ctx context.Context, path string, body []byte, out any) error {
	req, e := http.NewRequestWithContext(ctx, "POST", c.base+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	sent := time.Now()
	res, e := c.client.Do(req)
	if e != nil {
		return errors.New("coordination connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("coordination rejected (%d)", res.StatusCode)
	}
	c.clock.ObserveResponse(res.Header, sent)
	if out == nil {
		_, e = io.Copy(io.Discard, io.LimitReader(res.Body, 262145))
		return e
	}
	err := json.NewDecoder(io.LimitReader(res.Body, 262145)).Decode(out)
	if reply, ok := out.(*leaseReply); ok && err == nil {
		reply.serverTime, _ = pairing.ResponseTime(res.Header)
		if precise, preciseErr := time.Parse(time.RFC3339Nano, res.Header.Get("X-PairShell-Time")); preciseErr != nil || precise.IsZero() {
			reply.uncertainty = time.Second
		}
	}
	return err
}
func Create(ctx context.Context, base, deploy string, client *http.Client) (*Merchant, error) {
	i, key, e := pairing.NewInvite(base, 10*time.Minute)
	if e != nil {
		return nil, e
	}
	if client == nil {
		client, e = config.HTTPClient(base)
		if e != nil {
			return nil, e
		}
	}
	c := &coordinator{base: strings.TrimRight(base, "/"), id: i.SessionID, token: deploy, client: client, role: "merchant", secret: i.Secret}
	raw, _ := json.Marshal(struct {
		ID  string `json:"session_id"`
		Key string `json:"key"`
	}{i.SessionID, base64.RawURLEncoding.EncodeToString(i.MerchantKey)})
	var creds server.RoomCredentials
	if e = c.request(ctx, "/v1/rooms", raw, &creds); e != nil {
		return nil, e
	}
	i.Join, e = base64.RawURLEncoding.DecodeString(creds.JoinToken)
	if e != nil {
		return nil, e
	}
	expires := creds.ExpiresAt
	if limit := c.clock.Now().Add(10 * time.Minute); expires.After(limit) {
		expires = limit
	}
	if !expires.After(c.clock.Now()) {
		return nil, errors.New("INVITE_EXPIRED")
	}
	i.Expires = expires.Unix()
	c.token = creds.CreatorToken
	c.auth = pairing.SignalAuthenticator{Secret: i.Secret, SessionID: i.SessionID, PeerRole: "customer", Epoch: 1}
	return &Merchant{i, key, c}, nil
}
func (c *coordinator) send(ctx context.Context, kind string, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, e := json.Marshal(value)
	if e != nil {
		return e
	}
	c.sequence++
	s := pairing.Signal{Version: 1, SessionID: c.id, Role: c.role, Epoch: 1, Sequence: c.sequence, Type: kind, Payload: payload}
	mac, e := pairing.SignSignal(c.secret, s)
	if e != nil {
		return e
	}
	return c.call(ctx, "/v1/signal", envelope{s, mac}, nil)
}
func (c *coordinator) receive(ctx context.Context, kind string, out any) error {
	for {
		if len(c.queue) > 0 {
			f := c.queue[0]
			c.queue = c.queue[1:]
			if e := c.auth.Verify(f.Signal, f.MAC); e != nil {
				return e
			}
			if f.Signal.Type != kind {
				return errors.New("unexpected authenticated signal")
			}
			return json.Unmarshal(f.Signal.Payload, out)
		}
		var messages []server.Message
		if e := c.call(ctx, "/v1/poll", nil, &messages); e != nil {
			return e
		}
		for _, m := range messages {
			var f envelope
			if m.Role != c.auth.PeerRole || json.Unmarshal(m.Data, &f) != nil {
				return errors.New("PAIRING_MISMATCH")
			}
			c.queue = append(c.queue, f)
		}
		if len(c.queue) == 0 {
			// Leave room for both peers and lease renewals behind the same NAT.
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}
func (c *coordinator) handshake(ctx context.Context, i pairing.Invite, key ed25519.PrivateKey) (pairing.Transcript, error) {
	var tr pairing.Transcript
	challenge := make([]byte, 32)
	if _, e := rand.Read(challenge); e != nil {
		return tr, e
	}
	mode := os.Getenv("PAIRSHELL_TRANSPORT")
	if mode != "" && mode != "webrtc" && mode != "tailcat" {
		return tr, errors.New("invalid PAIRSHELL_TRANSPORT")
	}
	local := hello{Key: key.Public().(ed25519.PublicKey), Challenge: challenge, VerificationVersion: verificationVersion}
	if mode == "tailcat" {
		local.Transport = "tailcat-v1"
	}
	if e := c.send(ctx, "hello", local); e != nil {
		return tr, e
	}
	var peer hello
	if e := c.receive(ctx, "hello", &peer); e != nil {
		return tr, e
	}
	c.useTailcat = local.Transport == "tailcat-v1" && peer.Transport == "tailcat-v1"
	if peer.VerificationVersion != verificationVersion {
		return tr, errVerificationVersion
	}
	if len(peer.Key) != 32 || len(peer.Challenge) != 32 {
		return tr, errors.New("PAIRING_MISMATCH")
	}
	tr = pairing.Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: i.MerchantKey, Expires: i.Expires, Capabilities: "terminal,tasks,file_read,file_write;explicit-consent"}
	if c.role == "merchant" {
		tr.CustomerKey = peer.Key
		tr.MerchantChallenge = challenge
		tr.CustomerChallenge = peer.Challenge
	} else {
		if !bytes.Equal(peer.Key, i.MerchantKey) {
			return tr, errors.New("PAIRING_MISMATCH")
		}
		tr.CustomerKey = local.Key
		tr.MerchantChallenge = peer.Challenge
		tr.CustomerChallenge = challenge
	}
	proof, e := pairing.Authenticate(i.Secret, key, c.role, tr)
	if e != nil {
		return tr, e
	}
	if e = c.send(ctx, "proof", proof); e != nil {
		return tr, e
	}
	var other pairing.Proof
	if e = c.receive(ctx, "proof", &other); e != nil {
		return tr, e
	}
	if e = pairing.VerifyAt(i.Secret, c.auth.PeerRole, tr, other, c.clock.Now()); e != nil {
		return tr, e
	}
	return tr, nil
}
func (c *coordinator) servers(ctx context.Context) []webrtc.ICEServer {
	var turn struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username"`
		Credential string   `json:"credential"`
	}
	if c.call(ctx, "/v1/turn", nil, &turn) != nil {
		return nil
	}
	out := []webrtc.ICEServer{{URLs: turn.URLs, Username: turn.Username, Credential: turn.Credential}}
	for _, u := range turn.URLs {
		if strings.HasPrefix(u, "turn:") && !strings.Contains(u, "transport=tcp") {
			stun := strings.SplitN(u, "?", 2)[0]
			stun = "stun:" + strings.TrimPrefix(stun, "turn:")
			out = append(out, webrtc.ICEServer{URLs: []string{stun}})
		}
	}
	return out
}
func (c *coordinator) lease(ctx context.Context, stop context.CancelFunc, gate *session.Gate) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	until := time.Now().Add(30 * time.Second)
	revoke := func() {
		if gate != nil {
			gate.End()
		}
		stop()
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if !time.Now().Before(until) {
			revoke()
			return
		}
		var reply leaseReply
		sent := time.Now()
		requestCtx, cancel := context.WithTimeout(ctx, min(10*time.Second, time.Until(until)))
		err := c.call(requestCtx, "/v1/lease", nil, &reply)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			until, err = reply.deadline(sent)
			if err != nil || gate != nil && gate.RenewLease(until) != nil {
				revoke()
				return
			}
		}
		if !time.Now().Before(until) || gate != nil && gate.Snapshot().State == "ENDED" {
			revoke()
			return
		}
		expiry := time.NewTimer(time.Until(until))
		select {
		case <-ctx.Done():
			expiry.Stop()
			return
		case <-ticker.C:
			expiry.Stop()
		case <-expiry.C:
			revoke()
			return
		}
	}
}

func (c *coordinator) end() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = c.call(ctx, "/v1/end", nil, nil)
}

func (c *coordinator) awaitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	own, peer := "helper-ready", "customer-consent-ready"
	var ready struct{}
	if c.role == "customer" {
		own, peer = peer, own
		if err := c.receive(ctx, peer, &ready); err != nil {
			return err
		}
	}
	if err := c.call(ctx, "/v1/ready", nil, nil); err != nil {
		return err
	}
	if err := c.send(ctx, own, struct{}{}); err != nil {
		return err
	}
	if c.role == "merchant" {
		return c.receive(ctx, peer, &ready)
	}
	return nil
}

func (m *Merchant) ConnectStream(ctx context.Context, verify func(pairing.Transcript, string) error) (net.Conn, ssh.Signer, ssh.PublicKey, error) {
	ctx, stop := context.WithCancel(ctx)
	connected := false
	defer func() {
		if !connected {
			stop()
			m.c.end()
		}
	}()
	go m.c.lease(ctx, stop, nil)
	tr, e := m.c.handshake(ctx, m.Invite, m.key)
	if e != nil {
		return nil, nil, nil, e
	}
	if verify == nil {
		return nil, nil, nil, errors.New("peer confirmation required")
	}
	code, e := pairing.VerificationCode(m.Invite.Secret, tr)
	if e != nil {
		return nil, nil, nil, e
	}
	if e = verify(tr, code); e != nil {
		return nil, nil, nil, e
	}
	if e = m.c.awaitReady(ctx); e != nil {
		return nil, nil, nil, e
	}
	conn, cleanup, e := m.c.openTransport(ctx, stop)
	if e != nil {
		return nil, nil, nil, e
	}
	identity, _ := ssh.NewSignerFromKey(m.key)
	host, _ := ssh.NewPublicKey(tr.CustomerKey)
	go func() {
		<-ctx.Done()
		closeStream(conn)
		cleanup()
		m.c.end()
	}()
	connected = true
	return &link{Conn: conn, close: func() { stop(); cleanup() }}, identity, host, nil
}

type link struct {
	net.Conn
	close func()
}

func (l *link) Close() error {
	err := closeStream(l.Conn)
	l.close()
	return err
}
func (m *Merchant) Connect(ctx context.Context, verify func(pairing.Transcript, string) error) (*ssh.Client, error) {
	conn, key, host, e := m.ConnectStream(ctx, verify)
	if e != nil {
		return nil, e
	}
	client, e := sshbridge.Client(conn, m.Invite.SessionID, key, host)
	if e != nil {
		conn.Close()
	}
	return client, e
}
func ServeCustomer(ctx context.Context, raw string, client *http.Client, handler sshbridge.Handler, consent func(pairing.Transcript, string) (*session.Gate, error)) error {
	i, e := pairing.DecodeInvite(raw)
	if e != nil {
		return e
	}
	if client == nil {
		client, e = config.HTTPClient(i.Base)
		if e != nil {
			return e
		}
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	c := &coordinator{base: i.Base, id: i.SessionID, token: base64.RawURLEncoding.EncodeToString(i.Join), role: "customer", secret: i.Secret, client: client, auth: pairing.SignalAuthenticator{Secret: i.Secret, SessionID: i.SessionID, PeerRole: "merchant", Epoch: 1}}
	claim, _ := json.Marshal(struct {
		ID  string `json:"session_id"`
		Key string `json:"key"`
	}{i.SessionID, base64.RawURLEncoding.EncodeToString(pub)})
	if e = c.request(ctx, "/v1/claim", claim, nil); e != nil {
		return e
	}
	defer c.end()
	if _, e = pairing.ParseInvite(raw, c.clock.Now()); e != nil {
		return e
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	leaseCtx, stopInitialLease := context.WithCancel(ctx)
	defer stopInitialLease()
	go c.lease(leaseCtx, stop, nil)
	tr, e := c.handshake(ctx, i, key)
	if e != nil {
		return e
	}
	if consent == nil {
		return errors.New("CONSENT_REQUIRED")
	}
	code, e := pairing.VerificationCode(i.Secret, tr)
	if e != nil {
		return e
	}
	gate, e := consent(tr, code)
	if e != nil {
		return e
	}
	if gate == nil || gate.Snapshot().SessionID != i.SessionID {
		return errors.New("CONSENT_REQUIRED")
	}
	defer gate.End()
	stopInitialLease()
	go c.lease(ctx, stop, gate)
	go watchRevocation(ctx, stop, gate)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		state := gate.Snapshot().State
		if state == "ACTIVE" || state == "PAUSED" {
			break
		}
		if state != "AWAITING_CONSENT" {
			return errors.New("SESSION_REVOKED")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
	if e = c.awaitReady(ctx); e != nil {
		return e
	}
	conn, cleanup, e := c.openTransport(ctx, stop)
	if e != nil {
		return e
	}
	defer cleanup()
	defer conn.Close()
	go func() { <-ctx.Done(); closeStream(conn); cleanup() }()
	host, _ := ssh.NewSignerFromKey(key)
	merchant, _ := ssh.NewPublicKey(i.MerchantKey)
	return sshbridge.Serve(conn, host, merchant, handler)
}

// IdentityPEM exports the ephemeral merchant SSH private key; callers must write it with owner-only permissions.
func (m *Merchant) IdentityPEM() ([]byte, error) {
	block, err := ssh.MarshalPrivateKey(m.key, "PairShell ephemeral session")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}
