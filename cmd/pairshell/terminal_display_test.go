package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestTerminalDisplaysKeepEachStreamIndependent(t *testing.T) {
	var out bytes.Buffer
	stdout, stderr := terminalDisplays(&out, &out)
	if _, err := io.WriteString(stdout, "\x1b]placeholder"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stderr, "diagnostic"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdout, "\a visible"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "diagnostic visible" {
		t.Fatalf("separate SSH output streams produced %q", got)
	}
}

func TestTerminalDisplayStreaming(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "PTY formatting and Unicode",
			in:   "你好 \x1b[31m彩色\x1b[0m\r\n\x1b[?1049h\x1b[2J\x1b[4;5H\x1b(0\x1b(B\x1b[?1049l",
			want: "你好 \x1b[31m彩色\x1b[0m\r\n\x1b[?1049h\x1b[2J\x1b[4;5H\x1b(0\x1b(B\x1b[?1049l",
		},
		{
			name: "control strings and terminators",
			in:   "a\x1b]52;placeholder\ab\x1b]8;placeholder\x1b\\c\x1bPplaceholder\x1b\\d\x1b_placeholder\x1b\\e\x1b^placeholder\x1b\\f\x1bXplaceholder\x1b\\g",
			want: "abcdefg",
		},
		{
			name: "UTF-8 and single-byte C1",
			in:   "好\u009dplaceholder\u009c字\x9dplaceholder\x9c尾\u009b31m\x9b0m",
			want: "好字尾\x1b[31m\x1b[0m",
		},
		{
			name: "hold incomplete controls until session closes",
			in:   "visible\x1b]placeholder",
			want: "visible",
		},
		{
			name: "window and printer commands and device attributes",
			in:   "a\x1b[1tb\x1b[5ic\x1b[cd\x1b[xe\x1b[6n",
			want: "abcde\x1b[6n",
		},
		{
			name: "bounded parameter state and clean recovery",
			in:   "a\x1b[" + strings.Repeat("1", 160) + "mb\x1b[0m",
			want: "ab\x1b[0m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for chunkSize := 1; chunkSize <= len(tt.in); chunkSize++ {
				var out bytes.Buffer
				w := terminalDisplay{output: &out}
				for start := 0; start < len(tt.in); start += chunkSize {
					end := min(start+chunkSize, len(tt.in))
					if n, err := w.Write([]byte(tt.in[start:end])); err != nil || n != end-start {
						t.Fatalf("chunk size %d: consumed %d/%d, error %v", chunkSize, n, end-start, err)
					}
				}
				if got := out.String(); got != tt.want {
					t.Fatalf("chunk size %d: got %q, want %q", chunkSize, got, tt.want)
				}
			}
		})
	}
}
