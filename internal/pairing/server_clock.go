package pairing

import (
	"net/http"
	"sync"
	"time"
)

// ResponseTime reads the coordinator's precise time, with HTTP Date support for
// older coordinators. Callers must only use headers from their trusted server.
func ResponseTime(h http.Header) (time.Time, bool) {
	stamp, _, ok := responseTime(h)
	return stamp, ok
}

func responseTime(h http.Header) (time.Time, time.Duration, bool) {
	if stamp, err := time.Parse(time.RFC3339Nano, h.Get("X-PairShell-Time")); err == nil && !stamp.IsZero() {
		return stamp, 0, true
	}
	stamp, err := http.ParseTime(h.Get("Date"))
	return stamp, time.Second, err == nil && !stamp.IsZero()
}

// ServerClock advances trusted coordinator time with local monotonic elapsed
// time. Its zero value uses the local clock until a valid sample arrives.
// A ServerClock must not be copied after first use.
type ServerClock struct {
	mu     sync.Mutex
	server time.Time
	local  time.Time
}

// Observe accepts trusted response headers. Delayed or out-of-order samples
// cannot rewind an established reference and prolong expired authority.
func (c *ServerClock) Observe(h http.Header) {
	c.ObserveResponse(h, time.Now())
}

// ObserveResponse advances a trusted sample by the full request elapsed time
// and, for HTTP Date, its one-second truncation uncertainty. This conservative
// upper bound prevents network delay or coarse headers extending authority.
// sent should be captured with time.Now immediately before sending the request.
func (c *ServerClock) ObserveResponse(h http.Header, sent time.Time) {
	stamp, uncertainty, ok := responseTime(h)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(sent)
	if sent.IsZero() || elapsed < 0 {
		return
	}
	stamp = stamp.Add(elapsed).Add(uncertainty)
	if !c.local.IsZero() && !stamp.After(c.server.Add(now.Sub(c.local))) {
		return
	}
	c.server, c.local = stamp, now
}

func (c *ServerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.local.IsZero() {
		return now
	}
	return c.server.Add(now.Sub(c.local))
}
