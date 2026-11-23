// Package sshbridge binds SSH authentication to the verified pair keys.
package sshbridge

import (
	"bytes"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"sync"
	"time"
)

type Size struct{ Width, Height uint32 }
type Handler interface {
	Run(string, io.Reader, io.Writer, io.Writer) (int, error)
	Shell(io.Reader, io.Writer, uint32, uint32, <-chan Size) (int, error)
}

func Client(conn net.Conn, address string, identity ssh.Signer, host ssh.PublicKey) (*ssh.Client, error) {
	c, ch, r, e := ssh.NewClientConn(conn, address, &ssh.ClientConfig{User: "pairshell", Auth: []ssh.AuthMethod{ssh.PublicKeys(identity)}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(host.Marshal(), key.Marshal()) {
			return errors.New("PAIRING_MISMATCH")
		}
		return nil
	}})
	if e != nil {
		return nil, e
	}
	return ssh.NewClient(c, ch, r), nil
}
func Serve(conn net.Conn, host ssh.Signer, peer ssh.PublicKey, handler Handler) error {
	return serve(conn, host, peer, handler, 0)
}
func serve(conn net.Conn, host ssh.Signer, peer ssh.PublicKey, handler Handler, depth int) error {
	defer conn.Close()
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if !bytes.Equal(peer.Marshal(), key.Marshal()) {
			return nil, errors.New("PAIRING_MISMATCH")
		}
		return &ssh.Permissions{}, nil
	}}
	config.AddHostKey(host)
	sc, channels, requests, e := ssh.NewServerConn(conn, config)
	if e != nil {
		return e
	}
	defer sc.Close()
	go ssh.DiscardRequests(requests)
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 16)
	for next := range channels {
		select {
		case slots <- struct{}{}:
		default:
			next.Reject(ssh.ResourceShortage, "channel limit")
			continue
		}
		if next.ChannelType() == "pairshell-proxy" && depth == 0 && len(next.ExtraData()) == 0 {
			channel, req, err := next.Accept()
			if err != nil {
				<-slots
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				go ssh.DiscardRequests(req)
				_ = serve(&channelConn{Channel: channel, conn: conn}, host, peer, handler, depth+1)
			}()
			continue
		}
		if next.ChannelType() == "pairshell-rpc" {
			rpc, ok := handler.(interface {
				RPC(io.ReadWriteCloser) error
			})
			if !ok || len(next.ExtraData()) != 0 {
				<-slots
				next.Reject(ssh.Prohibited, "RPC disabled")
				continue
			}
			channel, req, err := next.Accept()
			if err != nil {
				<-slots
				continue
			}
			wg.Add(1)
			go func() {
				defer func() { <-slots }()
				defer wg.Done()
				defer channel.Close()
				go ssh.DiscardRequests(req)
				_ = rpc.RPC(channel)
			}()
			continue
		}
		if next.ChannelType() != "session" {
			<-slots
			next.Reject(ssh.Prohibited, "forwarding disabled")
			continue
		}
		channel, req, e := next.Accept()
		if e != nil {
			<-slots
			continue
		}
		wg.Add(1)
		go func() {
			defer func() { <-slots }()
			defer wg.Done()
			defer channel.Close()
			serveChannel(channel, req, handler)
		}()
	}
	return nil
}

// channelConn carries one inner native SSH connection over the verified outer connection.
type channelConn struct {
	ssh.Channel
	conn net.Conn
}

func (c *channelConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *channelConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *channelConn) SetDeadline(t time.Time) error      { return errors.ErrUnsupported }
func (c *channelConn) SetReadDeadline(t time.Time) error  { return errors.ErrUnsupported }
func (c *channelConn) SetWriteDeadline(t time.Time) error { return errors.ErrUnsupported }
func serveChannel(ch ssh.Channel, requests <-chan *ssh.Request, h Handler) {
	resize := make(chan Size, 1)
	defer close(resize)
	size := Size{80, 24}
	started := false
	for r := range requests {
		switch r.Type {
		case "pty-req":
			var p struct {
				Term                                   string
				Width, Height, PixelWidth, PixelHeight uint32
				Modes                                  string
			}
			ok := !started && ssh.Unmarshal(r.Payload, &p) == nil && p.Width > 0 && p.Height > 0 && p.Width <= 1000 && p.Height <= 1000
			if ok {
				size = Size{p.Width, p.Height}
			}
			r.Reply(ok, nil)
		case "window-change":
			var p struct{ Width, Height, PixelWidth, PixelHeight uint32 }
			ok := ssh.Unmarshal(r.Payload, &p) == nil && p.Width > 0 && p.Height > 0 && p.Width <= 1000 && p.Height <= 1000
			if ok {
				select {
				case resize <- Size{p.Width, p.Height}:
				default:
				}
			}
			r.Reply(ok, nil)
		case "shell", "exec":
			if started {
				r.Reply(false, nil)
				continue
			}
			command := ""
			if r.Type == "exec" {
				var p struct{ Command string }
				if ssh.Unmarshal(r.Payload, &p) != nil || len(p.Command) > 65536 {
					r.Reply(false, nil)
					continue
				}
				command = p.Command
			} else if len(r.Payload) != 0 {
				r.Reply(false, nil)
				continue
			}
			started = true
			r.Reply(true, nil)
			shell := r.Type == "shell"
			initial := size
			go func() {
				code := 255
				var e error
				if shell {
					code, e = h.Shell(ch, ch, initial.Width, initial.Height, resize)
				} else {
					code, e = h.Run(command, ch, ch, ch.Stderr())
				}
				if e != nil {
					io.WriteString(ch.Stderr(), "PairShell operation failed\n")
				}
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				ch.Close()
			}()
		default:
			r.Reply(false, nil)
		}
	}
}
