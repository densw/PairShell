package terminal

import "testing"

func TestDefaultShellSelection(t *testing.T) {
	for _, test := range []struct {
		name, goos, preferred, want string
		available                   map[string]bool
	}{
		{"mac-native", "darwin", "", "/bin/zsh", map[string]bool{"/bin/zsh": true, "/bin/bash": true}},
		{"user-shell", "darwin", "/opt/homebrew/bin/fish", "/opt/homebrew/bin/fish", map[string]bool{"/opt/homebrew/bin/fish": true, "/bin/zsh": true}},
		{"missing-user-shell", "darwin", "/missing/zsh", "/bin/zsh", map[string]bool{"/bin/zsh": true}},
		{"reject-relative", "linux", "custom-shell", "/bin/bash", map[string]bool{"custom-shell": true, "/bin/bash": true}},
		{"linux-fallback", "linux", "", "/bin/sh", map[string]bool{"/bin/sh": true}},
		{"windows", "windows", "/bin/zsh", "powershell.exe -NoProfile -NoLogo", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := selectShell(test.goos, test.preferred, func(shell string) bool { return test.available[shell] })
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
