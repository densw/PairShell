package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient scopes additional CA trust to the exact configured service origin.
func HTTPClient(base string) (*http.Client, error) {
	cfg, e := LoadDefault()
	if e != nil {
		return nil, e
	}
	return clientFor(base, cfg)
}
func clientFor(base string, cfg Settings) (*http.Client, error) {
	normalized, e := ValidateServer(base)
	if e != nil {
		return nil, e
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if normalized == cfg.Server && cfg.TrustedCAPEM != "" {
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(cfg.TrustedCAPEM)) {
			return nil, errors.New("invalid configured CA certificate")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	u, _ := url.Parse(normalized)
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		transport.Proxy = nil
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !strings.EqualFold(req.URL.Scheme+"://"+req.URL.Host, via[0].URL.Scheme+"://"+via[0].URL.Host) {
			return errors.New("cross-origin redirect denied")
		}
		if len(via) > 3 {
			return errors.New("too many redirects")
		}
		return nil
	}}, nil
}
