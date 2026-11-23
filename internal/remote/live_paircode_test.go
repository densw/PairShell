package remote

import (
	"context"
	"net/http"
	"os"
	"pairshell/internal/paircode"
	"testing"
	"time"
)

// Explicit opt-in: creates one disposable mailbox/room, never opens a remote shell.
func TestLiveDeviceCodeExchange(t *testing.T) {
	base := os.Getenv("PAIRSHELL_LIVE_TEST_SERVER")
	if base == "" {
		t.Skip("live test disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	code, result, err := paircode.Listen(ctx, base, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal("create mailbox:", paircode.Explain(err), err)
	}
	m, err := ConnectCode(ctx, base, code, "", &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal("exchange:", paircode.Explain(err), err)
	}
	defer m.c.end()
	select {
	case received := <-result:
		if received.Err != nil || received.Invite != m.Invite.URL() {
			t.Fatal("mailbox exchange did not deliver matching invitation", received.Err)
		}
	case <-ctx.Done():
		t.Fatal("mailbox exchange timeout")
	}
	t.Log("fresh device code exchanged successfully; no shell opened")
}
