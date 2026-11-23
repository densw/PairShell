package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"pairshell/internal/pairing"
	"pairshell/internal/remote"
)

func connectByCode(ctx context.Context, base, code, reference string, headless bool, input *bufio.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m, e := remote.ConnectCode(ctx, base, code, os.Getenv("PAIRSHELL_CREATE_TOKEN"), nil)
	if e != nil {
		return e
	}
	var peer []byte
	client, e := m.Connect(ctx, func(t pairing.Transcript, verification string) error {
		if err := verifyPeerFromInput(ctx, input, os.Stderr, verification); err != nil {
			return err
		}
		peer = append([]byte(nil), t.CustomerKey...)
		fmt.Fprintln(os.Stderr, "身份已验证，等待对方允许本次协助…")
		return nil
	})
	if e != nil {
		return e
	}
	defer client.Close()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- serveMerchantAdapters(ctx, client, m, peer, reference, func(ref string) { ready <- ref })
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-done:
		return e
	case ref := <-ready:
		fmt.Fprintf(os.Stderr, "MCP: pairshell mcp --session %q\n终端: pairshell shell --session %q\n", ref, ref)
	}
	if headless {
		select {
		case <-ctx.Done():
			return nil
		case e := <-done:
			return e
		}
	}
	return interactiveShell(ctx, client)
}
