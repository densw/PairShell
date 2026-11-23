package server

import (
	"testing"
	"time"
)

func TestClaimSeparatesInvitationAndActiveExpiry(t *testing.T) {
	s := New(Config{CreateToken: "create", RoomTTL: time.Minute})
	c, e := s.Create("create", "1234567890123456", "key")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Claim(c.SessionID, c.JoinToken, "customer"); e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	expiry := s.rooms[c.SessionID].credential.ExpiresAt
	s.mu.Unlock()
	if !expiry.After(c.ExpiresAt.Add(time.Hour)) {
		t.Fatal("active session still bound to invitation TTL")
	}
}
