package paircode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

type attemptFixtureTransport func(*http.Request) (*http.Response, error)

func (f attemptFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOwnerStopsAfterLocalProofLimit(t *testing.T) {
	for _, purpose := range []string{"device", "invite"} {
		t.Run(purpose, func(t *testing.T) {
			t.Parallel()
			ctx, stop := context.WithTimeout(context.Background(), 7*time.Second)
			defer stop()
			const base = "https://fixture.example"
			const secret = "123456"
			p, err := newPAKE(secret, 0, base, purpose)
			if err != nil {
				t.Fatal(err)
			}

			malformed := func(label string) response {
				if purpose == "device" {
					return response{Phase: 1, Preparation: []byte(label)}
				}
				return response{Phase: 2, Message: []byte(label)}
			}
			// The relay fixture has no attempt counter. After a reset it
			// replays a previous response, then provides later responses.
			first := malformed("malformed first proof")
			responses := []response{first, first}
			wantResets := maxLocalAttempts - 1
			if purpose == "device" {
				responses = append(responses, response{Expired: true})
				wantResets++
			}
			responses = append(responses, malformed("malformed second proof"), malformed("malformed third proof"))
			polls, resets := 0, 0
			client := &http.Client{Transport: attemptFixtureTransport(func(req *http.Request) (*http.Response, error) {
				var out response
				switch req.URL.Path {
				case "/v1/code/sender":
					if polls >= len(responses) {
						t.Fatal("owner asked the relay for another proof after exhausting the local limit")
					}
					out = responses[polls]
					polls++
				case "/v1/code/reset":
					resets++
				default:
					t.Fatalf("unexpected operation after malformed proofs: %s", req.URL.Path)
				}
				data, err := json.Marshal(out)
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}
			e, err := clientEndpoint(base, client)
			if err != nil {
				t.Fatal(err)
			}
			if purpose == "device" {
				var invite string
				invite, err = e.listen(ctx, p, secret)
				if invite != "" {
					t.Fatal("invalid proof delivered an invitation")
				}
			} else {
				err = e.publish(ctx, p, "not a real invitation", secret)
			}
			if !errors.Is(err, ErrCode) || polls != len(responses) || resets != wantResets || ctx.Err() != nil {
				t.Fatalf("owner stopped with err=%v, polls=%d, resets=%d, context=%v", err, polls, resets, ctx.Err())
			}
		})
	}
}
