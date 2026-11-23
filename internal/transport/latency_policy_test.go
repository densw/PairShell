package transport

import (
	"github.com/pion/webrtc/v4"
	"testing"
	"time"
)

func TestDirectCandidateNeverContainsTURNAndKeepsSTUN(t *testing.T) {
	servers := []webrtc.ICEServer{{URLs: []string{"turn:relay.example:3478", "stun:relay.example:3478"}, Username: "user", Credential: "secret"}}
	filtered := directServers(servers)
	if len(filtered) != 1 || len(filtered[0].URLs) != 1 || filtered[0].URLs[0] != "stun:relay.example:3478" {
		t.Fatalf("direct candidate contains relay or lacks STUN: %+v", filtered)
	}
}
func TestLatencySelectionHasNoTransportPreference(t *testing.T) {
	for _, initial := range []string{"direct", "relay"} {
		s := NewSelector(initial)
		other := "relay"
		if initial == "relay" {
			other = "direct"
		}
		now := time.Now()
		for i := 0; i < 2; i++ {
			if s.Observe(other, 20*time.Millisecond, 80*time.Millisecond, now.Add(time.Duration(i)*time.Second)) {
				t.Fatal("switched before hysteresis")
			}
		}
		if !s.Observe(other, 20*time.Millisecond, 80*time.Millisecond, now.Add(2*time.Second)) {
			t.Fatal("lower RTT transport was not selected", initial)
		}
	}
}

func TestSmallLatencyAdvantageSelectsEitherTransport(t *testing.T) {
	for _, initial := range []string{"direct", "relay"} {
		t.Run(initial, func(t *testing.T) {
			s := NewSelector(initial)
			other := "relay"
			if initial == "relay" {
				other = "direct"
			}
			now := time.Now()
			for i := 0; i < 2; i++ {
				if s.Observe(other, 3*time.Millisecond, 9*time.Millisecond, now.Add(time.Duration(i)*time.Second)) {
					t.Fatal("switched before stability window")
				}
			}
			if !s.Observe(other, 3*time.Millisecond, 9*time.Millisecond, now.Add(2*time.Second)) {
				t.Fatal("lower latency path blocked by absolute threshold")
			}
		})
	}
}

func TestEqualSlowerOrInvalidLatencyDoesNotSwitch(t *testing.T) {
	for _, candidate := range []time.Duration{0, -time.Millisecond, 9 * time.Millisecond, 10 * time.Millisecond} {
		s := NewSelector("direct")
		for i := 0; i < 4; i++ {
			if s.Observe("relay", candidate, 9*time.Millisecond, time.Now().Add(time.Duration(i)*time.Second)) {
				t.Fatalf("switched to RTT %v", candidate)
			}
		}
	}
}
