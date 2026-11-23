package session

import (
	"sync"
	"testing"
	"time"
)

func TestRevokeSerializesAdmission(t *testing.T) {
	g := New("s", "p", time.Now().Add(time.Hour))
	g.Consent(Capabilities{Tasks: true}, true)
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); g.WithAuthorization(1, "tasks", true, func() error { return nil }) }()
	}
	g.End()
	wg.Wait()
	if g.WithAuthorization(1, "tasks", true, func() error { t.Fatal("post-revoke admission"); return nil }) == nil {
		t.Fatal("revoked accepted")
	}
}
func TestAuditFailureCannotResume(t *testing.T) {
	g := New("s", "p", time.Now().Add(time.Hour))
	g.Consent(Capabilities{Terminal: true}, false)
	g.AuditFailure()
	if g.Resume() == nil {
		t.Fatal("audit bypass")
	}
}
func TestLeaseExpiryCannotRenew(t *testing.T) {
	g := New("s", "p", time.Now().Add(time.Hour))
	g.RenewLease(time.Now().Add(-time.Second))
	if g.RenewLease(time.Now().Add(time.Second)) == nil {
		t.Fatal("expired lease resurrected")
	}
}

func TestPassiveChecksAndLeaseRenewalDoNotResetIdleTimeout(t *testing.T) {
	g := New("s", "p", time.Now().Add(time.Hour))
	if err := g.Consent(Capabilities{Terminal: true}, false); err != nil {
		t.Fatal(err)
	}
	g.lastActivity = time.Now().Add(-DefaultIdleTimeout + time.Second)
	last := g.lastActivity
	if err := g.Check(1, "terminal", false); err != nil {
		t.Fatal(err)
	}
	if err := g.RenewLease(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if !g.lastActivity.Equal(last) {
		t.Fatal("passive check or lease renewal reset inactivity")
	}
	if err := g.WithAuthorization(1, "terminal", false, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !g.lastActivity.After(last) {
		t.Fatal("accepted terminal operation did not refresh inactivity")
	}
	g.lastActivity = time.Now().Add(-DefaultIdleTimeout)
	if got := g.Snapshot(); got.State != "ENDED" || got.AuthorizationEpoch != 2 {
		t.Fatalf("idle session stayed accessible: %+v", got)
	}
}

func TestStagedFileAuthorizationStaysCanceledAfterResume(t *testing.T) {
	g := New("s", "p", time.Now().Add(time.Hour))
	if err := g.Consent(Capabilities{FileRead: true}, true); err != nil {
		t.Fatal(err)
	}
	first, err := g.StagedAuthorization(1, "file_read", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = g.Pause(); err != nil {
		t.Fatal(err)
	}
	if err = g.Resume(); err != nil {
		t.Fatal(err)
	}
	second, err := g.StagedAuthorization(1, "file_read", true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Err() == nil || second.Err() != nil {
		t.Fatal("resume revived an old permit or did not issue a new one")
	}
}

func TestCustomIdleDurationOnlyAppliesAfterConsent(t *testing.T) {
	g := NewWithIdleTimeout("s", "p", time.Now().Add(time.Hour), time.Second)
	if got := g.Snapshot().State; got != "AWAITING_CONSENT" {
		t.Fatal(got)
	}
	if err := g.Consent(Capabilities{Terminal: true}, false); err != nil {
		t.Fatal(err)
	}
	g.lastActivity = time.Now().Add(-2 * time.Second)
	if got := g.Snapshot().State; got != "ENDED" {
		t.Fatal(got)
	}
}
