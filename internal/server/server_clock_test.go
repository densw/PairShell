package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAllResponsesExposeCoordinatorTime(t *testing.T) {
	s := New(Config{MaxMessageBytes: 16})
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/healthz", ""},
		{"GET", "/missing", ""},
		{"POST", "/v1/rooms", "{}"},
		{"POST", "/v1/rooms", strings.Repeat("x", 17)},
		{"POST", "/v1/code/claim", "{}"},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			before := time.Now()
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			stamp, err := time.Parse(time.RFC3339Nano, w.Header().Get("X-PairShell-Time"))
			if err != nil || stamp.Before(before) || stamp.After(time.Now()) || stamp.Location() != time.UTC {
				t.Fatalf("invalid coordinator time %q: %v", w.Header().Get("X-PairShell-Time"), err)
			}
			date, err := http.ParseTime(w.Header().Get("Date"))
			if err != nil || date.Unix() != stamp.Unix() {
				t.Fatalf("inconsistent Date %q: %v", w.Header().Get("Date"), err)
			}
		})
	}
}
