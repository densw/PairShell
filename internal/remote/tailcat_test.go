package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"pairshell/internal/pairing"
	"tailscale.com/derp/derpserver"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/nettype"
)

func TestTailcatDescriptorConfidentiality(t *testing.T) {
	c := &coordinator{id: "room", secret: bytes.Repeat([]byte{1}, 32)}
	plain := "secret tunnel address"
	encrypted, err := c.sealTailcat(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte(plain)) {
		t.Fatal("descriptor leaked")
	}
	var got string
	if err := c.openTailcat(encrypted, &got); err != nil || got != plain {
		t.Fatalf("round trip: %q %v", got, err)
	}
	for _, bad := range []*coordinator{{id: "another-room", secret: c.secret}, {id: c.id, secret: bytes.Repeat([]byte{2}, 32)}} {
		if bad.openTailcat(encrypted, &got) == nil {
			t.Fatal("descriptor accepted wrong binding")
		}
	}
	encrypted[len(encrypted)-1] ^= 1
	if c.openTailcat(encrypted, &got) == nil {
		t.Fatal("tampered descriptor accepted")
	}
	if c.openTailcat(nil, &got) == nil {
		t.Fatal("truncated descriptor accepted")
	}
}

func localDERPMap(t *testing.T) (string, func()) {
	t.Helper()
	quiet := func(string, ...any) {}
	d := derpserver.New(key.NewNode(), quiet)
	relay := httptest.NewTLSServer(derpserver.Handler(d))
	stun, stop := stuntest.ServeWithPacketListener(t, nettype.Std{})
	dm := &tailcfg.DERPMap{Regions: map[tailcfg.DERPRegionID]*tailcfg.DERPRegion{1: {RegionID: 1, RegionCode: "test", Nodes: []*tailcfg.DERPNode{{Name: "t1", RegionID: 1, HostName: "127.0.0.1", IPv4: "127.0.0.1", IPv6: "none", STUNPort: stun.Port, DERPPort: relay.Listener.Addr().(*net.TCPAddr).Port, InsecureForTests: true, STUNTestIP: "127.0.0.1"}}}}}
	maps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(dm) }))
	t.Cleanup(func() { maps.Close(); relay.CloseClientConnections(); relay.Close(); d.Close(); stop() })
	return maps.URL, func() { d.Close(); relay.CloseClientConnections() }
}

func TestPairTailcatSSH(t *testing.T) {
	t.Setenv("PAIRSHELL_TRANSPORT", "tailcat")
	mapURL, stopRelay := localDERPMap(t)
	t.Setenv("PAIRSHELL_DERP_MAP_URL", mapURL)
	// The same pairing, consent and pinned SSH execution contract must hold with
	// a real WireGuard tunnel and a local DERP/STUN bootstrap (no public relays).
	testPairSSH(t, func(m *Merchant) {
		if !m.c.useTailcat {
			t.Fatal("tailcat was not negotiated")
		}
		deadline := time.Now().Add(10 * time.Second)
		for m.RouteStatus().Selected != "direct" {
			if time.Now().After(deadline) {
				t.Fatalf("no direct path: %+v", m.RouteStatus())
			}
			time.Sleep(50 * time.Millisecond)
		}
		stopRelay()
	})
}

func TestTailcatSetupCancellation(t *testing.T) {
	c := &coordinator{id: "room", role: "customer", secret: bytes.Repeat([]byte{1}, 32)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	// A cancelled coordinator request must fail promptly, before starting a tunnel.
	c.base = "http://127.0.0.1:1"
	c.client = http.DefaultClient
	if _, _, err := c.tailcatStream(ctx); err == nil {
		t.Fatal("cancelled setup succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("setup ignored cancellation")
	}
}

// A subprocess initializes magicsock's cached debug knob before any tunnel is
// created, exercising DERP-only behavior independently of the direct-path test.
func TestPairTailcatRelay(t *testing.T) {
	if os.Getenv("PAIRSHELL_TEST_DERP_ONLY") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestPairTailcatRelay$", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), "PAIRSHELL_TEST_DERP_ONLY=1", "TS_DEBUG_ALWAYS_USE_DERP=true")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("DERP-only subprocess: %v\n%s", err, out)
		}
		return
	}
	t.Setenv("PAIRSHELL_TRANSPORT", "tailcat")
	mapURL, _ := localDERPMap(t)
	t.Setenv("PAIRSHELL_DERP_MAP_URL", mapURL)
	testPairSSH(t, func(m *Merchant) {
		deadline := time.Now().Add(5 * time.Second)
		for m.RouteStatus().Selected == "connecting" && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		status := m.RouteStatus()
		if !m.c.useTailcat || status.Selected != "relay" || status.Direct.Available || !status.Relay.Available {
			t.Fatalf("DERP misreported: %+v", status)
		}
	})
}

func TestHandshakeRejectsInvalidTransport(t *testing.T) {
	t.Setenv("PAIRSHELL_TRANSPORT", "typo")
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := &coordinator{}
	if _, err := c.handshake(context.Background(), pairing.Invite{}, private); err == nil || err.Error() != "invalid PAIRSHELL_TRANSPORT" {
		t.Fatalf("invalid transport: %v", err)
	}
}
