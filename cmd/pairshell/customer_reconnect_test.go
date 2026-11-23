package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func customerRequest(d *desktopServer, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://"+d.host+path, strings.NewReader("{}"))
	r.Header.Set("Origin", "http://"+d.host)
	r.Header.Set("X-PairShell", d.secret)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	return w
}
func TestCustomerCanStartAgainAfterFailedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &desktopServer{ctx: ctx, host: "127.0.0.1:1234", secret: "window"}
	calls := make(chan string, 2)
	d.run = func(ctx context.Context, invite string, ready func(string, string)) error {
		calls <- invite
		return errors.New("pairing failed")
	}
	for _, invite := range []string{"first", "second"} {
		if !d.launchCustomer(invite) {
			t.Fatal("session rejected", invite)
		}
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatal("not started")
		}
		deadline := time.Now().Add(time.Second)
		for {
			d.mu.Lock()
			active := d.started
			d.mu.Unlock()
			if !active {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("window stuck active")
			}
			time.Sleep(time.Millisecond)
		}
	}
}
func TestCustomerResetWaitsForOldSessionCleanupAndIgnoresLateReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &desktopServer{ctx: ctx, host: "127.0.0.1:1234", secret: "window"}
	entered := make(chan struct{})
	release := make(chan struct{})
	d.run = func(ctx context.Context, _ string, ready func(string, string)) error {
		close(entered)
		<-ctx.Done()
		ready("late-control", "late-code")
		<-release
		return ctx.Err()
	}
	if !d.launchCustomer("one") {
		t.Fatal("start")
	}
	<-entered
	if w := customerRequest(d, "/reset"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if d.launchCustomer("overlap") {
		t.Fatal("started before cleanup")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		d.mu.Lock()
		active := d.started
		control := d.control
		d.mu.Unlock()
		if !active {
			if control != "" {
				t.Fatal("stale control survived")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not release window")
		}
		time.Sleep(time.Millisecond)
	}
}
