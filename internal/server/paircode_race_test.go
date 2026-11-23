package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDeviceCodeClaimIsAtomic(t *testing.T) {
	s := New(Config{PublicDevicePairing: true})
	status, v := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": []byte("public PAKE")}})
	if status != 200 {
		t.Fatal(status)
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"locator": v["locator"]}})
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/v1/code/claim", bytes.NewReader(body))
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code == 200 {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d claimants", accepted.Load())
	}
}
func TestDeviceCodeCapacityAndMalformedMessage(t *testing.T) {
	s := New(Config{PublicDevicePairing: true, MaxRooms: 1})
	for i := 0; i < 2; i++ {
		status, _ := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": []byte("PAKE")}})
		if (status == 200) != (i == 0) {
			t.Fatalf("capacity status %d", status)
		}
	}
	if status, _ := codeCall(t, s, "/v1/device/create", "", map[string]any{"data": map[string]any{"message": make([]byte, 9000)}}); status == 200 {
		t.Fatal("oversized message accepted")
	}
}
