package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCoordinatorClockRejectsExpiryWithinDateUncertainty(t *testing.T) {
	i, key, err := NewInvite("https://example.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	i.Expires = stamp.Add(time.Second).Unix()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tr := Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: i.MerchantKey, CustomerKey: pub, MerchantChallenge: make([]byte, 32), CustomerChallenge: make([]byte, 32), Expires: i.Expires, Capabilities: "terminal"}
	p, err := Authenticate(i.Secret, key, "merchant", tr)
	if err != nil {
		t.Fatal(err)
	}
	var clock ServerClock
	h := http.Header{}
	h.Set("Date", stamp.Format(http.TimeFormat))
	clock.Observe(h)
	if _, err := ParseInvite(i.URL(), clock.Now()); err == nil {
		t.Fatal("Date truncation extended invitation authority")
	}
	if err := VerifyAt(i.Secret, "merchant", tr, p, clock.Now()); err == nil {
		t.Fatal("Date truncation extended proof authority")
	}
}

func TestDecodeInviteSeparatesStructureFromExpiry(t *testing.T) {
	i, _, err := NewInvite("https://example.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	i.Expires = time.Now().Add(-24 * time.Hour).Unix()
	decoded, err := DecodeInvite(i.URL())
	if err != nil || decoded.Expires != i.Expires {
		t.Fatalf("%+v: %v", decoded, err)
	}
	if _, err := ParseInvite(i.URL(), time.Now()); err == nil {
		t.Fatal("expired invite accepted")
	}
	if _, err := DecodeInvite(strings.Replace(i.URL(), "exp=", "exp=bad", 1)); err == nil {
		t.Fatal("malformed expiry accepted")
	}
	if _, err := DecodeInvite(strings.Replace(i.URL(), "secret=", "secret=x", 1)); err == nil {
		t.Fatal("malformed secret accepted")
	}
}

func TestVerifyAtUsesExplicitTimeAndPreservesBinding(t *testing.T) {
	i, key, err := NewInvite("https://example.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []time.Duration{-48 * time.Hour, 48 * time.Hour} {
		now := time.Now().Add(offset).In(time.FixedZone("test", -7*3600))
		i.Expires = now.Add(time.Minute).Unix()
		tr := Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: i.MerchantKey, CustomerKey: pub, MerchantChallenge: make([]byte, 32), CustomerChallenge: make([]byte, 32), Expires: i.Expires, Capabilities: "terminal"}
		p, err := Authenticate(i.Secret, key, "merchant", tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyAt(i.Secret, "merchant", tr, p, now); err != nil {
			t.Fatal(err)
		}
		if err := VerifyAt(i.Secret, "merchant", tr, p, time.Unix(i.Expires, 0)); err == nil {
			t.Fatal("expired proof accepted")
		}
		tr.Capabilities = "files"
		if err := VerifyAt(i.Secret, "merchant", tr, p, now); err == nil {
			t.Fatal("tampered proof accepted")
		}
		tr.Capabilities = "terminal"
		tr.SessionID = "substituted"
		if err := VerifyAt(i.Secret, "merchant", tr, p, now); err == nil {
			t.Fatal("substituted session accepted")
		}
	}
}
