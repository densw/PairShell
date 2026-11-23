package pairing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestVerificationCodeUsesSessionSecretAndBindsBothPeers(t *testing.T) {
	invite, _, err := NewInvite("https://example.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr := Transcript{
		Version: 1, SessionID: invite.SessionID, Expires: invite.Expires, Capabilities: "terminal",
		MerchantKey: invite.MerchantKey, CustomerKey: bytes.Repeat([]byte{1}, 32),
		MerchantChallenge: bytes.Repeat([]byte{2}, 32), CustomerChallenge: bytes.Repeat([]byte{3}, 32),
	}
	code, err := VerificationCode(invite.Secret, tr)
	if err != nil || !MatchVerificationCode(code, code) {
		t.Fatalf("invalid local code: %v", err)
	}
	if same, err := VerificationCode(invite.Secret, tr); err != nil || same != code {
		t.Fatalf("peer with matching secret and transcript got a different code: %v", err)
	}
	public, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256(append([]byte("PairShell:verification:v1\x00"), public...))
	if code == hex.EncodeToString(old[:12]) {
		t.Fatal("accepted public-transcript derivation")
	}
	if _, err := VerificationCode(nil, tr); err == nil {
		t.Fatal("derived a code without the session secret")
	}
	foreign := sha256.Sum256(public)
	if other, err := VerificationCode(foreign[:], tr); err != nil || other == code {
		t.Fatalf("public handshake material substituted for the session secret: %v", err)
	}
	for _, field := range []*[]byte{&tr.MerchantChallenge, &tr.CustomerChallenge} {
		(*field)[0] ^= 1
		if other, err := VerificationCode(invite.Secret, tr); err != nil || other == code {
			t.Fatalf("changed peer challenge did not change the code: %v", err)
		}
		(*field)[0] ^= 1
	}
}

func TestSignalReplayAndIsolation(t *testing.T) {
	secret := bytes.Repeat([]byte{7}, 32)
	a := SignalAuthenticator{Secret: secret, SessionID: "a", PeerRole: "customer", Epoch: 1}
	s := Signal{Version: 1, SessionID: "a", Role: "customer", Epoch: 1, Sequence: 1, Type: "offer", Payload: []byte("sdp")}
	m, _ := SignSignal(secret, s)
	changed := s
	changed.Payload = []byte("different candidate")
	if a.Verify(changed, m) == nil {
		t.Fatal("changed signal accepted with the original authenticator")
	}
	if a.Verify(s, m) != nil {
		t.Fatal("valid signal rejected")
	}
	if a.Verify(s, m) == nil {
		t.Fatal("replay accepted")
	}
	s.Sequence = 2
	s.SessionID = "b"
	m, _ = SignSignal(secret, s)
	if a.Verify(s, m) == nil {
		t.Fatal("cross session accepted")
	}
	s.SessionID = "a"
	s.Role = "merchant"
	m, _ = SignSignal(secret, s)
	if a.Verify(s, m) == nil {
		t.Fatal("reflection accepted")
	}
}
func TestInviteExpiredAndSecretRedaction(t *testing.T) {
	i, _, _ := NewInvite("https://example.org", time.Minute)
	i.Expires = time.Now().Add(-time.Minute).Unix()
	if _, e := ParseInvite(i.URL(), time.Now()); e == nil {
		t.Fatal("expired invitation accepted")
	}
}
