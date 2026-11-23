package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRPCResponseLimits(t *testing.T) {
	r, err := http.NewRequest("POST", "http://customer/bridge/session_info", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		wire string
		ok   bool
	}{
		{"valid", "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}", true},
		{"declared body too large", "HTTP/1.1 200 OK\r\nContent-Length: 4096\r\n\r\n", false},
		{"header over budget", "HTTP/1.1 200 OK\r\nX-Fill: " + strings.Repeat("a", rpcHeaderLimit) + "\r\n\r\n{}", false},
		{"chunked body over budget", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\n12345\r\n0\r\n\r\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body, err := readRPCResponse(strings.NewReader(test.wire), r, 4)
			if test.ok && (err != nil || status != 200 || string(body) != "{}") {
				t.Fatalf("expected valid response, got %d %q %v", status, body, err)
			}
			if !test.ok && err == nil {
				t.Fatal("oversized response accepted")
			}
		})
	}
}
