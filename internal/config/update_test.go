package config

import (
	"path/filepath"
	"testing"
)

func TestServerUpdatePreservesCAOnlyForSameOrigin(t *testing.T) {
	p := filepath.Join(t.TempDir(), "prefs", "config.json")
	Save(p, Settings{Server: "https://192.0.2.10:9443", TrustedCAPEM: "local public certificate"})
	if e := updateServer(p, "https://192.0.2.10:9443/"); e != nil {
		t.Fatal(e)
	}
	c, _ := Load(p)
	if c.TrustedCAPEM == "" {
		t.Fatal("unchanged origin lost trust")
	}
	if e := updateServer(p, "https://example.com"); e != nil {
		t.Fatal(e)
	}
	c, _ = Load(p)
	if c.TrustedCAPEM != "" {
		t.Fatal("trust moved to different origin")
	}
}
