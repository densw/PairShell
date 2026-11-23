package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMerchantFailedAttemptCanRetryInSameWindow(t *testing.T) {
	m := &merchantWindow{ctx: context.Background()}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "http://local/merchant/create", strings.NewReader(`{"Server":"http://invalid"}`))
		w := httptest.NewRecorder()
		m.serve(w, r)
		if w.Code != 200 {
			t.Fatalf("attempt %d locked: %d", i, w.Code)
		}
		deadline := time.Now().Add(time.Second)
		for {
			m.mu.Lock()
			failed := m.phase == "error"
			m.mu.Unlock()
			if failed {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("failure not reported")
			}
			time.Sleep(time.Millisecond)
		}
	}
}
func TestMerchantEndAllowsFreshAttemptWithoutCancelingWindow(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &merchantWindow{ctx: root}
	m.mu.Lock()
	first := m.beginAttemptLocked()
	m.mu.Unlock()
	r := httptest.NewRequest("POST", "http://local/merchant/end", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	m.serve(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if root.Err() != nil {
		t.Fatal("ending session canceled window")
	}
	m.mu.Lock()
	second := m.beginAttemptLocked()
	m.mu.Unlock()
	defer retireMerchantAttempt(second)
	if second.id <= first.id || second.ctx.Err() != nil {
		t.Fatal("second attempt was not fresh")
	}
	m.finishAttempt(first, "late old failure", "error")
	m.mu.Lock()
	if m.active != second {
		t.Fatal("old end interfered with new connection")
	}
	m.active = nil
	m.phase = "ended"
	m.mu.Unlock()
	get := httptest.NewRequest("GET", "http://local/merchant/state", nil)
	out := httptest.NewRecorder()
	m.serve(out, get)
	var state struct {
		Phase              string
		CanConnect, CanEnd bool
		AttemptID          uint64
	}
	if json.Unmarshal(out.Body.Bytes(), &state) != nil || !state.CanConnect || state.CanEnd || state.Phase != "ended" {
		t.Fatal(out.Body.String())
	}
}
func TestMerchantOldCallbackCannotOverwriteNewAttempt(t *testing.T) {
	m := &merchantWindow{ctx: context.Background()}
	m.mu.Lock()
	first := m.beginAttemptLocked()
	m.active = nil
	m.phase = "ended"
	second := m.beginAttemptLocked()
	m.mu.Unlock()
	defer retireMerchantAttempt(first)
	defer retireMerchantAttempt(second)
	m.finishAttempt(first, "old failed", "error")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != second || m.phase != "pairing" {
		t.Fatal("old callback replaced current session")
	}
	if second.ctx.Err() != nil {
		t.Fatal("new attempt inherited canceled context")
	}
}
