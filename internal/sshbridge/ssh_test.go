package sshbridge

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"testing"
	"time"
)

type testHandler struct{}

func (testHandler) Run(s string, _ io.Reader, o, e io.Writer) (int, error) {
	io.WriteString(o, s)
	return 7, nil
}
func (testHandler) Shell(io.Reader, io.Writer, uint32, uint32, <-chan Size) (int, error) {
	return 0, nil
}
func TestPinnedExecAndDeniedForward(t *testing.T) {
	_, h, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(h)
	_, k, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := ssh.NewSignerFromKey(k)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	go func() { c, _ := listener.Accept(); Serve(c, host, key.PublicKey(), testHandler{}) }()
	c, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	client, e := Client(c, "test", key, host.PublicKey())
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	s, _ := client.NewSession()
	b, e := s.Output("hello")
	if string(b) != "hello" {
		t.Fatal(string(b), e)
	}
	exit, ok := e.(*ssh.ExitError)
	if !ok || exit.ExitStatus() != 7 {
		t.Fatal(e)
	}
	if _, e := client.Dial("tcp", "127.0.0.1:22"); e == nil {
		t.Fatal("forward permitted")
	}
}

func TestSSHRejectsKeysOutsideVerifiedPair(t *testing.T) {
	newKey := func(t *testing.T) ssh.Signer {
		t.Helper()
		_, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.NewSignerFromKey(private)
		if err != nil {
			t.Fatal(err)
		}
		return signer
	}
	host, merchant, unrelated := newKey(t), newKey(t), newKey(t)
	for _, tc := range []struct {
		name     string
		hostKey  ssh.PublicKey
		identity ssh.Signer
	}{
		{"different customer key", unrelated.PublicKey(), merchant},
		{"different merchant key", host.PublicKey(), unrelated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			served := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					conn.SetDeadline(time.Now().Add(3 * time.Second))
					err = Serve(conn, host, merchant.PublicKey(), testHandler{})
				}
				served <- err
			}()
			conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			client, err := Client(conn, "test", tc.identity, tc.hostKey)
			if err == nil {
				client.Close()
				t.Fatal("SSH accepted a key outside the verified pair")
			}
			conn.Close()
			if err := <-served; err == nil {
				t.Fatal("customer SSH server accepted the mismatched pair")
			}
		})
	}
}
