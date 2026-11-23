package audit

import (
	"bufio"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"pairshell/internal/securefile"
	"path/filepath"
	"sync"
	"time"
)

type Checkpoint struct {
	SessionID           string `json:"session_id"`
	Sequence            uint64 `json:"sequence"`
	ChainHead           string `json:"chain_head"`
	Index               uint64 `json:"checkpoint_index"`
	PreviousReceiptHash string `json:"previous_receipt_hash"`
	Final               bool   `json:"final"`
	CustomerKey         []byte `json:"customer_key"`
	Signature           []byte `json:"signature"`
}
type Receipt struct {
	KeyID       string     `json:"key_id"`
	ReceivedAt  string     `json:"received_at"`
	Checkpoint  Checkpoint `json:"checkpoint"`
	RequestHash string     `json:"request_hash"`
	Signature   []byte     `json:"signature"`
}
type Witness struct {
	mu   sync.Mutex
	dir  string
	key  ed25519.PrivateKey
	last map[string]Receipt
}

func checkpointBytes(c Checkpoint) []byte {
	c.Signature = nil
	b, _ := json.Marshal(c)
	return append([]byte("PairShell:checkpoint:v1\x00"), b...)
}
func SignCheckpoint(c *Checkpoint, k ed25519.PrivateKey) {
	c.Signature = ed25519.Sign(k, checkpointBytes(*c))
}
func receiptBytes(r Receipt) []byte {
	r.Signature = nil
	b, _ := json.Marshal(r)
	return append([]byte("PairShell:receipt:v1\x00"), b...)
}
func ReceiptHash(r Receipt) string { b, _ := json.Marshal(r); return digest(b) }
func VerifyReceipt(r Receipt, trusted ed25519.PublicKey) error {
	if len(trusted) != ed25519.PublicKeySize || r.KeyID != digest(trusted) {
		return errors.New("untrusted witness key")
	}
	b, _ := json.Marshal(r.Checkpoint)
	if r.RequestHash != digest(b) || !ed25519.Verify(trusted, receiptBytes(r), r.Signature) {
		return errors.New("invalid receipt signature")
	}
	return nil
}
func OpenWitness(dir string, key ed25519.PrivateKey) (*Witness, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid witness private key")
	}
	if e := securefile.Mkdir(dir); e != nil {
		return nil, e
	}
	w := &Witness{dir: dir, key: key, last: make(map[string]Receipt)}
	f, e := os.Open(filepath.Join(dir, "receipts.jsonl"))
	if os.IsNotExist(e) {
		return w, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), MaxEventBytes)
	for s.Scan() {
		var r Receipt
		if e = json.Unmarshal(s.Bytes(), &r); e != nil {
			return nil, e
		}
		if e = VerifyReceipt(r, key.Public().(ed25519.PublicKey)); e != nil {
			return nil, e
		}
		if e = w.check(r.Checkpoint); e != nil {
			return nil, e
		}
		w.last[r.Checkpoint.SessionID] = r
	}
	if s.Err() != nil {
		return nil, s.Err()
	}
	return w, nil
}
func (w *Witness) check(c Checkpoint) error {
	if err := w.registered(c); err != nil {
		return err
	}
	head, e := hex.DecodeString(c.ChainHead)
	if c.SessionID == "" || len(c.SessionID) > 128 || e != nil || len(head) != 32 || len(c.CustomerKey) != 32 || !ed25519.Verify(c.CustomerKey, checkpointBytes(c), c.Signature) {
		return errors.New("invalid checkpoint")
	}
	p, ok := w.last[c.SessionID]
	if !ok {
		if c.Index != 1 || c.PreviousReceiptHash != "" {
			return errors.New("invalid initial checkpoint")
		}
		return nil
	}
	if p.Checkpoint.Final || string(c.CustomerKey) != string(p.Checkpoint.CustomerKey) || c.Index != p.Checkpoint.Index+1 || c.Sequence < p.Checkpoint.Sequence || (c.Sequence == p.Checkpoint.Sequence && c.ChainHead != p.Checkpoint.ChainHead) || c.PreviousReceiptHash != ReceiptHash(p) {
		return errors.New("checkpoint rollback or chain mismatch")
	}
	return nil
}
func (w *Witness) Submit(c Checkpoint) (Receipt, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.check(c); e != nil {
		return Receipt{}, e
	}
	b, _ := json.Marshal(c)
	r := Receipt{KeyID: digest(w.key.Public().(ed25519.PublicKey)), ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano), Checkpoint: c, RequestHash: digest(b)}
	r.Signature = ed25519.Sign(w.key, receiptBytes(r))
	line, _ := json.Marshal(r)
	f, e := os.OpenFile(filepath.Join(w.dir, "receipts.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return Receipt{}, e
	}
	_, e = f.Write(append(line, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return Receipt{}, e
	}
	if ce != nil {
		return Receipt{}, ce
	}
	w.last[c.SessionID] = r
	return r, nil
}
