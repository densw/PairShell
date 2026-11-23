package config

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivateCATrustIsScopedToConfiguredServer(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw})
	cfg := Settings{Server: s.URL, TrustedCAPEM: string(cert)}
	c, e := clientFor(s.URL, cfg)
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Get(s.URL)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	other := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer other.Close()
	c, e = clientFor(other.URL, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Get(other.URL); e == nil {
		t.Fatal("custom CA leaked to different origin")
	}
	_ = x509.NewCertPool()
}
