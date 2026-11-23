package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReferenceRejectsTrailingJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ref")
	os.WriteFile(p, []byte(`{"url":"http://127.0.0.1:1234","token":"s"} {}`), 0600)
	if _, e := load(p); e == nil {
		t.Fatal("trailing reference data accepted")
	}
}
