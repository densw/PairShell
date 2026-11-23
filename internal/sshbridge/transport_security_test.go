package sshbridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	probeInput  = "pairshell-regression-terminal-input-marker"
	probeOutput = "pairshell-regression-terminal-output-marker"
)

type terminalProbeHandler struct {
	dispatched chan<- string
}

func (h terminalProbeHandler) Run(command string, _ io.Reader, _, _ io.Writer) (int, error) {
	if h.dispatched != nil {
		h.dispatched <- command
	}
	return 0, nil
}

func (h terminalProbeHandler) Shell(in io.Reader, out io.Writer, _, _ uint32, _ <-chan Size) (int, error) {
	input := make([]byte, len(probeInput))
	if _, err := io.ReadFull(in, input); err != nil {
		return 1, err
	}
	if string(input) != probeInput {
		return 1, errors.New("unexpected test terminal input")
	}
	_, err := io.WriteString(out, probeOutput)
	return 0, err
}

type observedTransport struct {
	net.Conn
	mu          sync.Mutex
	sent        bytes.Buffer
	received    bytes.Buffer
	corruptNext atomic.Bool
	corrupted   atomic.Bool
}

func (c *observedTransport) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	c.received.Write(p[:n])
	c.mu.Unlock()
	return n, err
}

func (c *observedTransport) Write(p []byte) (int, error) {
	if len(p) != 0 && c.corruptNext.CompareAndSwap(true, false) {
		p = bytes.Clone(p)
		p[len(p)-1] ^= 1
		c.corrupted.Store(true)
	}
	n, err := c.Conn.Write(p)
	c.mu.Lock()
	c.sent.Write(p[:n])
	c.mu.Unlock()
	return n, err
}

func (c *observedTransport) snapshot() (sent, received []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.sent.Bytes()), bytes.Clone(c.received.Bytes())
}

func startObservedSSH(t *testing.T, handler Handler) (*ssh.Client, *observedTransport, <-chan error) {
	t.Helper()
	newSigner := func() ssh.Signer {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.NewSignerFromKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return signer
	}
	host, merchant := newSigner(), newSigner()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			err = conn.SetDeadline(time.Now().Add(10 * time.Second))
			if err != nil {
				_ = conn.Close()
			} else {
				err = Serve(conn, host, merchant.PublicKey(), handler)
			}
		}
		done <- err
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	wire := &observedTransport{Conn: conn}
	client, err := Client(wire, "test", merchant, host.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, wire, done
}

func TestTerminalInputAndOutputStayOffTheTransportInPlaintext(t *testing.T) {
	client, wire, _ := startObservedSSH(t, terminalProbeHandler{})
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 24, 80, nil); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	session.Stdin = strings.NewReader(probeInput)
	session.Stdout = &output
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err != nil || output.String() != probeOutput {
		t.Fatalf("terminal output = %q, wait error = %v", output.String(), err)
	}
	sent, received := wire.snapshot()
	for _, stream := range [][]byte{sent, received} {
		if len(stream) == 0 {
			t.Fatal("transport observer recorded no bytes")
		}
		if bytes.Contains(stream, []byte(probeInput)) || bytes.Contains(stream, []byte(probeOutput)) {
			t.Fatal("terminal plaintext appeared on the transport")
		}
	}
}

func TestEstablishedTransportCorruptionCannotDispatchCommand(t *testing.T) {
	dispatched := make(chan string, 1)
	client, wire, done := startObservedSSH(t, terminalProbeHandler{dispatched: dispatched})
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	wire.corruptNext.Store(true)
	if err := session.Start("pairshell-regression-command-marker"); err == nil {
		t.Fatal("customer accepted a command over corrupted SSH transport")
	}
	if !wire.corrupted.Load() {
		t.Fatal("no encrypted bytes were changed")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("customer did not close the corrupted connection")
	}
	select {
	case command := <-dispatched:
		t.Fatalf("customer dispatched %q after authentication failed", command)
	default:
	}
}
