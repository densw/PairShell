package main

import (
	"testing"
)

func TestRejectSSHConfigPathInjection(t *testing.T) {
	for _, path := range []string{"reference\nHost evil", "reference\"$(touch danger)", "reference%h"} {
		if e := writeSSHConfig(nil, nil, path); e == nil {
			t.Fatal("unsafe path accepted")
		}
	}
}
