package server

import (
	"testing"
	"time"
)

func TestDeviceRoomPermitIsSingleUseScopedAndExpires(t *testing.T) {
	s := New(Config{PublicDevicePairing: true, CreateToken: "admin"})
	permit, _ := approvedPermit(t, s)
	room := map[string]any{"session_id": "permit-room-123456789", "key": "key"}
	if n, _ := codeCall(t, s, "/v1/rooms", permit, room); n != 200 {
		t.Fatal("authorized permit rejected", n)
	}
	room["session_id"] = "permit-room-223456789"
	if n, _ := codeCall(t, s, "/v1/rooms", permit, room); n == 200 {
		t.Fatal("permit reused")
	}
	if _, err := s.Create("", "no-permit-room-123456789", "key"); err == nil {
		t.Fatal("global creation opened")
	}
	permit, id := approvedPermit(t, s)
	s.mu.Lock()
	s.deviceCodes[id].pairCode.claimExpires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	room["session_id"] = "expired-permit-123456789"
	if n, _ := codeCall(t, s, "/v1/rooms", permit, room); n == 200 {
		t.Fatal("expired permit")
	}
}

func approvedPermit(t *testing.T, s *Server) (string, string) {
	t.Helper()
	_, created := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": []byte("test initial message")}})
	status, claim := codeCall(t, s, "/v1/code/claim", "", map[string]any{"data": map[string]any{"locator": created["locator"]}})
	if status != 200 || claim["create_permit"] != nil {
		t.Fatal("mailbox claim exposed a room permit", status)
	}
	id, receiver, owner := claim["session_id"].(string), claim["token"].(string), created["token"].(string)
	request := map[string]any{"session_id": id}
	prepare := map[string]any{"session_id": id, "data": map[string]any{"message": []byte("test preparation")}}
	if n, _ := codeCall(t, s, "/v1/code/prepare", receiver, prepare); n != 200 {
		t.Fatal("prepare failed", n)
	}
	if n, _ := codeCall(t, s, "/v1/code/permit", receiver, request); n == 200 {
		t.Fatal("permit granted before owner authorization")
	}
	if n, _ := codeCall(t, s, "/v1/code/authorize", owner, request); n != 200 {
		t.Fatal("owner authorization failed", n)
	}
	n, obtained := codeCall(t, s, "/v1/code/permit", receiver, request)
	permit, _ := obtained["create_permit"].(string)
	if n != 200 || permit == "" {
		t.Fatal("owner-authorized permit was unavailable", n)
	}
	return permit, id
}
