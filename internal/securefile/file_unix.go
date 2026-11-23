//go:build !windows

package securefile

import (
	"errors"
	"os"
	"path/filepath"
)

func verify(p string, dir bool) error {
	s, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if s.Mode()&os.ModeSymlink != 0 || s.IsDir() != dir || s.Mode().Perm()&0077 != 0 {
		return errors.New("insecure file permissions or type")
	}
	return nil
}
func Mkdir(p string) error {
	if s, e := os.Lstat(p); e == nil {
		if s.Mode()&os.ModeSymlink != 0 || !s.IsDir() {
			return errors.New("invalid private directory")
		}
		if e = os.Chmod(p, 0700); e != nil {
			return e
		}
		return verify(p, true)
	}
	parent := filepath.Dir(p)
	if _, e := os.Stat(parent); os.IsNotExist(e) {
		if e = Mkdir(parent); e != nil {
			return e
		}
	}
	if e := os.Mkdir(p, 0700); e != nil {
		return e
	}
	return verify(p, true)
}
func Write(p string, b []byte) error {
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return verify(p, false)
}
