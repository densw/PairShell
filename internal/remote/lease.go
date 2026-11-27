package remote

import (
	"errors"
	"time"
)

type leaseReply struct {
	Expires     time.Time `json:"expires_at"`
	serverTime  time.Time
	uncertainty time.Duration
}

func (r leaseReply) deadline(sent time.Time) (time.Time, error) {
	if r.serverTime.IsZero() {
		return time.Time{}, errors.New("LEASE_TIME_UNAVAILABLE")
	}
	ttl := r.Expires.Sub(r.serverTime)
	// Sub saturates for remote/zero timestamps. Validate before subtracting the
	// rounding margin so a missing expiry cannot underflow into a positive TTL.
	if ttl <= r.uncertainty {
		return time.Time{}, errors.New("LEASE_EXPIRED")
	}
	ttl -= r.uncertainty
	if ttl > 30*time.Second {
		ttl = 30 * time.Second
	}
	// sent retains the local monotonic clock. Charging the entire request latency
	// and legacy Date's one-second rounding prevents extending server authority.
	until := sent.Add(ttl)
	if !time.Now().Before(until) {
		return time.Time{}, errors.New("LEASE_EXPIRED")
	}
	return until, nil
}
