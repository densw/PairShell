package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"pairshell/internal/audit"
	"pairshell/internal/server"
	"path/filepath"
	"strings"
	"time"
)

func witnessHandler(witness *audit.Witness, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
		d.DisallowUnknownFields()
		var e error
		switch r.URL.Path {
		case "/witness/register":
			var x audit.Declaration
			e = d.Decode(&x)
			if e == nil {
				var tail any
				if d.Decode(&tail) != io.EOF {
					e = errors.New("trailing JSON")
				}
			}
			if e == nil {
				e = witness.Register(x)
			}
			if e == nil {
				w.WriteHeader(204)
				return
			}
		case "/witness/checkpoint":
			var x audit.Checkpoint
			e = d.Decode(&x)
			if e == nil {
				var tail any
				if d.Decode(&tail) != io.EOF {
					e = errors.New("trailing JSON")
				}
			}
			if e == nil {
				var receipt audit.Receipt
				receipt, e = witness.Submit(x)
				if e == nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(receipt)
					return
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid witness request", 400)
	})
}
func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run() error {
	listen := flag.String("listen", "127.0.0.1:8080", "listen address")
	cert := flag.String("tls-cert", "", "TLS certificate")
	key := flag.String("tls-key", "", "TLS key")
	dir := flag.String("witness-dir", "data/witness", "receipt directory")
	keyfile := flag.String("witness-key", "data/witness.key", "private key file")
	public := flag.Bool("public-create", false, "allow public room creation")
	devicePairing := flag.Bool("device-pairing", false, "allow anonymous one-time device pairing (public resource admission)")
	proxies := flag.String("trusted-proxies", "", "comma-separated trusted reverse proxy IPs/CIDRs for X-Forwarded-For")
	flag.Parse()
	trustedProxies, e := parseTrustedProxies(*proxies)
	if e != nil {
		return e
	}
	host, _, e := net.SplitHostPort(*listen)
	if e != nil {
		return e
	}
	ip := net.ParseIP(host)
	if *cert == "" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("TLS required for non-loopback listener")
	}
	if os.Getenv("PAIRSHELL_CREATE_TOKEN") == "" && !*public {
		return errors.New("PAIRSHELL_CREATE_TOKEN required")
	}
	if e = os.MkdirAll(*dir, 0700); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(*keyfile), 0700); e != nil {
		return e
	}
	kb, e := os.ReadFile(*keyfile)
	if os.IsNotExist(e) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*keyfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(priv)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
		kb = priv
	} else if e != nil {
		return e
	}
	witness, e := audit.OpenWitness(*dir, ed25519.PrivateKey(kb))
	if e != nil {
		return e
	}
	mux := http.NewServeMux()
	mux.Handle("/witness/", witnessHandler(witness, os.Getenv("PAIRSHELL_WITNESS_TOKEN")))
	coordinator := server.New(server.Config{CreateToken: os.Getenv("PAIRSHELL_CREATE_TOKEN"), PublicCreate: *public, PublicDevicePairing: *devicePairing, TrustedProxies: trustedProxies, TURNSecret: os.Getenv("PAIRSHELL_TURN_SECRET"), TURNURLs: splitURLs(os.Getenv("PAIRSHELL_TURN_URLS"))})
	mux.Handle("/", coordinator)
	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go coordinator.RunPruner(ctx.Done())
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	fmt.Fprintln(os.Stderr, "PairShell coordinator listening on", *listen)
	if *cert != "" {
		e = srv.ListenAndServeTLS(*cert, *key)
	} else {
		e = srv.ListenAndServe()
	}
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func splitURLs(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var proxies []netip.Prefix
	if strings.TrimSpace(raw) == "" {
		return proxies, nil
	}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if ip, err := netip.ParseAddr(value); err == nil && ip.Zone() == "" {
			ip = ip.Unmap()
			proxies = append(proxies, netip.PrefixFrom(ip, ip.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("invalid trusted proxy IP or CIDR: %q", value)
		}
		proxies = append(proxies, prefix.Masked())
	}
	return proxies, nil
}
