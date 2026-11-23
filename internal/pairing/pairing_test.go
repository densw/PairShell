package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestInviteAndProof(t *testing.T) {
	i, key, e := NewInvite("https://example.org", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := ParseInvite(i.URL(), time.Now())
	if e != nil || parsed.SessionID != i.SessionID {
		t.Fatal(e)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	tr := Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: i.MerchantKey, CustomerKey: pub, MerchantChallenge: make([]byte, 32), CustomerChallenge: make([]byte, 32), Expires: i.Expires, Capabilities: "terminal"}
	p, e := Authenticate(i.Secret, key, "merchant", tr)
	if e != nil {
		t.Fatal(e)
	}
	if e = Verify(i.Secret, "merchant", tr, p); e != nil {
		t.Fatal(e)
	}
	tr.Capabilities = "files"
	if Verify(i.Secret, "merchant", tr, p) == nil {
		t.Fatal("tamper accepted")
	}
}
