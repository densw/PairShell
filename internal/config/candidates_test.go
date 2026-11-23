package config

import "testing"

func TestCustomServerFallbackAndForcedMode(t *testing.T) {
	for _, tc := range []struct {
		custom, official string
		force            bool
		want             []string
	}{
		{"https://custom.example", "https://official.example", false, []string{"https://custom.example", "https://official.example"}},
		{"https://custom.example", "https://official.example", true, []string{"https://custom.example"}},
		{"https://custom.example", "", false, []string{"https://custom.example"}},
		{"", "https://official.example", false, []string{"https://official.example"}},
	} {
		got, err := serverCandidates("", "", Settings{Server: tc.custom, ForceCustom: tc.force}, tc.official)
		if err != nil || len(got) != len(tc.want) {
			t.Fatalf("%+v: %v %v", tc, got, err)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("candidate %d: %q", i, got[i])
			}
		}
	}
	if _, err := serverCandidates("", "", Settings{}, ""); err == nil {
		t.Fatal("unavailable official service silently accepted")
	}
	if _, err := serverCandidates("", "", Settings{ForceCustom: true}, "https://official.example"); err == nil {
		t.Fatal("forced mode used official service")
	}
}
