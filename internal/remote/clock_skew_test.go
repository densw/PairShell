package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"pairshell/internal/paircode"
	"pairshell/internal/pairing"
	"pairshell/internal/server"
	"pairshell/internal/session"
	"testing"
	"time"
)

func TestLeaseIgnoresCustomerWallClockSkew(t *testing.T) {
	for _, skew := range []time.Duration{-48 * time.Hour, 48 * time.Hour} {
		for _, precise := range []bool{false, true} {
			t.Run(skew.String()+map[bool]string{false: "/legacy-date", true: "/precise-time"}[precise], func(t *testing.T) {
				requested := make(chan struct{}, 1)
				srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					stamp := time.Now().Add(skew)
					w.Header().Set("Date", stamp.UTC().Format(http.TimeFormat))
					if precise {
						w.Header().Set("X-PairShell-Time", stamp.In(time.FixedZone("UTC-11", -11*3600)).Format(time.RFC3339Nano))
					}
					json.NewEncoder(w).Encode(map[string]any{"expires_at": stamp.Add(30 * time.Second)})
					requested <- struct{}{}
				}))
				defer srv.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				g := session.New("clock-skew", "peer", time.Now().Add(time.Hour))
				if err := g.Consent(session.Capabilities{Tasks: true}, false); err != nil {
					t.Fatal(err)
				}
				stopped := make(chan struct{}, 1)
				go (&coordinator{base: srv.URL, id: "clock-skew", client: srv.Client()}).lease(ctx, func() { stopped <- struct{}{} }, g)
				select {
				case <-requested:
				case <-time.After(3 * time.Second):
					t.Fatal("lease request not received")
				}
				select {
				case <-stopped:
					t.Fatal("valid lease stopped due to wall-clock offset")
				case <-time.After(50 * time.Millisecond):
				}
				if g.Snapshot().State != "ACTIVE" {
					t.Fatal("authorized gate expired solely due to wall-clock offset")
				}
			})
		}
	}
}

func TestLeaseDeadlineBounds(t *testing.T) {
	stamp := time.Now().Add(-72 * time.Hour)
	sent := time.Now().Add(-2 * time.Second)
	until, err := (leaseReply{Expires: stamp.Add(time.Hour), serverTime: stamp}).deadline(sent)
	if err != nil || until.Sub(sent) != 30*time.Second {
		t.Fatalf("lease lifetime must be capped and charged from request start: %v %v", until, err)
	}
	for name, reply := range map[string]leaseReply{
		"missing-time":        {Expires: stamp.Add(time.Minute)},
		"missing-expiry":      {serverTime: stamp},
		"missing-expiry-date": {serverTime: stamp, uncertainty: time.Second},
		"expired":             {Expires: stamp, serverTime: stamp},
		"delayed":             {Expires: stamp.Add(time.Second), serverTime: stamp},
		"date-rounding":       {Expires: stamp.Add(time.Second), serverTime: stamp, uncertainty: time.Second},
	} {
		if _, err := reply.deadline(sent); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestLeaseDateFallbackChargesRounding(t *testing.T) {
	for _, header := range []string{"", "invalid", "0001-01-01T00:00:00Z"} {
		t.Run(header, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				stamp := time.Now().UTC().Truncate(time.Second)
				w.Header().Set("Date", stamp.Format(http.TimeFormat))
				w.Header().Set("X-PairShell-Time", header)
				json.NewEncoder(w).Encode(map[string]any{"expires_at": stamp.Add(500 * time.Millisecond)})
			}))
			defer srv.Close()
			c := coordinator{base: srv.URL, client: srv.Client()}
			sent := time.Now()
			var reply leaseReply
			if err := c.call(context.Background(), "/v1/lease", nil, &reply); err != nil {
				t.Fatal(err)
			}
			if _, err := reply.deadline(sent); err == nil {
				t.Fatal("legacy timestamp rounding extended lease authority")
			}
		})
	}
}

// The transport, PAKE, consent and pinned SSH exchange remain real. Only wire
// timestamps are shifted; neither the OS clock nor certificate checks change.
func shiftedCoordinator(skew time.Duration, precise bool) *httptest.Server {
	origin := server.New(server.Config{PublicDevicePairing: true, CreateToken: "deploy", RoomTTL: time.Minute})
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		origin.ServeHTTP(rec, r)
		for key, values := range rec.Header() {
			w.Header()[key] = values
		}
		stamp, _ := pairing.ResponseTime(rec.Header())
		stamp = stamp.Add(skew)
		w.Header().Set("Date", stamp.UTC().Format(http.TimeFormat))
		w.Header().Del("X-PairShell-Time")
		if precise {
			w.Header().Set("X-PairShell-Time", stamp.In(time.FixedZone("UTC+14", 14*3600)).Format(time.RFC3339Nano))
		}
		body := rec.Body.Bytes()
		if r.URL.Path == "/v1/rooms" || r.URL.Path == "/v1/lease" {
			var fields map[string]json.RawMessage
			if json.Unmarshal(body, &fields) == nil {
				var expires time.Time
				if json.Unmarshal(fields["expires_at"], &expires) == nil {
					fields["expires_at"], _ = json.Marshal(expires.Add(skew))
					body, _ = json.Marshal(fields)
				}
			}
		}
		w.WriteHeader(rec.Code)
		w.Write(body)
	}))
}

func TestPairCodeAndSSHIgnoreWallClockSkew(t *testing.T) {
	for _, backend := range []string{"webrtc", "tailcat"} {
		for _, precise := range []bool{false, true} {
			for _, skew := range []time.Duration{-48 * time.Hour, 48 * time.Hour} {
				t.Run(backend+"/"+skew.String()+map[bool]string{false: "/date", true: "/precise"}[precise], func(t *testing.T) {
					t.Setenv("PAIRSHELL_TRANSPORT", backend)
					if backend == "tailcat" {
						mapURL, _ := localDERPMap(t)
						t.Setenv("PAIRSHELL_DERP_MAP_URL", mapURL)
					}
					srv := shiftedCoordinator(skew, precise)
					defer srv.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					code, invites, err := paircode.Listen(ctx, srv.URL, srv.Client())
					if err != nil {
						t.Fatal(err)
					}
					m, err := ConnectCode(ctx, srv.URL, code, "deploy", srv.Client())
					if err != nil {
						t.Fatal(err)
					}
					result := <-invites
					if result.Err != nil {
						t.Fatal(result.Err)
					}
					verified := make(chan string, 1)
					done := make(chan error, 1)
					go func() {
						done <- ServeCustomer(ctx, result.Invite, srv.Client(), handler{}, func(tr pairing.Transcript, code string) (*session.Gate, error) {
							verified <- code
							g := session.New(tr.SessionID, "merchant", time.Now().Add(time.Hour))
							return g, g.Consent(session.Capabilities{Tasks: true}, false)
						})
					}()
					defer func() {
						cancel()
						select {
						case <-done:
						case <-time.After(3 * time.Second):
							t.Error("customer did not stop after cancellation")
						}
					}()
					client, err := m.Connect(ctx, func(_ pairing.Transcript, code string) error {
						select {
						case peer := <-verified:
							if !pairing.MatchVerificationCode(code, peer) {
								return errors.New("verification mismatch")
							}
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					})
					if err != nil {
						t.Fatal(err)
					}
					defer client.Close()
					s, err := client.NewSession()
					if err != nil {
						t.Fatal(err)
					}
					defer s.Close()
					out, err := s.Output("clock-independent")
					if err != nil || string(out) != "remote:clock-independent" {
						t.Fatalf("SSH after skewed pairing: %q %v", out, err)
					}
				})
			}
		}
	}
}

func TestExpiredInviteStillRejectedWithSkew(t *testing.T) {
	srv := shiftedCoordinator(-48*time.Hour, true)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := Create(ctx, srv.URL, "deploy", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	m.Invite.Expires = m.c.clock.Now().Add(-time.Minute).Unix()
	called := false
	err = ServeCustomer(ctx, m.Invite.URL(), srv.Client(), handler{}, func(pairing.Transcript, string) (*session.Gate, error) {
		called = true
		return nil, nil
	})
	if err == nil || called {
		t.Fatalf("expired invite reached consent: %v %v", err, called)
	}
}

func TestClockReferenceDoesNotBypassTLSValidation(t *testing.T) {
	srv := shiftedCoordinator(-48*time.Hour, true)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// This client has no trust anchor for the test server. A time reference must
	// never be learned by accepting an unverified certificate.
	if _, err := Create(ctx, srv.URL, "deploy", &http.Client{}); err == nil {
		t.Fatal("untrusted TLS connection accepted to obtain coordinator time")
	}
}

func TestLeaseExpiryStopsMerchantWithoutGate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stamp := time.Now().Add(48 * time.Hour)
		w.Header().Set("X-PairShell-Time", stamp.Format(time.RFC3339Nano))
		json.NewEncoder(w).Encode(map[string]any{"expires_at": stamp.Add(100 * time.Millisecond)})
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{}, 1)
	go (&coordinator{base: srv.URL, client: srv.Client()}).lease(ctx, func() { stopped <- struct{}{} }, nil)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("merchant continued beyond coordinator lease without a customer gate")
	}
}

func TestMalformedLeaseRevokesGate(t *testing.T) {
	for _, body := range []string{`{}`, `{"expires_at":"2000-01-01T00:00:00Z"}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
				io.WriteString(w, body)
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			g := session.New("lease", "peer", time.Now().Add(time.Hour))
			if err := g.Consent(session.Capabilities{Tasks: true}, false); err != nil {
				t.Fatal(err)
			}
			stopped := make(chan struct{}, 1)
			go (&coordinator{base: srv.URL, client: srv.Client()}).lease(ctx, func() { stopped <- struct{}{} }, g)
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("invalid lease retained authority")
			}
			if g.Snapshot().State != "ENDED" {
				t.Fatal("stopped lease left gate authorized")
			}
		})
	}
}
