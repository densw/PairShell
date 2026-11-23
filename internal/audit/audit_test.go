package audit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionLogCapacityLeavesRoomToSeal(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "bounded")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Event{Actor: "customer", EventType: "session.start"}); err != nil {
		t.Fatal(err)
	}
	w.bytes = maxSessionLogBytes - 2*MaxEventBytes
	if err := w.Append(Event{Actor: "pty", EventType: "terminal.output"}); !errors.Is(err, ErrLogCapacity) {
		t.Fatalf("expected audit capacity error, got %v", err)
	}
	if err := w.Close("audit capacity"); err != nil {
		t.Fatal(err)
	}
	v, err := Verify(dir)
	if err != nil || !v.Complete || v.Events != 2 {
		t.Fatalf("quota left an unverifiable log: %+v %v", v, err)
	}
}

func TestChainAndTamper(t *testing.T) {
	d := t.TempDir()
	w, e := NewWriter(d, "session")
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Append(Event{Actor: "customer", EventType: "session.start", AuthorizationEpoch: 1, Payload: json.RawMessage(`{}`)}); e != nil {
		t.Fatal(e)
	}
	if e = w.Append(Event{Actor: "pty", EventType: "terminal.output", Payload: json.RawMessage(`{"data_b64":"aGVsbG8="}`)}); e != nil {
		t.Fatal(e)
	}
	if e = w.Close("ended"); e != nil {
		t.Fatal(e)
	}
	v, e := Verify(d)
	if e != nil || !v.Complete || v.Events != 3 {
		t.Fatalf("%+v %v", v, e)
	}
	p := filepath.Join(d, "events.jsonl")
	b, _ := os.ReadFile(p)
	b = bytes.Replace(b, []byte(`"previous_hash":"0`), []byte(`"previous_hash":"1`), 1)
	os.WriteFile(p, b, 0600)
	if _, e = Verify(d); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestUnfinished(t *testing.T) {
	d := t.TempDir()
	w, _ := NewWriter(d, "s")
	w.Append(Event{EventType: "session.start", Payload: json.RawMessage(`{}`)})
	w.file.Close()
	v, e := Verify(d)
	if e != nil || v.Complete {
		t.Fatalf("%+v %v", v, e)
	}
}
func TestVector(t *testing.T) {
	if got := hashEvent([32]byte{}, []byte("{}")); got != "34b9e8989c5defbab11875b3f8bdee13dc4c3353820fc5d41335c5e71a3bd6e2" {
		t.Fatal(got)
	}
}
func TestManifestReasonBoundToEnd(t *testing.T) {
	d := t.TempDir()
	w, _ := NewWriter(d, "s")
	w.Append(Event{EventType: "session.start"})
	w.Close("normal")
	p := filepath.Join(d, "manifest.json")
	b, _ := os.ReadFile(p)
	b = bytes.Replace(b, []byte(`"reason": "normal"`), []byte(`"reason": "forged"`), 1)
	os.WriteFile(p, b, 0600)
	if _, e := Verify(d); e == nil {
		t.Fatal("forged reason accepted")
	}
}
func TestWitnessPersistenceAndRegistration(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	_, customer, _ := ed25519.GenerateKey(rand.Reader)
	_, merchant, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	w, _ := OpenWitness(dir, key)
	c := Checkpoint{SessionID: "registered", Sequence: 1, Index: 1, ChainHead: digest(nil), CustomerKey: customer.Public().(ed25519.PublicKey)}
	SignCheckpoint(&c, customer)
	if _, e := w.Submit(c); e == nil {
		t.Fatal("unregistered accepted")
	}
	d := Declaration{SessionID: c.SessionID, CustomerKey: c.CustomerKey, MerchantKey: merchant.Public().(ed25519.PublicKey)}
	SignDeclaration(&d, customer, merchant)
	if e := w.Register(d); e != nil {
		t.Fatal(e)
	}
	r, e := w.Submit(c)
	if e != nil {
		t.Fatal(e)
	}
	w, e = OpenWitness(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	c.Index = 2
	c.PreviousReceiptHash = ReceiptHash(r)
	c.Final = true
	SignCheckpoint(&c, customer)
	if _, e = w.Submit(c); e != nil {
		t.Fatal(e)
	}
	c.Index++
	SignCheckpoint(&c, customer)
	if _, e = w.Submit(c); e == nil {
		t.Fatal("checkpoint after final accepted")
	}
}
