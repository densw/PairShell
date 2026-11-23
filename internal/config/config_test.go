package config

import (
	"encoding/json"
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

func TestLegacyTestServerIsNotAnImplicitCustomDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"server":"https://192.168.31.178:9443","trusted_ca_pem":"old test certificate"}`), 0600)
	cfg, err := Load(p)
	if err != nil || cfg.Server != "" || cfg.TrustedCAPEM != "" || cfg.ForceCustom {
		t.Fatal("old test configuration must default to official service", cfg, err)
	}
	if err = Save(p, Settings{Server: "https://192.168.31.178:9443"}); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(p)
	if err != nil || cfg.Server != "https://192.168.31.178:9443" {
		t.Fatal("explicit user settings must remain usable", cfg, err)
	}
}

func TestFreshProfileHasNoCustomServer(t *testing.T) {
	p := filepath.Join(t.TempDir(), "PairShell", "config.json")
	cfg, err := Load(p)
	if err != nil || cfg != (Settings{}) {
		t.Fatal("fresh profiles must have no custom server or trust", cfg, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("reading defaults must not create user preferences", err)
	}
}

func TestLegacyLANServerVariantsAndExplicitPreference(t *testing.T) {
	for _, server := range []string{"https://192.168.31.178", "https://192.168.31.178/", "https://192.168.31.178:9443", "https://192.168.31.178:9443/"} {
		t.Run(server, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			for _, force := range []bool{false, true} {
				for _, configured := range []bool{false, true} {
					body, err := json.Marshal(Settings{Server: server, ForceCustom: force, ServerConfigured: configured, TrustedCAPEM: "legacy certificate"})
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, body, 0600); err != nil {
						t.Fatal(err)
					}
					cfg, err := Load(p)
					if err != nil {
						t.Fatal(err)
					}
					if !configured && cfg != (Settings{}) {
						t.Fatal("legacy test preferences must be cleared", cfg)
					}
					if configured && (cfg.Server != strings.TrimSuffix(server, "/") || cfg.ForceCustom != force || cfg.TrustedCAPEM != "legacy certificate") {
						t.Fatal("explicit preferences must remain usable", cfg)
					}
				}
			}
		})
	}
}
