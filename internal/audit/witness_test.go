package audit

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestWitness(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	_, customer, _ := ed25519.GenerateKey(rand.Reader)
	w, e := OpenWitness(t.TempDir(), key)
	if e != nil {
		t.Fatal(e)
	}
	_, merchant, _ := ed25519.GenerateKey(rand.Reader)
	d := Declaration{SessionID: "s", CustomerKey: customer.Public().(ed25519.PublicKey), MerchantKey: merchant.Public().(ed25519.PublicKey)}
	SignDeclaration(&d, customer, merchant)
	if err := w.Register(d); err != nil {
		t.Fatal(err)
	}
	cp := Checkpoint{SessionID: "s", Sequence: 1, ChainHead: digest([]byte("x")), Index: 1, CustomerKey: customer.Public().(ed25519.PublicKey)}
	SignCheckpoint(&cp, customer)
	r, e := w.Submit(cp)
	if e != nil {
		t.Fatal(e)
	}
	if e = VerifyReceipt(r, pub); e != nil {
		t.Fatal(e)
	}
	if e = VerifyReceipt(r, nil); e == nil {
		t.Fatal("unknown trusted key accepted")
	}
	cp.Sequence = 0
	SignCheckpoint(&cp, customer)
	if _, e = w.Submit(cp); e == nil {
		t.Fatal("rollback accepted")
	}
}
