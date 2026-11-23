// Package history manages local annotations and a recoverable history index.
// Original audit events are never modified by history edits or deletion.
package history

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"pairshell/internal/securefile"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var validID = regexp.MustCompile(`^(session-[A-Za-z0-9_-]{1,128}|note-[a-f0-9]{32})$`)

type Entry struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	Note     string    `json:"note"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Deleted  bool      `json:"deleted"`
	Complete bool      `json:"complete"`
}
type Store struct {
	Root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	path, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = securefile.Mkdir(path); e != nil {
		return nil, e
	}
	if e = securefile.Mkdir(filepath.Join(path, ".history")); e != nil {
		return nil, e
	}
	return &Store{Root: path}, nil
}
func (s *Store) metaPath(id string) string { return filepath.Join(s.Root, ".history", id+".json") }
func (s *Store) read(id string) (Entry, error) {
	if !validID.MatchString(id) {
		return Entry{}, errors.New("invalid history id")
	}
	entry := Entry{ID: id, Kind: "session", Title: "远程协助"}
	if strings.HasPrefix(id, "session-merchant-") {
		entry.Title = "发起协助"
	}
	if strings.HasPrefix(id, "note-") {
		entry.Kind = "note"
		entry.Title = "手动备注"
	}
	if entry.Kind == "session" {
		info, e := os.Lstat(filepath.Join(s.Root, id))
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Entry{}, os.ErrNotExist
		}
		entry.Created = info.ModTime().UTC()
		manifest, e := os.Lstat(filepath.Join(s.Root, id, "manifest.json"))
		entry.Complete = e == nil && manifest.Mode().IsRegular()
	}
	if info, e := os.Lstat(s.metaPath(id)); e == nil {
		if !info.Mode().IsRegular() || info.Size() > 16384 {
			return Entry{}, errors.New("invalid history metadata")
		}
		raw, e := os.ReadFile(s.metaPath(id))
		if e != nil {
			return Entry{}, e
		}
		var saved Entry
		if json.Unmarshal(raw, &saved) != nil || saved.ID != id {
			return Entry{}, errors.New("invalid history metadata")
		}
		entry.Title, entry.Note, entry.Deleted, entry.Updated = saved.Title, saved.Note, saved.Deleted, saved.Updated
		if !saved.Created.IsZero() {
			entry.Created = saved.Created
		}
	} else if entry.Kind == "note" {
		return Entry{}, os.ErrNotExist
	}
	return entry, nil
}
func (s *Store) save(v Entry) error {
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	tmp := filepath.Join(s.Root, ".history", "tmp-"+hex.EncodeToString(nonce))
	if e = securefile.Write(tmp, raw); e != nil {
		return e
	}
	defer os.Remove(tmp)
	return os.Rename(tmp, s.metaPath(v.ID))
}
func (s *Store) List(query string, deleted bool) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := map[string]bool{}
	dirs, e := os.ReadDir(s.Root)
	if e != nil {
		return nil, e
	}
	for _, d := range dirs {
		if d.IsDir() && validID.MatchString(d.Name()) {
			ids[d.Name()] = true
		}
	}
	files, e := os.ReadDir(filepath.Join(s.Root, ".history"))
	if e != nil {
		return nil, e
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "note-") && strings.HasSuffix(f.Name(), ".json") {
			ids[strings.TrimSuffix(f.Name(), ".json")] = true
		}
	}
	if len(ids) > 10000 {
		return nil, errors.New("too many history entries")
	}
	out := []Entry{}
	q := strings.ToLower(strings.TrimSpace(query))
	for id := range ids {
		v, e := s.read(id)
		if e != nil {
			continue
		}
		if v.Deleted != deleted {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Title+" "+v.Note+" "+v.ID), q) {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
func (s *Store) Get(id string) (Entry, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) Create(title, note string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return Entry{}, e
	}
	return s.update(Entry{ID: "note-" + hex.EncodeToString(b), Kind: "note", Created: time.Now().UTC()}, title, note)
}
func (s *Store) update(v Entry, title, note string) (Entry, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "未命名记录"
	}
	if len([]rune(title)) > 100 || len(note) > 8192 {
		return Entry{}, errors.New("标题最多100字，备注最多8KiB")
	}
	v.Title, v.Note, v.Updated = title, note, time.Now().UTC()
	return v, s.save(v)
}
func (s *Store) Update(id, title, note string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil {
		return Entry{}, e
	}
	return s.update(v, title, note)
}
func (s *Store) Delete(id string, deleted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil {
		return e
	}
	v.Deleted = deleted
	v.Updated = time.Now().UTC()
	return s.save(v)
}
func (s *Store) EventsPath(id string) (string, error) {
	v, e := s.Get(id)
	if e != nil {
		return "", e
	}
	if v.Kind != "session" {
		return "", os.ErrNotExist
	}
	p := filepath.Join(s.Root, id, "events.jsonl")
	info, e := os.Lstat(p)
	if e != nil {
		return "", e
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("invalid events file")
	}
	return p, nil
}
