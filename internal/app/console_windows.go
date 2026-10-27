package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// os/exec supplies NUL for nil streams and sets STARTF_USESTDHANDLES. A new
// console instead needs Windows to create its own input and screen buffers.
func startConsole(exe string, args []string) (SteamCMDProc, error) {
	return createSteamProcess(exe, args, windows.CREATE_NEW_CONSOLE, false)
}

// A job owns just this invocation and descendants, including updater children.
// Assign before resuming so cancellation cannot race a spawned child.
func startWorkshopCMD(exe string, args []string, console bool) (SteamCMDProc, error) {
	flags := uint32(windows.CREATE_NO_WINDOW)
	if console {
		flags = windows.CREATE_NEW_CONSOLE
	}
	return createSteamProcess(exe, args, flags, true)
}

func createSteamProcess(exe string, args []string, flags uint32, tracked bool) (SteamCMDProc, error) {
	application, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil, err
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, args...)))
	if err != nil {
		return nil, err
	}
	dir, err := windows.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return nil, err
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var job windows.Handle
	if tracked {
		job, err = windows.CreateJobObject(nil, nil)
		if err != nil {
			return nil, err
		}
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
			windows.CloseHandle(job)
			return nil, err
		}
		flags |= windows.CREATE_SUSPENDED
	}
	var info windows.ProcessInformation
	if err := windows.CreateProcess(application, command, nil, nil, false, flags, nil, dir, &startup, &info); err != nil {
		if job != 0 {
			windows.CloseHandle(job)
		}
		return nil, err
	}
	if tracked {
		err = windows.AssignProcessToJobObject(job, info.Process)
		if err == nil {
			_, err = windows.ResumeThread(info.Thread)
		}
		if err != nil {
			windows.TerminateProcess(info.Process, 1)
			windows.CloseHandle(info.Thread)
			windows.CloseHandle(info.Process)
			windows.CloseHandle(job)
			return nil, err
		}
	}
	windows.CloseHandle(info.Thread)
	return &consoleProcess{handle: info.Process, job: job}, nil
}

type consoleProcess struct {
	mu     sync.Mutex
	handle windows.Handle
	job    windows.Handle
	once   sync.Once
	err    error
}

func (p *consoleProcess) Wait() error {
	p.once.Do(func() {
		defer func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			windows.CloseHandle(p.handle)
			p.handle = 0
			if p.job != 0 {
				windows.CloseHandle(p.job)
				p.job = 0
			}
		}()
		if _, err := windows.WaitForSingleObject(p.handle, windows.INFINITE); err != nil {
			p.err = err
			return
		}
		// SteamCMD can hand off to an updater child and exit. Keep the job and
		// log ownership until its descendants have finished too.
		if p.job != 0 {
			var accounting struct {
				TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
				PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
			}
			for {
				if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
					p.err = err
					return
				}
				if accounting.ActiveProcesses == 0 {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
		var code uint32
		p.err = windows.GetExitCodeProcess(p.handle, &code)
		if p.err == nil && code != 0 {
			p.err = fmt.Errorf("process exited with code %d", code)
		}
	})
	return p.err
}

func (p *consoleProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateProcess(p.handle, 1)
}

func (p *consoleProcess) KillTree() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateJobObject(p.job, 1)
}
