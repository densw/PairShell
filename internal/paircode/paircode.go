// Package paircode exchanges the existing authenticated invitation using a
// attempt-bounded PAKE mailbox. Only the six-digit locator is sent to the server;
// the six-digit password remains on the endpoints.
package paircode

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/schollz/pake/v3"
	"golang.org/x/crypto/hkdf"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"pairshell/internal/config"
	"pairshell/internal/pairing"
	"strings"
	"time"
	"unicode"
)

var ErrCode = errors.New("PAIR_CODE_UNAVAILABLE: 配对码无效、已使用或已过期，请生成新码")
var ErrFormat = errors.New("PAIR_CODE_FORMAT: 请输入完整的 12 位配对码，不支持旧版码")

const maxLocalAttempts = 3

// The owner keeps this budget for the lifetime of the displayed code.
// A mailbox reset replaces the PAKE instance but never the budget.
type ownerAttempts struct {
	proofs [maxLocalAttempts][sha256.Size]byte
	count  int
}

func (a *ownerAttempts) start(message []byte) bool {
	if a.count == len(a.proofs) {
		return false
	}
	proof := sha256.Sum256(message)
	for _, previous := range a.proofs[:a.count] {
		if previous == proof {
			return false
		}
	}
	a.proofs[a.count] = proof
	a.count++
	return true
}

func (a *ownerAttempts) exhausted() bool { return a.count == len(a.proofs) }

type Result struct {
	Invite string
	Err    error
}
type endpoint struct {
	base, id, locator, token string
	client                   *http.Client
	clock                    pairing.ServerClock
}
type response struct {
	Permit       string `json:"create_permit"`
	Approved     bool   `json:"approved"`
	Expired      bool   `json:"expired"`
	ID           string `json:"session_id"`
	Token        string `json:"token"`
	Locator      string `json:"locator"`
	Phase        int    `json:"phase"`
	Message      []byte `json:"message"`
	Preparation  []byte `json:"preparation"`
	Confirmation []byte `json:"confirmation"`
}
type reply struct {
	PAKE       []byte `json:"pake"`
	Ciphertext []byte `json:"ciphertext,omitempty"`
	Proof      []byte `json:"proof,omitempty"`
}

func clientEndpoint(base string, client *http.Client) (*endpoint, error) {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, ErrCode
	}
	if client == nil {
		client, e = config.HTTPClient(base)
		if e != nil {
			return nil, e
		}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return ErrCode }
	if copyClient.Timeout == 0 || copyClient.Timeout > 10*time.Second {
		copyClient.Timeout = 10 * time.Second
	}
	return &endpoint{base: strings.TrimRight(base, "/"), client: &copyClient}, nil
}
func (e *endpoint) call(ctx context.Context, path string, message []byte) (response, error) {
	raw, _ := json.Marshal(struct {
		ID   string `json:"session_id,omitempty"`
		Data struct {
			Locator string `json:"locator,omitempty"`
			Message []byte `json:"message,omitempty"`
		} `json:"data"`
	}{e.id, struct {
		Locator string `json:"locator,omitempty"`
		Message []byte `json:"message,omitempty"`
	}{e.locator, message}})
	r, err := http.NewRequestWithContext(ctx, "POST", e.base+path, bytes.NewReader(raw))
	if err != nil {
		return response{}, ErrCode
	}
	r.Header.Set("Authorization", "Bearer "+e.token)
	r.Header.Set("Content-Type", "application/json")
	sent := time.Now()
	res, err := e.client.Do(r)
	if err != nil {
		return response{}, &RequestError{Operation: path}
	}
	defer res.Body.Close()
	e.clock.ObserveResponse(res.Header, sent)
	if res.StatusCode != http.StatusOK {
		return response{}, &RequestError{Operation: path, Status: res.StatusCode}
	}
	var out response
	if err = json.NewDecoder(io.LimitReader(res.Body, 16385)).Decode(&out); err != nil {
		return out, &RequestError{Operation: path, Status: http.StatusBadGateway}
	}
	return out, nil
}
func password() (string, error) {
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%06d", n), nil
}
func displayCode(locator, secret string) string {
	raw := locator + secret
	groups := []string{}
	for len(raw) > 4 {
		groups = append(groups, raw[:4])
		raw = raw[4:]
	}
	return strings.Join(append(groups, raw), "-")
}
func parse(code string) (string, string, error) {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' || r >= 0x2010 && r <= 0x2015 || r == 0x2212 || r == 0xfe63 || r == 0xff0d || r >= 0x200b && r <= 0x200f || r >= 0x202a && r <= 0x202e || r >= 0x2060 && r <= 0x2069 || r == 0xfeff {
			return -1
		}
		if r >= '０' && r <= '９' {
			return r - 0xfee0
		}
		return r
	}, code)
	if len(compact) != 12 {
		return "", "", ErrFormat
	}
	for _, r := range compact {
		if r < '0' || r > '9' {
			return "", "", ErrFormat
		}
	}
	return compact[:6], compact[6:], nil
}
func newPAKE(secret string, role int, base, purpose string) (*pake.Pake, error) {
	return pake.InitCurveWithIdentities([]byte(secret), role, "p256", []byte("PairShell:code:v1:"+purpose+":owner:"+base), []byte("PairShell:code:v1:"+purpose+":joiner:"+base))
}

// The dependency rejects invalid curve points. Recover also contains malformed
// public-value panics at this untrusted wire boundary; no crypto is reimplemented.
func update(p *pake.Pake, data []byte) (err error) {
	if len(data) == 0 || len(data) > 4096 {
		return ErrCode
	}
	defer func() {
		if recover() != nil {
			err = ErrCode
		}
	}()
	if p.Update(data) != nil {
		return ErrCode
	}
	return nil
}
func (e *endpoint) key(p *pake.Pake) ([]byte, error) {
	raw, err := p.SessionKey()
	if err != nil {
		return nil, ErrCode
	}
	key := make([]byte, 32)
	_, err = io.ReadFull(hkdf.New(sha256.New, raw, nil, []byte("PairShell:code:invite:v1\x00"+e.base+"\x00"+e.id+"\x00"+e.locator)), key)
	return key, err
}
func (e *endpoint) aad() []byte {
	return []byte("PairShell:code:envelope:v1\x00" + e.base + "\x00" + e.id + "\x00" + e.locator)
}
func (e *endpoint) seal(key []byte, text string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, []byte(text), e.aad()), nil
}
func (e *endpoint) open(key, data []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrCode
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(data) < aead.NonceSize() {
		return "", ErrCode
	}
	text, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], e.aad())
	if err != nil {
		return "", ErrCode
	}
	i, err := pairing.ParseInvite(string(text), e.clock.Now())
	if err != nil || i.Base != e.base {
		return "", ErrCode
	}
	return string(text), nil
}
func (e *endpoint) proof(key []byte, label string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(e.aad())
	m.Write([]byte(label))
	return m.Sum(nil)
}
func wait(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Listen displays a customer-generated temporary code without creating an
// execution session or requiring a deployment credential.
func Listen(ctx context.Context, base string, client *http.Client) (string, <-chan Result, error) {
	e, err := clientEndpoint(base, client)
	if err != nil {
		return "", nil, err
	}
	secret, err := password()
	if err != nil {
		return "", nil, err
	}
	p, err := newPAKE(secret, 0, e.base, "device")
	if err != nil {
		return "", nil, err
	}
	created, err := e.call(ctx, "/v1/device/create", p.Bytes())
	if err != nil {
		return "", nil, err
	}
	e.id, e.token, e.locator = created.ID, created.Token, created.Locator
	code := displayCode(e.locator, secret)
	done := make(chan Result, 1)
	go func() {
		defer close(done)
		timeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		invite, err := e.listen(timeout, p, secret)
		if err != nil {
			e.cancel()
		}
		done <- Result{invite, err}
	}()
	return code, done, nil
}
func (e *endpoint) cancel() {
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_, _ = e.call(ctx, "/v1/code/cancel", nil)
}

func (e *endpoint) reset(ctx context.Context, secret, purpose string) (*pake.Pake, error) {
	p, err := newPAKE(secret, 0, e.base, purpose)
	if err != nil {
		return nil, err
	}
	_, err = e.call(ctx, "/v1/code/reset", p.Bytes())
	return p, err
}

func (e *endpoint) listen(ctx context.Context, p *pake.Pake, secret string) (string, error) {
	var key []byte
	var attempts ownerAttempts
	for {
		out, err := e.call(ctx, "/v1/code/sender", nil)
		if err != nil {
			return "", err
		}
		if out.Expired {
			if key != nil {
				return "", ErrCode
			}
			p, err = e.reset(ctx, secret, "device")
			if err != nil {
				return "", err
			}
			if err = wait(ctx); err != nil {
				return "", ErrCode
			}
			continue
		}
		if key == nil && len(out.Preparation) != 0 && attempts.start(out.Preparation) {
			var prepared reply
			if json.Unmarshal(out.Preparation, &prepared) == nil && update(p, prepared.PAKE) == nil {
				key, err = e.key(p)
				if err != nil || !hmac.Equal(prepared.Proof, e.proof(key, "device-joiner-ready")) {
					key = nil
				}
			}
			if key == nil {
				if attempts.exhausted() {
					return "", ErrCode
				}
				p, err = e.reset(ctx, secret, "device")
				if err != nil {
					return "", err
				}
			} else if _, err = e.call(ctx, "/v1/code/authorize", nil); err != nil {
				return "", err
			}
		}
		if out.Phase >= 2 {
			var r reply
			if key == nil || json.Unmarshal(out.Message, &r) != nil {
				return "", ErrCode
			}
			invite, err := e.open(key, r.Ciphertext)
			if err != nil {
				return "", err
			}
			if _, err = e.call(ctx, "/v1/code/deliver", e.proof(key, "device-owner-accepted")); err != nil {
				return "", err
			}
			return invite, nil
		}
		if err = wait(ctx); err != nil {
			return "", ErrCode
		}
	}
}

// Send transfers an invitation to the customer displaying code. Possessing a
// code never opens a shell: the existing customer consent gate still applies.
func Send(ctx context.Context, base, code, invite string, client *http.Client) error {
	return SendWith(ctx, base, code, client, func(string) (string, error) { return invite, nil })
}

// SendWith obtains an invitation only after the device owner verifies the
// PAKE proof and authorizes a single-room permit.
func SendWith(ctx context.Context, base, code string, client *http.Client, create func(string) (string, error)) error {
	e, err := clientEndpoint(base, client)
	if err != nil {
		return err
	}
	locator, secret, err := parse(code)
	if err != nil {
		return err
	}
	e.locator = locator
	claimed, err := e.call(ctx, "/v1/code/claim", nil)
	if err != nil {
		return err
	}
	e.id, e.token = claimed.ID, claimed.Token
	p, err := newPAKE(secret, 1, e.base, "device")
	if err != nil || update(p, claimed.Message) != nil {
		return ErrCode
	}
	key, err := e.key(p)
	if err != nil {
		return err
	}
	prepared, _ := json.Marshal(reply{PAKE: p.Bytes(), Proof: e.proof(key, "device-joiner-ready")})
	if _, err = e.call(ctx, "/v1/code/prepare", prepared); err != nil {
		return err
	}
	for {
		out, err := e.call(ctx, "/v1/code/receiver", nil)
		if err != nil {
			return err
		}
		if out.Approved {
			break
		}
		if err = wait(ctx); err != nil {
			return ErrCode
		}
	}
	permit, err := e.call(ctx, "/v1/code/permit", nil)
	if err != nil || permit.Permit == "" {
		return ErrCode
	}
	invite, err := create(permit.Permit)
	if err != nil {
		return err
	}
	encrypted, err := e.seal(key, invite)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(reply{Ciphertext: encrypted})
	if _, err = e.call(ctx, "/v1/code/reply", raw); err != nil {
		return err
	}
	for {
		out, err := e.call(ctx, "/v1/code/receiver", nil)
		if err != nil {
			return err
		}
		if out.Phase >= 3 {
			if !hmac.Equal(out.Message, e.proof(key, "device-owner-accepted")) {
				return ErrCode
			}
			_, err = e.call(ctx, "/v1/code/finish", e.proof(key, "device-joiner-finished"))
			return err
		}
		if err = wait(ctx); err != nil {
			return ErrCode
		}
	}
}

// Publish supports the legacy direction (merchant shares a code) without
// revealing the invitation secret to the server.
func Publish(ctx context.Context, base, sessionID, credential, invite string, client *http.Client) (string, <-chan error, error) {
	e, err := clientEndpoint(base, client)
	if err != nil {
		return "", nil, err
	}
	e.id, e.token = sessionID, credential
	secret, err := password()
	if err != nil {
		return "", nil, err
	}
	p, err := newPAKE(secret, 0, e.base, "invite")
	if err != nil {
		return "", nil, err
	}
	created, err := e.call(ctx, "/v1/code/create", p.Bytes())
	if err != nil {
		return "", nil, err
	}
	e.locator = created.Locator
	done := make(chan error, 1)
	go func() {
		defer close(done)
		timeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		err := e.publish(timeout, p, invite, secret)
		if err != nil {
			e.cancel()
		}
		done <- err
	}()
	return displayCode(e.locator, secret), done, nil
}
func (e *endpoint) publish(ctx context.Context, p *pake.Pake, invite, secret string) error {
	var attempts ownerAttempts
	for {
		out, err := e.call(ctx, "/v1/code/sender", nil)
		if err != nil {
			return err
		}
		if out.Phase >= 2 && attempts.start(out.Message) {
			var r reply
			var key []byte
			if json.Unmarshal(out.Message, &r) == nil && update(p, r.PAKE) == nil {
				key, err = e.key(p)
			}
			if key == nil || err != nil || !hmac.Equal(r.Proof, e.proof(key, "invite-receiver")) {
				if attempts.exhausted() {
					return ErrCode
				}
				p, err = e.reset(ctx, secret, "invite")
				if err != nil {
					return err
				}
				if err = wait(ctx); err != nil {
					return ErrCode
				}
				continue
			}
			encrypted, err := e.seal(key, invite)
			if err != nil {
				return err
			}
			if _, err = e.call(ctx, "/v1/code/deliver", encrypted); err != nil {
				return err
			}
			return nil
		}
		if err = wait(ctx); err != nil {
			return ErrCode
		}
	}
}
func Receive(ctx context.Context, base, code string, client *http.Client) (string, error) {
	e, err := clientEndpoint(base, client)
	if err != nil {
		return "", err
	}
	locator, secret, err := parse(code)
	if err != nil {
		return "", err
	}
	e.locator = locator
	claimed, err := e.call(ctx, "/v1/code/claim", nil)
	if err != nil {
		return "", err
	}
	e.id, e.token = claimed.ID, claimed.Token
	p, err := newPAKE(secret, 1, e.base, "invite")
	if err != nil || update(p, claimed.Message) != nil {
		return "", ErrCode
	}
	key, err := e.key(p)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(reply{PAKE: p.Bytes(), Proof: e.proof(key, "invite-receiver")})
	if _, err = e.call(ctx, "/v1/code/reply", raw); err != nil {
		return "", err
	}
	for {
		out, err := e.call(ctx, "/v1/code/receiver", nil)
		if err != nil {
			return "", err
		}
		if out.Phase >= 3 {
			invite, err := e.open(key, out.Message)
			if err != nil {
				return "", err
			}
			if _, err = e.call(ctx, "/v1/code/finish", e.proof(key, "invite-finished")); err != nil {
				return "", err
			}
			return invite, nil
		}
		if err = wait(ctx); err != nil {
			return "", ErrCode
		}
	}
}
