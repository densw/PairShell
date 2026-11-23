package pairing

import (
	"testing"
	"time"
)

func FuzzParseInvite(f *testing.F) {
	f.Add("")
	f.Add("https://example.com/pair#v=1")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ParseInvite(s, time.Unix(1, 0)) })
}
