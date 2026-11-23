package securefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveWrite(t *testing.T) {
	d := filepath.Join(t.TempDir(), "private")
	if e := Mkdir(d); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, "key")
	if e := Write(p, []byte("secret")); e != nil {
		t.Fatal(e)
	}
	if e := Write(p, []byte("replace")); e == nil {
		t.Fatal("existing key overwritten")
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "secret" {
		t.Fatalf("%q %v", b, e)
	}
	if e = verify(p, false); e != nil {
		t.Fatal(e)
	}
	if e = verify(d, true); e != nil {
		t.Fatal(e)
	}
}
func TestRefuseFileAsDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	os.WriteFile(p, nil, 0600)
	if e := Mkdir(p); e == nil {
		t.Fatal("accepted file")
	}
}
