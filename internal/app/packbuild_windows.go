package app

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func diskFree(dir string) int64 {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return -1
	}
	var free, total, all uint64
	if windows.GetDiskFreeSpaceEx(p, &free, &total, &all) != nil {
		return -1
	}
	return int64(free)
}

// startDetached starts a child with no console that outlives this process: out of this
// process's job when the job allows it (CREATE_BREAKAWAY_FROM_JOB), else as it is.
func startDetached(mk func() *exec.Cmd) (*exec.Cmd, error) {
	const noWindow, breakaway = 0x08000000, 0x01000000
	cmd := mk()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: noWindow | breakaway}
	if err := cmd.Start(); err == nil {
		return cmd, nil
	}
	cmd = mk() // a Cmd is not started twice
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: noWindow}
	return cmd, cmd.Start()
}
