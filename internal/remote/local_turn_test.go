package remote

import (
	"context"
	"io"
	"net"
	"net/http/httptest"
	"pairshell/internal/server"
	"pairshell/internal/transport"
	"testing"
	"time"

	"github.com/pion/turn/v5"
)

func TestIssuedTURNCredentialsCarryRelayOnlyStream(t *testing.T) {
	const secret = "local TURN test secret"
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	relay, err := turn.NewServer(turn.ServerConfig{
		Realm:       "pairshell.test",
		AuthHandler: turn.LongTermTURNRESTAuthHandler(secret, nil),
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn: listener,
			RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP("127.0.0.1"),
				Address:      "127.0.0.1",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	coordinatorServer := server.New(server.Config{
		CreateToken: "test-admin",
		TURNSecret:  secret,
		TURNURLs:    []string{"turn:" + listener.LocalAddr().String() + "?transport=udp"},
	})
	s := httptest.NewTLSServer(coordinatorServer)
	defer s.Close()
	const id = "local-turn-room-1234"
	creds, err := coordinatorServer.Create("test-admin", id, "helper")
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinatorServer.Claim(id, creds.JoinToken, "customer"); err != nil {
		t.Fatal(err)
	}
	aCoord := coordinator{base: s.URL, client: s.Client(), id: id, token: creds.CreatorToken}
	bCoord := coordinator{base: s.URL, client: s.Client(), id: id, token: creds.JoinToken}
	for _, c := range []*coordinator{&aCoord, &bCoord} {
		if err := c.call(ctx, "/v1/ready", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, err := transport.NewPeer(id, 1, aCoord.servers(ctx), true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := transport.NewPeer(id, 1, bCoord.servers(ctx), true)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	offer, err := a.Offer(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := b.Answer(ctx, id, 1, offer)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetAnswer(answer); err != nil {
		t.Fatal(err)
	}
	aConn, err := a.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bConn, err := b.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deadline, _ := ctx.Deadline()
	aConn.SetDeadline(deadline)
	bConn.SetDeadline(deadline)
	const payload = "local TURN loopback regression"
	written := make(chan error, 1)
	go func() {
		_, err := aConn.Write([]byte(payload))
		written <- err
	}()
	out := make([]byte, len(payload))
	if _, err := io.ReadFull(bConn, out); err != nil || string(out) != payload {
		t.Fatalf("relay-only stream read got %q, error %v", out, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}
