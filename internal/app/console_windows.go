package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// os/exec supplies NUL for nil streams and sets STARTF_USESTDHANDLES. A new
// console instead needs Windows to create its own input and screen buffers.
func startConsole(exe string, args []string) (SteamCMDProc, error) {
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
	var info windows.ProcessInformation
	if err := windows.CreateProcess(application, command, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, dir, &startup, &info); err != nil {
		return nil, err
	}
	windows.CloseHandle(info.Thread)
	return &consoleProcess{handle: info.Process}, nil
}

type consoleProcess struct {
	mu     sync.Mutex
	handle windows.Handle
	once   sync.Once
	err    error
}

func (p *consoleProcess) Wait() error {
	p.once.Do(func() {
		defer func() { p.mu.Lock(); defer p.mu.Unlock(); windows.CloseHandle(p.handle); p.handle = 0 }()
		if _, err := windows.WaitForSingleObject(p.handle, windows.INFINITE); err != nil {
			p.err = err
			return
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
