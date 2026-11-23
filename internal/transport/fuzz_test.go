package transport

import "testing"

func FuzzFrame(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":1,"session_id":"s","authorization_epoch":1,"seq":1,"ack":0,"type":"data","payload":"YQ=="}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		c := NewConn("s", 1, func([]byte) error { return nil })
		_ = c.Accept(b)
		c.Close()
	})
}
