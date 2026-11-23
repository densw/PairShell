package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryCRUDPreservesAuditAndAllowsRestore(t *testing.T) {
	s, e := New(filepath.Join(t.TempDir(), "Sessions"))
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(s.Root, "session-real")
	os.Mkdir(dir, 0700)
	original := []byte("original audit\n")
	os.WriteFile(filepath.Join(dir, "events.jsonl"), original, 0600)
	list, e := s.List("", false)
	if e != nil || len(list) != 1 {
		t.Fatal(list, e)
	}
	if _, e = s.Update("session-real", "修复网络", "客户确认完成"); e != nil {
		t.Fatal(e)
	}
	list, _ = s.List("网络", false)
	if len(list) != 1 {
		t.Fatal("search missing")
	}
	if e = s.Delete("session-real", true); e != nil {
		t.Fatal(e)
	}
	list, _ = s.List("", false)
	if len(list) != 0 {
		t.Fatal("deleted item visible")
	}
	list, _ = s.List("", true)
	if len(list) != 1 {
		t.Fatal("trash missing")
	}
	s.Delete("session-real", false)
	after, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if string(after) != string(original) {
		t.Fatal("audit was modified")
	}
	note, e := s.Create("电话备注", "未执行命令")
	if e != nil || note.Kind != "note" {
		t.Fatal(note, e)
	}
	if _, e = s.Get("../outside"); e == nil {
		t.Fatal("traversal accepted")
	}
	if _, e = s.EventsPath(note.ID); e == nil {
		t.Fatal("note fabricated an audit")
	}
}
