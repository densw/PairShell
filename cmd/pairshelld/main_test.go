package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/audit"
	"testing"
)

func TestWitnessRejectUnsigned(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	w, _ := audit.OpenWitness(t.TempDir(), key)
	h := witnessHandler(w, "secret")
	r := httptest.NewRequest("POST", "/witness/checkpoint", nil)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code != http.StatusUnauthorized {
		t.Fatal(out.Code)
	}
}
