package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestInviteBindingRejectsChangedMerchant(t *testing.T) {
	i, _, _ := NewInvite("https://example.org", time.Minute)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	customer, _, _ := ed25519.GenerateKey(rand.Reader)
	tr := Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: pub, CustomerKey: customer, MerchantChallenge: make([]byte, 32), CustomerChallenge: make([]byte, 32), Expires: i.Expires, Capabilities: "terminal"}
	p, _ := Authenticate(i.Secret, priv, "merchant", tr)
	if VerifyInvite(i, "merchant", tr, p) == nil {
		t.Fatal("substituted merchant accepted")
	}
}
