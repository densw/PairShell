package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopFileScopeRequiresDedicatedDirectory(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	share := filepath.Join(home, "share")
	app := filepath.Join(home, "settings", "PairShell")
	logs := filepath.Join(app, "Sessions")
	for _, path := range []string{share, logs} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"", "relative", home, filepath.Dir(home), filepath.Dir(app), app, logs} {
		pinned := false
		_, err := selectDesktopFileRoot(path, home, app, func(string, os.FileInfo) error { pinned = true; return nil })
		if err == nil || pinned {
			t.Fatalf("protected or invalid folder accepted: %q", path)
		}
	}
	var pinned string
	got, err := selectDesktopFileRoot(share, home, app, func(path string, _ os.FileInfo) error { pinned = path; return nil })
	if err != nil || got != share || pinned != share {
		t.Fatalf("dedicated folder was not selected: %q %q %v", got, pinned, err)
	}
}
