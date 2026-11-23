package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// interactiveShell connects the local terminal to a single persistent SSH PTY.
// Raw mode keeps Ctrl+C, Tab and cursor keys at the remote shell, not the local process.
func interactiveShell(ctx context.Context, c *ssh.Client) error {
	s, e := c.NewSession()
	if e != nil {
		return e
	}
	defer s.Close()
	fd := int(os.Stdin.Fd())
	w, h := 80, 24
	if term.IsTerminal(fd) {
		state, e := term.MakeRaw(fd)
		if e != nil {
			return e
		}
		defer term.Restore(fd, state)
		if cols, rows, e := term.GetSize(fd); e == nil {
			w, h = cols, rows
		}
	}
	if e = s.RequestPty("xterm-256color", h, w, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}); e != nil {
		return e
	}
	s.Stdin = os.Stdin
	s.Stdout, s.Stderr = terminalDisplays(os.Stdout, os.Stderr)
	if e = s.Shell(); e != nil {
		return e
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				s.Close()
				return
			case <-tick.C:
				cols, rows, e := term.GetSize(fd)
				if e == nil && (cols != w || rows != h) {
					w, h = cols, rows
					s.WindowChange(h, w)
				}
			}
		}
	}()
	return s.Wait()
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
func localSSHClient(ctx context.Context, path string) (*ssh.Client, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	var ref struct{ URL, Token string }
	dec := json.NewDecoder(io.LimitReader(f, 4097))
	dec.DisallowUnknownFields()
	e = dec.Decode(&ref)
	var tail any
	if e == nil && dec.Decode(&tail) != io.EOF {
		e = errors.New("invalid reference")
	}
	f.Close()
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(ref.URL)
	if e != nil || u.Scheme != "http" || u.User != nil || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || ref.Token == "" || strings.ContainsAny(ref.Token, "\r\n") {
		return nil, errors.New("invalid local session reference")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("session must be local")
	}
	key, e := os.ReadFile(path + ".key")
	if e != nil {
		return nil, e
	}
	signer, e := ssh.ParsePrivateKey(key)
	if e != nil {
		return nil, e
	}
	hostCheck, e := knownhosts.New(path + ".known_hosts")
	if e != nil {
		return nil, e
	}
	conn, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", u.Host)
	if e != nil {
		return nil, e
	}
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, e = fmt.Fprintf(conn, "CONNECT /proxy HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n", u.Host, ref.Token); e != nil {
		conn.Close()
		return nil, e
	}
	r := bufio.NewReader(conn)
	res, e := http.ReadResponse(r, &http.Request{Method: "CONNECT"})
	if e != nil || res.StatusCode != 200 {
		conn.Close()
		return nil, errors.New("local proxy rejected connection")
	}
	cc, ch, req, e := ssh.NewClientConn(bufferedConn{conn, r}, "pairshell-current:22", &ssh.ClientConfig{User: "pairshell", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: hostCheck})
	if e != nil {
		conn.Close()
		return nil, e
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(cc, ch, req), nil
}
