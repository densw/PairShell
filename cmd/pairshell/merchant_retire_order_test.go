package main

import (
	"context"
	"golang.org/x/crypto/ssh"
	"sync"
	"testing"
)

type closeBeforeCancelConn struct {
	ssh.Conn
	ctx      context.Context
	observed chan error
	done     chan struct{}
	once     sync.Once
}

func (c *closeBeforeCancelConn) Close() error {
	c.once.Do(func() { c.observed <- c.ctx.Err(); close(c.done) })
	return nil
}
func (c *closeBeforeCancelConn) Wait() error { <-c.done; return nil }

func TestMerchantRetiresSSHBeforeCancelingCandidatePaths(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &closeBeforeCancelConn{ctx: ctx, observed: make(chan error, 1), done: make(chan struct{})}
	channels := make(chan ssh.NewChannel)
	close(channels)
	requests := make(chan *ssh.Request)
	close(requests)
	client := ssh.NewClient(conn, channels, requests)
	retireMerchantAttempt(&merchantAttempt{ctx: ctx, cancel: cancel, client: client})
	if err := <-conn.observed; err != nil {
		t.Fatalf("candidate context canceled before SSH close: %v", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("attempt context not canceled after SSH close")
	}
}
