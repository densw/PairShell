package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func selectDesktopFileRoot(path, home, appData string, pin func(string, os.FileInfo) error) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) || (runtime.GOOS == "windows" && strings.HasPrefix(filepath.ToSlash(path), "//")) {
		return "", errors.New("请输入要共享的本机目录的完整路径")
	}
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("无法读取共享目录，请检查路径")
	}
	if resolved != path {
		return "", errors.New("共享目录包含链接，请输入实际目录的完整路径")
	}
	selected, err := os.Stat(path)
	if err != nil || !selected.IsDir() {
		return "", errors.New("请输入一个现有目录的完整路径")
	}
	protected, err := os.Stat(appData)
	if err != nil {
		return "", errors.New("无法确认应用数据的位置")
	}
	if containsDirectory(selected, home) || containsDirectory(selected, appData) || containsDirectory(protected, path) {
		return "", errors.New("请选择单独的共享目录，避免共享整个用户主目录或 PairShell 应用数据")
	}
	if err = pin(path, selected); err != nil {
		return "", errors.New("无法打开共享目录")
	}
	return path, nil
}

// Compare directory identities so alternate casing on case-insensitive volumes
// cannot bypass the protected-directory check.
func containsDirectory(ancestor os.FileInfo, path string) bool {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	for {
		if info, err := os.Stat(path); err == nil && os.SameFile(ancestor, info) {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
