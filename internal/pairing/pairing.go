package pairing

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var enc = base64.RawURLEncoding

type Invite struct {
	Base        string
	SessionID   string
	Join        []byte
	Secret      []byte
	MerchantKey ed25519.PublicKey
	Expires     int64
}

func random(n int) ([]byte, error) { b := make([]byte, n); _, e := rand.Read(b); return b, e }
func NewInvite(base string, ttl time.Duration) (Invite, ed25519.PrivateKey, error) {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return Invite{}, nil, errors.New("invalid server URL")
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		return Invite{}, nil, errors.New("invalid ttl")
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return Invite{}, nil, e
	}
	sid, e := random(16)
	if e != nil {
		return Invite{}, nil, e
	}
	join, e := random(32)
	if e != nil {
		return Invite{}, nil, e
	}
	secret, e := random(32)
	if e != nil {
		return Invite{}, nil, e
	}
	return Invite{Base: strings.TrimRight(base, "/"), SessionID: enc.EncodeToString(sid), Join: join, Secret: secret, MerchantKey: pub, Expires: time.Now().Add(ttl).Unix()}, key, nil
}
func (i Invite) URL() string {
	q := url.Values{"v": {"1"}, "sid": {i.SessionID}, "join": {enc.EncodeToString(i.Join)}, "secret": {enc.EncodeToString(i.Secret)}, "merchant_key": {enc.EncodeToString(i.MerchantKey)}, "exp": {strconv.FormatInt(i.Expires, 10)}}
	return i.Base + "/pair#" + q.Encode()
}
func ParseInvite(raw string, now time.Time) (Invite, error) {
	var i Invite
	if len(raw) > 4096 {
		return i, errors.New("invalid invite")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return i, errors.New("invalid invite")
	}
	q, e := url.ParseQuery(u.Fragment)
	if e != nil || len(q) != 6 || q.Get("v") != "1" {
		return i, errors.New("invalid invite")
	}
	for _, v := range q {
		if len(v) != 1 {
			return i, errors.New("invalid invite")
		}
	}
	i.Base = "https://" + u.Host
	i.SessionID = q.Get("sid")
	sid, e := enc.DecodeString(i.SessionID)
	if e != nil || len(sid) != 16 {
		return i, errors.New("invalid invite")
	}
	i.Join, e = enc.DecodeString(q.Get("join"))
	if e != nil || len(i.Join) != 32 {
		return i, errors.New("invalid invite")
	}
	i.Secret, e = enc.DecodeString(q.Get("secret"))
	if e != nil || len(i.Secret) != 32 {
		return i, errors.New("invalid invite")
	}
	i.MerchantKey, e = enc.DecodeString(q.Get("merchant_key"))
	if e != nil || len(i.MerchantKey) != 32 {
		return i, errors.New("invalid invite")
	}
	i.Expires, e = strconv.ParseInt(q.Get("exp"), 10, 64)
	if e != nil || i.Expires <= now.Unix() {
		return i, errors.New("INVITE_EXPIRED")
	}
	return i, nil
}

type Transcript struct {
	Version           int               `json:"version"`
	SessionID         string            `json:"session_id"`
	MerchantKey       ed25519.PublicKey `json:"merchant_key"`
	CustomerKey       ed25519.PublicKey `json:"customer_key"`
	MerchantChallenge []byte            `json:"merchant_challenge"`
	CustomerChallenge []byte            `json:"customer_challenge"`
	Expires           int64             `json:"expires"`
	Capabilities      string            `json:"capabilities"`
}
type Proof struct {
	MAC       []byte `json:"mac"`
	Signature []byte `json:"signature"`
}

func transcriptBytes(role string, t Transcript) ([]byte, error) {
	if (role != "merchant" && role != "customer") || t.Version != 1 || len(t.MerchantKey) != 32 || len(t.CustomerKey) != 32 || len(t.MerchantChallenge) != 32 || len(t.CustomerChallenge) != 32 || len(t.Capabilities) > 4096 {
		return nil, errors.New("PAIRING_MISMATCH")
	}
	b, e := json.Marshal(t)
	return append([]byte("PairShell:handshake:v1\x00"+role+"\x00"), b...), e
}
func Authenticate(secret []byte, key ed25519.PrivateKey, role string, t Transcript) (Proof, error) {
	b, e := transcriptBytes(role, t)
	if e != nil || len(secret) != 32 || len(key) != 64 {
		return Proof{}, errors.New("PAIRING_MISMATCH")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(b)
	return Proof{mac.Sum(nil), ed25519.Sign(key, b)}, nil
}
func Verify(secret []byte, role string, t Transcript, p Proof) error {
	b, e := transcriptBytes(role, t)
	if e != nil || len(secret) != 32 {
		return errors.New("PAIRING_MISMATCH")
	}
	if t.Expires <= time.Now().Unix() {
		return errors.New("INVITE_EXPIRED")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(b)
	key := t.MerchantKey
	if role == "customer" {
		key = t.CustomerKey
	}
	if !hmac.Equal(p.MAC, mac.Sum(nil)) || !ed25519.Verify(key, b, p.Signature) {
		return errors.New("PAIRING_MISMATCH")
	}
	return nil
}

// VerificationCode binds the human check to the authenticated transcript and
// the 32-byte invitation secret. The relay can observe the transcript.
func VerificationCode(secret []byte, t Transcript) (string, error) {
	if len(secret) != 32 {
		return "", errors.New("PAIRING_MISMATCH")
	}
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("PairShell:verification:v2\x00"))
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil)[:12]), nil
}

// MatchVerificationCode compares the full code read from a trusted channel.
// The device pairing code and abbreviated transcript codes are not accepted.
func MatchVerificationCode(expected, supplied string) bool {
	supplied = strings.TrimSpace(supplied)
	if len(expected) != 24 || len(supplied) != 24 {
		return false
	}
	a, aErr := hex.DecodeString(expected)
	b, bErr := hex.DecodeString(supplied)
	return aErr == nil && bErr == nil && hmac.Equal(a, b)
}

type Signal struct {
	Version   int    `json:"version"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`
	Epoch     uint64 `json:"authorization_epoch"`
	Sequence  uint64 `json:"sequence"`
	Type      string `json:"type"`
	Payload   []byte `json:"payload"`
}
type SignalAuthenticator struct {
	mu        sync.Mutex
	Secret    []byte
	SessionID string
	PeerRole  string
	Epoch     uint64
	last      uint64
}

func SignSignal(secret []byte, s Signal) ([]byte, error) {
	if len(secret) != 32 || len(s.Payload) > 262144 {
		return nil, errors.New("invalid signal")
	}
	b, e := json.Marshal(s)
	if e != nil {
		return nil, e
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("PairShell:signal:v1\x00"))
	m.Write(b)
	return m.Sum(nil), nil
}
func (a *SignalAuthenticator) Verify(s Signal, mac []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s.Version != 1 || s.SessionID != a.SessionID || s.Role != a.PeerRole || s.Epoch != a.Epoch || s.Sequence <= a.last || (s.Role != "merchant" && s.Role != "customer") {
		return errors.New("PAIRING_MISMATCH")
	}
	expected, e := SignSignal(a.Secret, s)
	if e != nil || !hmac.Equal(mac, expected) {
		return errors.New("PAIRING_MISMATCH")
	}
	a.last = s.Sequence
	return nil
}

// VerifyInvite additionally binds the authenticated transcript to the original invitation.
func VerifyInvite(i Invite, role string, t Transcript, p Proof) error {
	if t.SessionID != i.SessionID || t.Expires != i.Expires || !hmac.Equal(t.MerchantKey, i.MerchantKey) {
		return errors.New("PAIRING_MISMATCH")
	}
	return Verify(i.Secret, role, t, p)
}
