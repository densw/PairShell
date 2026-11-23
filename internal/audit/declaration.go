package audit

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Declaration binds both temporary identities to a session. The coordinator must
// obtain these signatures through the authenticated pairing protocol.
type Declaration struct {
	SessionID         string `json:"session_id"`
	CustomerKey       []byte `json:"customer_key"`
	MerchantKey       []byte `json:"merchant_key"`
	CustomerSignature []byte `json:"customer_signature"`
	MerchantSignature []byte `json:"merchant_signature"`
}

func declarationBytes(d Declaration) []byte {
	d.CustomerSignature = nil
	d.MerchantSignature = nil
	b, _ := json.Marshal(d)
	return append([]byte("PairShell:declaration:v1\x00"), b...)
}
func SignDeclaration(d *Declaration, customer, merchant ed25519.PrivateKey) {
	d.CustomerSignature = ed25519.Sign(customer, declarationBytes(*d))
	d.MerchantSignature = ed25519.Sign(merchant, declarationBytes(*d))
}
func validateDeclaration(d Declaration) error {
	if d.SessionID == "" || len(d.SessionID) > 128 || len(d.CustomerKey) != 32 || len(d.MerchantKey) != 32 || !ed25519.Verify(d.CustomerKey, declarationBytes(d), d.CustomerSignature) || !ed25519.Verify(d.MerchantKey, declarationBytes(d), d.MerchantSignature) {
		return errors.New("invalid jointly signed declaration")
	}
	return nil
}
func (w *Witness) Register(d Declaration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := validateDeclaration(d); e != nil {
		return e
	}
	name := filepath.Join(w.dir, "declaration-"+digest([]byte(d.SessionID))+".json")
	b, _ := json.Marshal(d)
	f, e := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(e) {
		old, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if string(old) != string(b) {
			return errors.New("session declaration conflict")
		}
		return nil
	}
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (w *Witness) registered(c Checkpoint) error {
	b, e := os.ReadFile(filepath.Join(w.dir, "declaration-"+digest([]byte(c.SessionID))+".json"))
	if e != nil {
		return errors.New("unregistered witness session")
	}
	var d Declaration
	if json.Unmarshal(b, &d) != nil || validateDeclaration(d) != nil || string(d.CustomerKey) != string(c.CustomerKey) {
		return errors.New("declaration identity mismatch")
	}
	return nil
}
