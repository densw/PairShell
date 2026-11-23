package server

import (
	"sync"
	"testing"
	"time"
)

func TestAtomicClaim(t *testing.T) {
	s := New(Config{CreateToken: "deploy", MaxRooms: 2, RoomTTL: time.Minute})
	r, e := s.Create("deploy", "abcdefghijklmnop", "key")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Claim("abcdefghijklmnop", r.JoinToken, "customer") == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("claims %d", wins)
	}
}
