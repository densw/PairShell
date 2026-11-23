//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavedSettingsAreCurrentUserOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "config.json")
	for _, server := range []string{"https://first.example", "https://second.example"} {
		if e := Save(p, Settings{Server: server}); e != nil {
			t.Fatal(e)
		}
		s, e := os.Stat(p)
		if e != nil || s.Mode().Perm() != 0600 {
			t.Fatal("configuration is not private", e)
		}
		d, e := os.Stat(filepath.Dir(p))
		if e != nil || d.Mode().Perm() != 0700 {
			t.Fatal("configuration directory is not private", e)
		}
	}
}
