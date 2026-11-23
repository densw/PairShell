//go:build windows

package terminal

import (
	"context"
	"errors"
	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
	"unsafe"
)

type PTY struct {
	*conpty.ConPty
	job windows.Handle
}

func Start(shell, cwd string, env []string, width, height int) (*PTY, error) {
	if shell == "" || width < 1 || height < 1 {
		return nil, errors.New("invalid terminal parameters")
	}
	p, e := conpty.Start(shell, conpty.ConPtyWorkDir(cwd), conpty.ConPtyEnv(env), conpty.ConPtyDimensions(width, height))
	if e != nil {
		return nil, e
	}
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		p.Close()
		return nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		windows.CloseHandle(job)
		p.Close()
		return nil, e
	}
	process, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid()))
	if e != nil {
		windows.CloseHandle(job)
		p.Close()
		return nil, e
	}
	defer windows.CloseHandle(process)
	if e = windows.AssignProcessToJobObject(job, process); e != nil {
		windows.CloseHandle(job)
		p.Close()
		return nil, e
	}
	return &PTY{p, job}, nil
}
func (p *PTY) Close() error                          { e := windows.CloseHandle(p.job); p.ConPty.Close(); return e }
func (p *PTY) Wait(ctx context.Context) (int, error) { n, e := p.ConPty.Wait(ctx); return int(n), e }
