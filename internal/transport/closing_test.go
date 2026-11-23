package transport

import (
	"encoding/json"
	"testing"
)

func TestLateAcknowledgementDoesNotTearDownGracefulClose(t *testing.T) {
	c := NewConn("s", 1, func([]byte) error { return nil })
	c.Close()
	raw, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: "s", Epoch: 1, Type: "ack"})
	if e := c.Accept(raw); e != nil {
		t.Fatalf("late ACK would close path before close acknowledgement: %v", e)
	}
	raw, _ = json.Marshal(frame{Stream: 1, Version: 1, Session: "s", Epoch: 1, Type: "data", Seq: 1, Payload: []byte("x"), PayloadLength: 1})
	if e := c.Accept(raw); e != nil {
		t.Fatal(e)
	}
	if c.buffer.Len() != 0 {
		t.Fatal("closed stream delivered data")
	}
}
