//go:build windows

package config

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestSavedSettingsAreCurrentUserOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "config.json")
	for _, server := range []string{"https://first.example", "https://second.example"} {
		if e := Save(p, Settings{Server: server}); e != nil {
			t.Fatal(e)
		}
		sd, e := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if e != nil {
			t.Fatal(e)
		}
		control, _, e := sd.Control()
		if e != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("ACL inheritance is not disabled", e)
		}
		acl, _, e := sd.DACL()
		if e != nil || acl == nil || acl.AceCount != 1 {
			t.Fatal("ACL is not private", e)
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if e = windows.GetAce(acl, 0, &ace); e != nil {
			t.Fatal(e)
		}
		user, e := windows.GetCurrentProcessToken().GetTokenUser()
		if e != nil {
			t.Fatal(e)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
			t.Fatal("ACL is not current-user-only")
		}
	}
}
