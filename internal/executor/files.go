package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const maxFileBytes = 1 << 20

type File struct {
	Data   []byte `json:"data"`
	SHA256 string `json:"sha256"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// SetFileRoot pins the approved directory independently of the terminal CWD.
// A session may select it once, before any file operations.
func (m *Manager) SetFileRoot(path string, expected os.FileInfo) error {
	if !filepath.IsAbs(path) || expected == nil {
		return errors.New("file directory must be absolute")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, actual) {
		root.Close()
		return errors.New("file directory changed during approval")
	}
	m.mu.Lock()
	if m.closed || m.fileBusy || m.root != nil {
		m.mu.Unlock()
		root.Close()
		return errors.New("file directory cannot be changed")
	}
	m.root = root
	m.mu.Unlock()
	return nil
}

type fileAccess interface {
	Stat(string) (os.FileInfo, error)
	OpenFile(string, int, os.FileMode) (*os.File, error)
	Remove(string) error
	Rename(string, string) error
}

type systemFiles struct{ cwd string }

func (s systemFiles) resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(s.cwd, path)
}
func (s systemFiles) Stat(path string) (os.FileInfo, error) { return os.Stat(s.resolve(path)) }
func (s systemFiles) OpenFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(s.resolve(path), flags, mode)
}
func (s systemFiles) Remove(path string) error     { return os.Remove(s.resolve(path)) }
func (s systemFiles) Rename(from, to string) error { return os.Rename(s.resolve(from), s.resolve(to)) }

func (m *Manager) beginFile(epoch uint64, ai bool, path string, write bool) (context.Context, fileAccess, error) {
	if path == "" || (!m.systemFilesystem && !filepath.IsLocal(path)) {
		return nil, nil, errors.New("invalid path")
	}
	capability := "file_read"
	if write {
		capability = "file_write"
	}
	ctx, err := m.prepareOperation(epoch, capability, ai)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	if m.closed || ctx.Err() != nil {
		m.mu.Unlock()
		return nil, nil, errors.New("SESSION_REVOKED")
	}
	if m.fileBusy || (write && m.busy && !m.systemFilesystem) {
		m.mu.Unlock()
		return nil, nil, errors.New("EXECUTION_BUSY")
	}
	m.fileBusy = true
	root := m.root
	m.mu.Unlock()
	if m.systemFilesystem {
		if err = m.checkOperation(ctx); err != nil {
			m.finishFile()
			return nil, nil, err
		}
		return ctx, systemFiles{cwd: m.rootPath}, nil
	}
	if root == nil {
		root, err = os.OpenRoot(m.rootPath)
		if err != nil {
			m.finishFile()
			return nil, nil, err
		}
		m.mu.Lock()
		m.root = root
		m.mu.Unlock()
	}
	if err = m.checkOperation(ctx); err != nil {
		m.finishFile()
		return nil, nil, err
	}
	return ctx, root, nil
}

func (m *Manager) finishFile() {
	m.mu.Lock()
	m.fileBusy = false
	var root *os.Root
	if m.closed {
		root, m.root = m.root, nil
	}
	m.mu.Unlock()
	if root != nil {
		root.Close()
	}
}

func (m *Manager) ReadFile(epoch uint64, ai bool, path string, limit int64) (File, error) {
	if limit < 1 || limit > maxFileBytes {
		return File{}, errors.New("invalid limit")
	}
	ctx, root, err := m.beginFile(epoch, ai, path, false)
	if err != nil {
		return File{}, err
	}
	defer m.finishFile()
	data, err := readRegularFile(root, path, limit)
	if err != nil {
		return File{}, err
	}
	if err = m.checkOperation(ctx); err != nil {
		return File{}, err
	}
	out := File{data, digest(data)}
	if err = m.record("file.read", "", struct {
		Path string
		Size int
		Hash string
	}{path, len(data), out.SHA256}); err != nil {
		return File{}, err
	}
	if err = m.authorize(epoch, "file_read", ai, func() error { return m.checkOperation(ctx) }); err != nil {
		return File{}, err
	}
	return out, nil
}

func (m *Manager) WriteFile(epoch uint64, ai bool, path string, data []byte, expected string) error {
	if len(data) > maxFileBytes {
		return errors.New("FILE_LIMIT")
	}
	ctx, root, err := m.beginFile(epoch, ai, path, true)
	if err != nil {
		return err
	}
	defer m.finishFile()
	old, err := readRegularFile(root, path, maxFileBytes)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if (err == nil && digest(old) != expected) || (os.IsNotExist(err) && expected != "") {
		return errors.New("PRECONDITION_FAILED")
	}
	if err = m.checkOperation(ctx); err != nil {
		return err
	}
	hash := digest(data)
	if err = m.record("file.write.intent", "", struct {
		Path, Before, After string
		Size                int
	}{path, expected, hash, len(data)}); err != nil {
		return err
	}
	if err = m.checkOperation(ctx); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.pairshell-%d", path, time.Now().UnixNano())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if info, statErr := root.Stat(path); statErr == nil {
		if err = f.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// The rename is the commit point. An already admitted OS rename cannot
	// be rolled back if revocation races with that syscall.
	if err = m.authorize(epoch, "file_write", ai, func() error { return m.checkOperation(ctx) }); err != nil {
		return err
	}
	if err = root.Rename(tmp, path); err != nil {
		return err
	}
	return m.record("file.write.complete", "", struct{ Path, Hash string }{path, hash})
}

func readRegularFile(root fileAccess, path string, limit int64) ([]byte, error) {
	// Reject special files before opening where possible. Verify the opened
	// handle as well because a local rename can race with the first check.
	info, err := root.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	f, err := root.OpenFile(path, regularFileOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	if info.Size() > limit {
		return nil, errors.New("FILE_LIMIT")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("FILE_LIMIT")
	}
	return data, nil
}
