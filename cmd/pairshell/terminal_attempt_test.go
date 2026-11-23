package main

import (
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTerminalRejectsStaleAttemptEvenWithValidWindowToken(t *testing.T) {
	d := &desktopServer{secret: "window-token", merchant: &merchantWindow{client: &ssh.Client{}, state: "已连接", attemptID: 2}}
	server := httptest.NewServer(http.HandlerFunc(d.serveTerminal))
	defer server.Close()
	d.host = strings.TrimPrefix(server.URL, "http://")
	ws, e := websocket.Dial("ws://"+d.host+"/merchant/terminal", "", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	if e = websocket.Message.Send(ws, `{"token":"window-token","cols":80,"rows":24,"attempt_id":1}`); e != nil {
		t.Fatal(e)
	}
	var reply struct{ Error string }
	if e = websocket.JSON.Receive(ws, &reply); e != nil {
		t.Fatal(e)
	}
	if reply.Error == "" {
		t.Fatal("stale session admitted")
	}
	d.merchant.mu.Lock()
	defer d.merchant.mu.Unlock()
	if d.merchant.running {
		t.Fatal("stale websocket occupied current terminal")
	}
}
