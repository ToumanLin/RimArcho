package app

import (
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

// lockUpdate holds the update folder's lock file open with no sharing: a second process
// (or a second install in this one) cannot open it. The handle belongs to the process,
// unlike a mutex, so the install may run on any thread.
func lockUpdate(path string) (func(), bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, false
	}
	return func() { windows.CloseHandle(h) }, true
}

// waitExit waits up to limit for process pid to end.
func waitExit(pid string, limit time.Duration) {
	n, err := strconv.ParseUint(pid, 10, 32)
	if err != nil {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(n))
	if err != nil {
		return // already gone
	}
	defer windows.CloseHandle(h)
	windows.WaitForSingleObject(h, uint32(limit/time.Millisecond))
}
