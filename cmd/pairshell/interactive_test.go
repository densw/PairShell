package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInteractiveReferenceRejectsRemoteTarget(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	os.WriteFile(p, []byte(`{"url":"http://example.com:80","token":"secret"}`), 0600)
	if _, e := localSSHClient(context.Background(), p); e == nil {
		t.Fatal("nonlocal target accepted")
	}
}
