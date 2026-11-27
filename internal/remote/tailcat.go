package remote

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/tailscale/tailcat"
	"pairshell/internal/transport"
	"tailscale.com/types/key"
)

// The address contains a WireGuard PSK. Encrypt it separately from the signal
// MAC so the coordinator cannot learn the tunnel admission secret.
func (c *coordinator) tailcatCipher() cipher.AEAD {
	k := sha256.Sum256(append([]byte("PairShell tailcat descriptor v1\x00"), c.secret...))
	block, _ := aes.NewCipher(k[:])
	aead, _ := cipher.NewGCM(block)
	return aead
}
func (c *coordinator) sealTailcat(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	a := c.tailcatCipher()
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, b, []byte(c.id)), nil
}
func (c *coordinator) openTailcat(b []byte, v any) error {
	a := c.tailcatCipher()
	if len(b) < a.NonceSize() {
		return errors.New("invalid tailcat descriptor")
	}
	plain, err := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte(c.id))
	if err != nil {
		return errors.New("invalid tailcat descriptor")
	}
	return json.Unmarshal(plain, v)
}
func (c *coordinator) sendTailcat(ctx context.Context, kind string, v any) error {
	b, err := c.sealTailcat(v)
	if err != nil {
		return err
	}
	return c.send(ctx, kind, b)
}
func (c *coordinator) receiveTailcat(ctx context.Context, kind string, v any) error {
	var b []byte
	if err := c.receive(ctx, kind, &b); err != nil {
		return err
	}
	return c.openTailcat(b, v)
}

type tailcatOffer struct {
	Address tailcat.Addr
	Port    uint16
}
type tailcatClient struct{ Key key.NodePublic }

// tailcatStream uses a single reliable TCP stream over WireGuard. Magicsock
// maintains the UDP/DERP path underneath it, without replaying SSH commands.
func (c *coordinator) tailcatStream(ctx context.Context) (net.Conn, func(), error) {
	setup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	quiet := func(string, ...any) {}
	mapURL := os.Getenv("PAIRSHELL_DERP_MAP_URL")
	var stream net.Conn
	var monitorClient *tailcat.Client
	var closeBackend func()
	var err error
	if c.role == "merchant" {
		private := key.NewNode()
		if err = c.sendTailcat(setup, "tailcat-client", tailcatClient{private.Public()}); err != nil {
			return nil, nil, err
		}
		var offer tailcatOffer
		if err = c.receiveTailcat(setup, "tailcat-offer", &offer); err != nil {
			return nil, nil, err
		}
		if offer.Port == 0 {
			return nil, nil, errors.New("invalid tailcat port")
		}
		client := &tailcat.Client{Server: offer.Address, Key: private, DERPMapURL: mapURL, Logf: quiet}
		stream, err = client.DialTCPPort(setup, offer.Port)
		closeBackend = func() { client.Close() }
		if err == nil {
			monitorClient = client
		}
	} else {
		var peer tailcatClient
		if err = c.receiveTailcat(setup, "tailcat-client", &peer); err != nil {
			return nil, nil, err
		}
		if peer.Key.IsZero() {
			return nil, nil, errors.New("invalid tailcat client")
		}
		server := &tailcat.Server{DERPMapURL: mapURL, Logf: quiet, AllowClient: func(k key.NodePublic) bool { return k == peer.Key }}
		closeBackend = func() { server.Close() }
		var ln net.Listener
		ln, err = server.Listen(setup, "tcp", ":0")
		if err == nil {
			defer ln.Close()
			offer := tailcatOffer{server.TailcatAddr(), uint16(ln.Addr().(*net.TCPAddr).Port)}
			err = c.sendTailcat(setup, "tailcat-offer", offer)
			if err == nil {
				stream, err = ln.(tailcat.ContextListener).AcceptContext(setup)
			}
		}
	}
	if err != nil {
		closeBackend()
		return nil, nil, fmt.Errorf("tailcat setup failed: %w", err)
	}
	lifetime, stop := context.WithCancel(ctx)
	var once sync.Once
	cleanup := func() { once.Do(func() { stop(); stream.Close(); closeBackend() }) }
	if monitorClient != nil {
		reporter := newTailcatRoute()
		c.route.Store(&routeSource{reporter})
		go reporter.monitor(lifetime, monitorClient)
	}
	go func() { <-lifetime.Done(); cleanup() }()
	return stream, cleanup, nil
}

type tailcatRoute struct {
	mu     sync.RWMutex
	status transport.RouteStatus
}

func newTailcatRoute() *tailcatRoute {
	return &tailcatRoute{status: transport.RouteStatus{Selected: "connecting"}}
}
func (r *tailcatRoute) RouteStatus() transport.RouteStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}
func (r *tailcatRoute) monitor(ctx context.Context, client *tailcat.Client) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, time.Second)
		result, err := client.DiscoPing(probe)
		cancel()
		s := transport.RouteStatus{Selected: "connecting"}
		if err == nil {
			s.RTTMillis = int64(result.LatencySeconds * 1000)
			if result.Endpoint != "" {
				s.Selected = "direct"
				s.Direct = transport.PathStatus{Available: true, RTTMillis: s.RTTMillis}
			} else if result.DERPRegionID != 0 {
				s.Selected = "relay"
				s.Relay = transport.PathStatus{Available: true, RTTMillis: s.RTTMillis}
			}
		}
		r.mu.Lock()
		r.status = s
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
