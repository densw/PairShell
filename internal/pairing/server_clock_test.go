package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestObserveResponseChargesElapsedBeforeProofExpiry(t *testing.T) {
	i, key, err := NewInvite("https://example.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 11, 27, 8, 0, 0, 900000000, time.UTC)
	i.Expires = stamp.Truncate(time.Second).Add(time.Second).Unix()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tr := Transcript{Version: 1, SessionID: i.SessionID, MerchantKey: i.MerchantKey, CustomerKey: pub, MerchantChallenge: make([]byte, 32), CustomerChallenge: make([]byte, 32), Expires: i.Expires, Capabilities: "terminal"}
	p, err := Authenticate(i.Secret, key, "merchant", tr)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("X-PairShell-Time", stamp.Format(time.RFC3339Nano))
	var clock ServerClock
	clock.ObserveResponse(h, time.Now().Add(-200*time.Millisecond))
	if _, err := ParseInvite(i.URL(), clock.Now()); err == nil {
		t.Fatal("request elapsed time omitted from invitation expiry")
	}
	if err := VerifyAt(i.Secret, "merchant", tr, p, clock.Now()); err == nil {
		t.Fatal("request elapsed time omitted from proof expiry")
	}
}

func TestResponseTimePrefersPrecisionAndFallsBackToDate(t *testing.T) {
	for _, tc := range []struct {
		precise, date string
		want          int64
		ok            bool
	}{
		{"2026-11-27T16:00:00.123456789+08:00", "Fri, 27 Nov 2026 09:00:00 GMT", 1795766400123456789, true},
		{"broken", "Fri, 27 Nov 2026 08:00:00 GMT", 1795766400000000000, true},
		{"", "Fri, 27 Nov 2026 08:00:00 GMT", 1795766400000000000, true},
		{"broken", "broken", 0, false},
		{"", "", 0, false},
	} {
		h := http.Header{}
		h.Set("X-PairShell-Time", tc.precise)
		h.Set("Date", tc.date)
		got, ok := ResponseTime(h)
		if ok != tc.ok || ok && got.UnixNano() != tc.want {
			t.Fatalf("%+v: %v %v", tc, got, ok)
		}
	}
}

func TestServerClockTracksSkewAndMonotonicElapsed(t *testing.T) {
	var local ServerClock
	before := time.Now()
	if now := local.Now(); now.Before(before) || now.After(time.Now()) {
		t.Fatal(now)
	}
	for _, offset := range []time.Duration{-365 * 24 * time.Hour, 365 * 24 * time.Hour} {
		var c ServerClock
		server := time.Now().Add(offset)
		h := http.Header{}
		h.Set("X-PairShell-Time", server.Format(time.RFC3339Nano))
		c.Observe(h)
		first := c.Now()
		if first.Before(server) || first.Sub(server) > time.Second {
			t.Fatalf("offset %v: %v", offset, first)
		}
		time.Sleep(10 * time.Millisecond)
		if elapsed := c.Now().Sub(first); elapsed < 5*time.Millisecond || elapsed > time.Second {
			t.Fatal(elapsed)
		}
		// Neither invalid nor out-of-order replies may extend authority by moving time back.
		c.Observe(http.Header{"X-Pairshell-Time": {"broken"}, "Date": {"broken"}})
		var wg sync.WaitGroup
		for n := 0; n < 20; n++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				stale := http.Header{}
				stale.Set("X-PairShell-Time", server.Add(-time.Duration(n)*time.Hour).Format(time.RFC3339Nano))
				c.Observe(stale)
				_ = c.Now()
			}(n)
		}
		wg.Wait()
		if c.Now().Before(first.Add(5 * time.Millisecond)) {
			t.Fatal("stale sample rewound clock")
		}
	}
}
