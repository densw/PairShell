package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestSourceOnlyTrustsForwardingThroughConfiguredProxies(t *testing.T) {
	s := New(Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}})
	for _, tc := range []struct{ remote, forwarded, want string }{
		{"198.51.100.1:44000", "203.0.113.8", "198.51.100.1"},
		{"192.0.2.1:44000", "203.0.113.8, 198.51.100.1, 192.0.2.2", "198.51.100.1"},
		{"192.0.2.1:44000", "malformed, 192.0.2.2", "192.0.2.1"},
		{"[2001:db8:1::beef]:44000", "", "2001:db8:1::/64"},
	} {
		r := httptest.NewRequest("POST", "/v1/lease", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := s.sourceKey(r); got != tc.want {
			t.Errorf("client source from %s and %q = %q, want %q", tc.remote, tc.forwarded, got, tc.want)
		}
	}
}

func TestSourceIngressBudgetBoundsConcurrentAndLargeRequests(t *testing.T) {
	s := New(Config{})
	source := "192.0.2.1"
	if !s.admit(source, 256<<10) || !s.admit(source, 256<<10) {
		t.Fatal("valid request burst rejected")
	}
	if s.admit(source, 256<<10) {
		t.Fatal("source byte budget allowed excess requests")
	}
	s.release(source)
	s.release(source)
	other := "192.0.2.2"
	for range maxSourceConcurrent {
		if !s.admit(other, 64) {
			t.Fatal("separate source was unexpectedly blocked")
		}
	}
	if s.admit(other, 64) {
		t.Fatal("source exceeded concurrent request budget")
	}
	for range maxSourceConcurrent {
		s.release(other)
	}
}

func TestTURNRequiresBothPeersReadyAndUsesStableCredential(t *testing.T) {
	s := New(Config{CreateToken: "admin", TURNSecret: "test secret"})
	c, err := s.Create("admin", "turn-room-1234567890", "merchant")
	if err != nil {
		t.Fatal(err)
	}
	req := roomRequest{SessionID: c.SessionID}
	if status, _ := s.roomResponse("/v1/turn", c.CreatorToken, req); status != http.StatusForbidden {
		t.Fatal("unclaimed room received TURN credentials")
	}
	if err := s.Claim(c.SessionID, c.JoinToken, "customer"); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.roomResponse("/v1/ready", c.CreatorToken, req); status != http.StatusOK {
		t.Fatal(status)
	}
	if status, _ := s.roomResponse("/v1/turn", c.CreatorToken, req); status != http.StatusForbidden {
		t.Fatal("room received TURN credentials before both peers were ready")
	}
	if status, _ := s.roomResponse("/v1/ready", c.JoinToken, req); status != http.StatusOK {
		t.Fatal(status)
	}
	merchantStatus, merchantCredential := s.roomResponse("/v1/turn", c.CreatorToken, req)
	customerStatus, customerCredential := s.roomResponse("/v1/turn", c.JoinToken, req)
	if merchantStatus != http.StatusOK || customerStatus != http.StatusOK {
		t.Fatal(merchantStatus, customerStatus)
	}
	a, _ := json.Marshal(merchantCredential)
	b, _ := json.Marshal(customerCredential)
	if string(a) != string(b) {
		t.Fatal("TURN quota identity was not shared by the room")
	}
}

func TestSignalQueueAndBurstHaveByteLimits(t *testing.T) {
	s := New(Config{CreateToken: "admin"})
	c, err := s.Create("admin", "budget-room-1234567890", "merchant")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(c.SessionID, c.JoinToken, "customer"); err != nil {
		t.Fatal(err)
	}
	req := roomRequest{SessionID: c.SessionID, Data: json.RawMessage(fmt.Sprintf("%q", strings.Repeat("a", 128<<10)))}
	if status, _ := s.roomResponse("/v1/signal", c.CreatorToken, req); status != http.StatusOK {
		t.Fatal(status)
	}
	if status, _ := s.roomResponse("/v1/signal", c.CreatorToken, req); status != http.StatusTooManyRequests {
		t.Fatal("queue accepted bytes over capacity", status)
	}
	if status, _ := s.roomResponse("/v1/poll", c.JoinToken, req); status != http.StatusOK {
		t.Fatal(status)
	}
	if status, _ := s.roomResponse("/v1/signal", c.CreatorToken, req); status != http.StatusTooManyRequests {
		t.Fatal("poll reset signal byte-rate limit", status)
	}
	req.Data = json.RawMessage(fmt.Sprintf("%q", strings.Repeat("a", maxSignalQueueBytes)))
	if status, _ := s.roomResponse("/v1/signal", c.CreatorToken, req); status != http.StatusRequestEntityTooLarge {
		t.Fatal("overlarge signal accepted", status)
	}
}

func TestExpiredPeerLeaseReclaimsRoom(t *testing.T) {
	s := New(Config{CreateToken: "admin", MaxRooms: 1})
	c, err := s.Create("admin", "expiring-room-1234567", "merchant")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(c.SessionID, c.JoinToken, "customer"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.rooms[c.SessionID].merchantLease = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if status, _ := s.roomResponse("/v1/lease", c.JoinToken, roomRequest{SessionID: c.SessionID}); status != http.StatusForbidden {
		t.Fatal("other peer kept the expired room alive", status)
	}
	if _, err := s.Create("admin", "replacement-room-12345", "merchant"); err != nil {
		t.Fatal("expired room retained capacity", err)
	}
}

func TestMailboxReservationExpiresWithinAttemptBudget(t *testing.T) {
	s := New(Config{PublicDevicePairing: true})
	_, created := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": []byte("test initial message")}})
	claim := map[string]any{"data": map[string]any{"locator": created["locator"]}}
	for i := 0; i < maxPairAttempts; i++ {
		n, v := codeCall(t, s, "/v1/code/claim", "", claim)
		if n != http.StatusOK {
			t.Fatal("expired mailbox reservation blocked later claim", n)
		}
		s.mu.Lock()
		s.deviceCodes[v["session_id"].(string)].pairCode.claimExpires = time.Now().Add(-time.Second)
		s.mu.Unlock()
	}
	if n, _ := codeCall(t, s, "/v1/code/claim", "", claim); n != http.StatusForbidden {
		t.Fatal("mailbox claim attempts were unbounded", n)
	}
}
