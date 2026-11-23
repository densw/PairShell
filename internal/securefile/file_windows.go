//go:build windows

package securefile

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func attributes(dir bool) (*windows.SecurityAttributes, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return nil, e
	}
	flags := ""
	if dir {
		flags = "OICI"
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + u.User.Sid.String() + ")")
	if e != nil {
		return nil, e
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}
func verify(p string, dir bool) error {
	s, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if s.Mode()&os.ModeSymlink != 0 || s.IsDir() != dir {
		return errors.New("invalid private path type")
	}
	ptr, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return e
	}
	attrs, e := windows.GetFileAttributes(ptr)
	if e != nil {
		return e
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse point forbidden")
	}
	sd, e := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	control, _, e := sd.Control()
	if e != nil {
		return e
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("private ACL must disable inheritance")
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	if acl == nil || acl.AceCount != 1 {
		return errors.New("private ACL must have one entry")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if e = windows.GetAce(acl, 0, &ace); e != nil {
		return e
	}
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return e
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid(sid, u.User.Sid) || ace.Mask&0x001f01ff != 0x001f01ff {
		return errors.New("private ACL is not current user only")
	}
	return nil
}
func Mkdir(p string) error {
	ptr, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return e
	}
	sa, e := attributes(true)
	if e != nil {
		return e
	}
	if s, err := os.Lstat(p); err == nil {
		if !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid private directory")
		}
		a, e := windows.GetFileAttributes(ptr)
		if e != nil {
			return e
		}
		if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("reparse point forbidden")
		}
		acl, _, e := sa.SecurityDescriptor.DACL()
		if e != nil {
			return e
		}
		if e = windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
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
	if e = windows.CreateDirectory(ptr, sa); e != nil {
		return e
	}
	return verify(p, true)
}
func Write(p string, b []byte) error {
	sa, e := attributes(false)
	if e != nil {
		return e
	}
	ptr, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return e
	}
	h, e := windows.CreateFile(ptr, windows.GENERIC_WRITE|windows.READ_CONTROL, 0, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(h), p)
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
