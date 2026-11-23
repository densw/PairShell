package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReplayEscapesTerminalControls(t *testing.T) {
	var b bytes.Buffer
	p := []byte("hello\x1b[2J\u009b31m\n中文")
	n, e := (safeOutput{&b}).Write(p)
	if n != len(p) || e != nil {
		t.Fatal(n, e)
	}
	s := b.String()
	if strings.ContainsAny(s, "\x1b\u009b") || !strings.Contains(s, "中文") {
		t.Fatal("unsafe replay:", s)
	}
}
