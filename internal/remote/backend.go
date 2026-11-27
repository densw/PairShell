package remote

import (
	"context"
	"net"
	"pairshell/internal/transport"
)

type routeReporter interface{ RouteStatus() transport.RouteStatus }
type routeSource struct{ reporter routeReporter }

// openTransport is called only after identity verification and explicit consent.
func (c *coordinator) openTransport(ctx context.Context, stop context.CancelFunc) (net.Conn, func(), error) {
	if c.useTailcat {
		return c.tailcatStream(ctx)
	}
	conn, peers, err := c.candidatePaths(ctx, stop)
	return conn, func() { closeCandidates(peers) }, err
}

func closeStream(conn net.Conn) error {
	if c, ok := conn.(*transport.Conn); ok {
		return c.GracefulClose()
	}
	return conn.Close()
}
