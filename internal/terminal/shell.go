package terminal

import (
	"os"
	"path"
	"runtime"
)

// DefaultShell uses the customer's interactive shell, rather than forcing sh.
// A GUI launch may not inherit SHELL, so each Unix platform has native fallbacks.
func DefaultShell() string {
	return selectShell(runtime.GOOS, os.Getenv("SHELL"), func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
	})
}

func selectShell(goos, preferred string, executable func(string) bool) string {
	if goos == "windows" {
		return "powershell.exe -NoProfile -NoLogo"
	}
	if path.IsAbs(preferred) && executable(preferred) {
		return preferred
	}
	candidates := []string{"/bin/bash", "/bin/sh"}
	if goos == "darwin" {
		candidates = append([]string{"/bin/zsh"}, candidates...)
	}
	for _, shell := range candidates {
		if executable(shell) {
			return shell
		}
	}
	return "/bin/sh"
}
