package server

import "testing"

func TestPrivateServerDoesNotMintAnonymousDevicePermits(t *testing.T) {
	s := New(Config{CreateToken: "private-admin"})
	status, _ := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": []byte("untrusted PAKE")}})
	if status != 403 {
		t.Fatalf("private server exposed anonymous admission: status %d", status)
	}
	if _, err := s.Create("", "unauthorized-room-12345", "key"); err == nil {
		t.Fatal("private creation bypass")
	}
	if _, err := s.Create("private-admin", "authorized-room-123456", "key"); err != nil {
		t.Fatal(err)
	}
}
