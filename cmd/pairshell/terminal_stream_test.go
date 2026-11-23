package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/sshbridge"
	"strings"
	"testing"
	"time"
)

type terminalGatewayHandler struct {
	input  chan []byte
	resize chan sshbridge.Size
	size   int
}

func (h terminalGatewayHandler) Run(string, io.Reader, io.Writer, io.Writer) (int, error) {
	return 255, io.ErrUnexpectedEOF
}
func (h terminalGatewayHandler) Shell(in io.Reader, out io.Writer, w, height uint32, changes <-chan sshbridge.Size) (int, error) {
	io.WriteString(out, "ready>")
	b := make([]byte, h.size)
	if _, e := io.ReadFull(in, b); e != nil {
		return 255, e
	}
	h.input <- b
	select {
	case size := <-changes:
		h.resize <- size
	case <-time.After(5 * time.Second):
		return 255, io.ErrUnexpectedEOF
	}
	io.WriteString(out, "done")
	return 0, nil
}
func TestTerminalStreamsKeystrokesAndResizeOverOneSSHPTY(t *testing.T) {
	_, hostKey, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(hostKey)
	_, identityKey, _ := ed25519.GenerateKey(rand.Reader)
	identity, _ := ssh.NewSignerFromKey(identityKey)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	payload := "echo hello\r\x03\x1b[A\t中文"
	handler := terminalGatewayHandler{make(chan []byte, 1), make(chan sshbridge.Size, 1), len(payload)}
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			sshbridge.Serve(conn, host, identity.PublicKey(), handler)
		}
	}()
	conn, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	client, e := sshbridge.Client(conn, "test", identity, host.PublicKey())
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	d := &desktopServer{secret: "private", merchant: &merchantWindow{client: client, state: "已连接"}}
	srv := httptest.NewServer(http.HandlerFunc(d.serveTerminal))
	defer srv.Close()
	d.host = strings.TrimPrefix(srv.URL, "http://")
	ws, e := websocket.Dial("ws://"+d.host+"/merchant/terminal", "", srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	ws.SetDeadline(time.Now().Add(10 * time.Second))
	if e = websocket.JSON.Send(ws, struct {
		Token      string `json:"token"`
		Cols, Rows int
	}{"private", 80, 24}); e != nil {
		t.Fatal(e)
	}
	var sawReady, sawPrompt bool
	for i := 0; i < 2; i++ {
		var output []byte
		if e = websocket.Message.Receive(ws, &output); e != nil {
			t.Fatal(e)
		}
		var status struct {
			Ready bool `json:"ready"`
		}
		if json.Unmarshal(output, &status) == nil && status.Ready {
			sawReady = true
		} else if string(output) == "ready>" {
			sawPrompt = true
		} else {
			t.Fatal("unexpected terminal frame", string(output))
		}
	}
	if !sawReady || !sawPrompt {
		t.Fatal("missing protocol readiness or real PTY prompt")
	}
	if e = websocket.JSON.Send(ws, map[string]any{"type": "input", "data": payload}); e != nil {
		t.Fatal(e)
	}
	if e = websocket.JSON.Send(ws, map[string]any{"type": "resize", "cols": 120, "rows": 40}); e != nil {
		t.Fatal(e)
	}
	select {
	case got := <-handler.input:
		if string(got) != payload {
			t.Fatalf("keystrokes changed: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("input timeout")
	}
	select {
	case got := <-handler.resize:
		if got.Width != 120 || got.Height != 40 {
			t.Fatalf("wrong resize: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resize timeout")
	}
}
