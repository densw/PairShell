package sshbridge

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"net"
	"testing"
)

func TestNativeProxyInnerPinnedSSH(t *testing.T) {
	_, h, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(h)
	_, k, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := ssh.NewSignerFromKey(k)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() { c, _ := l.Accept(); Serve(c, host, key.PublicKey(), testHandler{}) }()
	raw, _ := net.Dial("tcp", l.Addr().String())
	outer, e := Client(raw, "test", key, host.PublicKey())
	if e != nil {
		t.Fatal(e)
	}
	defer outer.Close()
	ch, req, e := outer.OpenChannel("pairshell-proxy", nil)
	if e != nil {
		t.Fatal(e)
	}
	go ssh.DiscardRequests(req)
	inner, e := Client(&channelConn{Channel: ch, conn: raw}, "test", key, host.PublicKey())
	if e != nil {
		t.Fatal(e)
	}
	defer inner.Close()
	s, e := inner.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := s.Output("native")
	if string(b) != "native" {
		t.Fatal(string(b))
	}
	if nested, _, e := inner.OpenChannel("pairshell-proxy", nil); e == nil {
		nested.Close()
		t.Fatal("recursive proxy allowed")
	}
}
