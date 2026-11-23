package sshbridge

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"testing"
)

type rpcHandler struct{ testHandler }

func (rpcHandler) RPC(ch io.ReadWriteCloser) error {
	b := make([]byte, 4)
	_, e := io.ReadFull(ch, b)
	if e != nil {
		return e
	}
	_, e = ch.Write(b)
	return e
}
func TestExplicitRPCChannel(t *testing.T) {
	_, h, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(h)
	_, k, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := ssh.NewSignerFromKey(k)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() { c, _ := l.Accept(); Serve(c, host, key.PublicKey(), rpcHandler{}) }()
	raw, _ := net.Dial("tcp", l.Addr().String())
	client, e := Client(raw, "test", key, host.PublicKey())
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	ch, req, e := client.OpenChannel("pairshell-rpc", nil)
	if e != nil {
		t.Fatal(e)
	}
	go ssh.DiscardRequests(req)
	defer ch.Close()
	ch.Write([]byte("test"))
	b := make([]byte, 4)
	if _, e = io.ReadFull(ch, b); e != nil || string(b) != "test" {
		t.Fatal(string(b), e)
	}
}
