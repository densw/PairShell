package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"pairshell/internal/remote"
	"pairshell/internal/securefile"
	"strings"
)

func proxy(ctx context.Context, path string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var ref struct{ URL, Token string }
	if e = json.Unmarshal(b, &ref); e != nil {
		return e
	}
	u, e := url.Parse(ref.URL)
	if e != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.Port() == "" || ref.Token == "" || strings.ContainsAny(ref.Token, "\r\n") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid reference")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("invalid reference")
	}
	c, e := net.Dial("tcp", u.Host)
	if e != nil {
		return e
	}
	defer c.Close()
	go func() { <-ctx.Done(); c.Close() }()
	fmt.Fprintf(c, "CONNECT /proxy HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n", u.Host, ref.Token)
	r := bufio.NewReader(c)
	res, e := http.ReadResponse(r, &http.Request{Method: "CONNECT"})
	if e != nil {
		return e
	}
	if res.StatusCode != 200 {
		return errors.New("proxy rejected")
	}
	go func() {
		io.Copy(c, os.Stdin)
		if tcp, ok := c.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	_, e = io.Copy(os.Stdout, r)
	return e
}
func writeSSHConfig(m *remote.Merchant, pub []byte, ref string) error {
	if strings.ContainsAny(ref, "\r\n\"\x27\x60$%") {
		return errors.New("unsafe SSH configuration path")
	}
	key, e := m.IdentityPEM()
	if e != nil {
		return e
	}
	keyPath := ref + ".key"
	hostPath := ref + ".known_hosts"
	if e = securefile.Write(keyPath, key); e != nil {
		return e
	}
	host, e := ssh.NewPublicKey(ed25519.PublicKey(pub))
	if e != nil {
		return e
	}
	if e = securefile.Write(hostPath, append([]byte("pairshell-current "), ssh.MarshalAuthorizedKey(host)...)); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, `\`, `/`) + `"` }
	config := "Host pairshell-current\n  HostName pairshell-current\n  ProxyCommand " + quote(exe) + " proxy --session " + quote(ref) + "\n  StrictHostKeyChecking yes\n  UserKnownHostsFile " + quote(hostPath) + "\n  IdentityFile " + quote(keyPath) + "\n  IdentitiesOnly yes\n"
	if e = securefile.Write(ref+".ssh_config", []byte(config)); e != nil {
		return e
	}
	fmt.Fprintln(os.Stderr, "原生 SSH: ssh -F", quote(ref+".ssh_config"), "pairshell-current")
	return nil
}
