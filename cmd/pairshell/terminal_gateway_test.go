package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/net/websocket"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/uiassets"
	"strings"
	"testing"
)

func TestPinnedTerminalAssetsIntegrity(t *testing.T) {
	manifest, e := uiassets.TerminalAssets.ReadFile("terminal-assets/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var entries []struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	}
	if e = json.Unmarshal(manifest, &entries); e != nil {
		t.Fatal(e)
	}
	if len(entries) < 7 {
		t.Fatal("terminal license and assets missing")
	}
	for _, entry := range entries {
		b, e := uiassets.TerminalAssets.ReadFile("terminal-assets/" + entry.File)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != entry.SHA256 {
			t.Fatalf("asset changed without provenance update: %s", entry.File)
		}
	}
}

func TestTerminalRejectsForeignOriginAndDisconnectedSession(t *testing.T) {
	d := &desktopServer{host: "127.0.0.1:1234", secret: "private", merchant: &merchantWindow{}}
	for _, origin := range []string{"http://evil.test", ""} {
		r := httptest.NewRequest("GET", "http://"+d.host+"/merchant/terminal", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		d.serveTerminal(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("origin %q accepted: %d", origin, w.Code)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(d.serveTerminal))
	defer srv.Close()
	d.host = strings.TrimPrefix(srv.URL, "http://")
	ws, e := websocket.Dial("ws://"+d.host+"/merchant/terminal", "", srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	if e = websocket.Message.Send(ws, `{"token":"private","cols":80,"rows":24}`); e != nil {
		t.Fatal(e)
	}
	var message string
	if e = websocket.Message.Receive(ws, &message); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(message, "连接尚未就绪") {
		t.Fatal(message)
	}
}

func TestTerminalRequiresFirstFrameToken(t *testing.T) {
	d := &desktopServer{secret: "private", merchant: &merchantWindow{}}
	srv := httptest.NewServer(http.HandlerFunc(d.serveTerminal))
	defer srv.Close()
	d.host = strings.TrimPrefix(srv.URL, "http://")
	ws, e := websocket.Dial("ws://"+d.host+"/merchant/terminal", "", srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	websocket.Message.Send(ws, `{"token":"wrong","cols":80,"rows":24}`)
	var message string
	if e = websocket.Message.Receive(ws, &message); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(message, "窗口认证失败") {
		t.Fatal(message)
	}
}

func TestTerminalPageHasPersistentTerminalWithoutCommandButton(t *testing.T) {
	page := merchantPage()
	if !strings.Contains(page, `id="terminal"`) {
		t.Fatal("terminal missing")
	}
	if strings.Contains(page, `id="execute"`) || strings.Contains(page, `id="command"`) {
		t.Fatal("per-command execution remains")
	}
}
