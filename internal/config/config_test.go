package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultAndOverridePrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	s, e := Load(p)
	if e != nil || s.Server != DefaultServer {
		t.Fatal(s, e)
	}
	if e = Save(p, Settings{Server: "https://saved.example"}); e != nil {
		t.Fatal(e)
	}
	s, e = Load(p)
	if e != nil || s.Server != "https://saved.example" {
		t.Fatal(s, e)
	}
	for _, tc := range []struct{ flag, env, want string }{{"", "", "https://saved.example"}, {"", "https://environment.example", "https://environment.example"}, {"https://flag.example", "https://environment.example", "https://flag.example"}} {
		got, e := resolve(tc.flag, tc.env, p)
		if e != nil || got != tc.want {
			t.Fatal(got, e)
		}
	}
}
func TestRejectUnsafeServers(t *testing.T) {
	for _, raw := range []string{"http://relay.example", "https://user:secret@relay.example", "https://relay.example/#secret", "https://relay.example?token=x", "https://relay.example/path", "https://relay.example:abc", "https://relay.example:70000", "https://relay.example:", "https://relay.example#", "https://relay.example/%2f", "https://relay.example\r\nInjected: value", "https://", ""} {
		if _, e := ValidateServer(raw); e == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
func TestSaveReplacesWithoutSecretsAndRejectsInvalidUpdate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "config.json")
	if e := Save(p, Settings{Server: "https://first.example"}); e != nil {
		t.Fatal(e)
	}
	if e := Save(p, Settings{Server: "https://second.example/"}); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(p)
	if e != nil || !strings.Contains(string(raw), "https://second.example") {
		t.Fatal(string(raw), e)
	}
	if e := Save(p, Settings{Server: "http://bad.example"}); e == nil {
		t.Fatal("invalid save")
	}
	s, e := Load(p)
	if e != nil || s.Server != "https://second.example" {
		t.Fatal(s, e)
	}
}
func TestRejectUnknownOversizedAndTrailingConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	for _, body := range []string{`{"server":"https://relay.example","token":"secret"}`, `{"server":"https://relay.example"}{}`, strings.Repeat(" ", 5000)} {
		os.WriteFile(p, []byte(body), 0600)
		if _, e := Load(p); e == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
