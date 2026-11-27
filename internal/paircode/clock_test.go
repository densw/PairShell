package paircode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/pairing"
	"testing"
	"time"
)

func TestOpenRejectsInviteExpiredDuringResponse(t *testing.T) {
	stamp := time.Date(2026, 11, 27, 8, 0, 0, 900000000, time.UTC)
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-PairShell-Time", stamp.Format(time.RFC3339Nano))
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("{}"))
	}))
	defer s.Close()
	e, err := clientEndpoint(s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.call(context.Background(), "/sample", nil); err != nil {
		t.Fatal(err)
	}
	i, _, err := pairing.NewInvite(s.URL, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	i.Expires = stamp.Truncate(time.Second).Add(time.Second).Unix()
	key := make([]byte, 32)
	sealed, err := e.seal(key, i.URL())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.open(key, sealed); err == nil {
		t.Fatal("response latency extended invitation authority")
	}
}

func TestOpenUsesObservedCoordinatorTime(t *testing.T) {
	for _, offset := range []time.Duration{-48 * time.Hour, 48 * time.Hour} {
		t.Run(offset.String(), func(t *testing.T) {
			now := time.Now().Add(offset)
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-PairShell-Time", now.Format(time.RFC3339Nano))
				w.Write([]byte("{}"))
			}))
			defer s.Close()
			e, err := clientEndpoint(s.URL, s.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = e.call(context.Background(), "/sample", nil); err != nil {
				t.Fatal(err)
			}
			i, _, err := pairing.NewInvite(s.URL, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			key := make([]byte, 32)
			for _, expired := range []bool{false, true} {
				i.Expires = now.Add(time.Minute).Unix()
				if expired {
					i.Expires = now.Add(-time.Minute).Unix()
				}
				sealed, err := e.seal(key, i.URL())
				if err != nil {
					t.Fatal(err)
				}
				_, err = e.open(key, sealed)
				if (err != nil) != expired {
					t.Fatalf("expired=%v: %v", expired, err)
				}
			}
		})
	}
}
