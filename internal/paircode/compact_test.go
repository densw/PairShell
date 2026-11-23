package paircode

import (
	"strings"
	"testing"
)

func TestCompactCodeOnlyAndClipboardFormatting(t *testing.T) {
	for _, tc := range []struct{ code, locator, secret string }{
		{"1234-5678-9012", "123456", "789012"},
		{"123456789012", "123456", "789012"},
		{"\u200e１２３４–５６７８–９０１２\u200f", "123456", "789012"},
	} {
		locator, secret, err := parse(tc.code)
		if err != nil || locator != tc.locator || secret != tc.secret {
			t.Fatalf("parse failed for length %d", len(tc.code))
		}
	}
	for _, code := range []string{"12345678901", "1234567890123", "12345678901234", "123456-1234-5678", "123456789012345", "123456ABCDEF"} {
		if _, _, err := parse(code); err == nil {
			t.Fatal("invalid compact code accepted")
		}
	}
	for range 32 {
		secret, err := password()
		if err != nil || len(secret) != 6 {
			t.Fatal("six-digit secret generation failed")
		}
		code := displayCode("001234", secret)
		if len(strings.ReplaceAll(code, "-", "")) != 12 {
			t.Fatal("compact code length changed")
		}
		locator, recovered, err := parse(code)
		if err != nil || locator != "001234" || recovered != secret {
			t.Fatal("compact format lost digits")
		}
	}
}
