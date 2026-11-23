package transport

import (
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestOutOfOrderShortReadAckAndSwitch(t *testing.T) {
	var frames [][]byte
	sender := NewConn("s", 1, func(p []byte) error { frames = append(frames, append([]byte(nil), p...)); return nil })
	defer sender.Close()
	receiver := NewConn("s", 1, func(p []byte) error { return sender.Accept(p) })
	defer receiver.Close()
	sender.Write([]byte("abc"))
	sender.Write([]byte("def"))
	if e := receiver.Accept(frames[1]); e != nil {
		t.Fatal(e)
	}
	receiver.SetReadDeadline(time.Now().Add(time.Millisecond))
	if _, e := receiver.Read(make([]byte, 1)); e == nil {
		t.Fatal("out of order delivered")
	}
	receiver.SetReadDeadline(time.Time{})
	if e := receiver.Accept(frames[0]); e != nil {
		t.Fatal(e)
	}
	got := make([]byte, 6)
	if _, e := io.ReadFull(receiver, got[:2]); e != nil {
		t.Fatal(e)
	}
	if _, e := io.ReadFull(receiver, got[2:]); e != nil || string(got) != "abcdef" {
		t.Fatal(string(got), e)
	}
	count := 0
	if e := sender.Switch(func([]byte) error { count++; return nil }); e != nil || count != 0 {
		t.Fatal("acked bytes retransmitted", count, e)
	}
}
func TestInvalidAckAndOversizedFrames(t *testing.T) {
	c := NewConn("s", 1, func([]byte) error { return nil })
	defer c.Close()
	b, _ := json.Marshal(frame{Stream: 1, Version: 1, Session: "s", Epoch: 1, Ack: 10, Type: "ack"})
	if c.Accept(b) == nil {
		t.Fatal("future ack accepted")
	}
	if c.Accept(make([]byte, MaxPayload*2+1)) == nil {
		t.Fatal("oversized frame accepted")
	}
}
