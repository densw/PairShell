package main

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCLIPeerVerificationRequiresFullPeerCode(t *testing.T) {
	const expected = "0123456789abcdef01234567"
	for _, tc := range []struct {
		name string
		in   string
		ok   bool
	}{
		{"same code", "0123456789ABCDEF01234567\r\n", true},
		{"old approval", "yes\n", false},
		{"incorrect code", "1123456789abcdef01234567\n", false},
		{"short code", "123456789012\n", false},
		{"no answer", "", false},
		{"oversized", strings.Repeat("a", 4096) + "\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var prompt bytes.Buffer
			err := verifyPeerFromInput(context.Background(), bufio.NewReader(strings.NewReader(tc.in)), &prompt, expected)
			if (err == nil) != tc.ok {
				t.Fatalf("verification error = %v, expected success = %v", err, tc.ok)
			}
			if !strings.Contains(prompt.String(), expected) {
				t.Fatal("session code was not displayed")
			}
		})
	}
}
