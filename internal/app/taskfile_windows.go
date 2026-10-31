package app

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

// processStart is when process pid started (0: unknown) and whether it still runs.
func processStart(pid uint32) (int64, bool) {
	if pid == 0 {
		return 0, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil || code != 259 { // STILL_ACTIVE
		return 0, false
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return 0, true
	}
	return created.Nanoseconds(), true
}

// lockTaskFile holds the record's lock across processes for one read and write (at most a
// second's wait; without it the write goes ahead).
func lockTaskFile(path string) func() {
	// Windows mutex ownership belongs to the OS thread, not the goroutine.
	runtime.LockOSThread()
	key := sha256.Sum256([]byte(strings.ToLower(path)))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Local\RimArcho.tasks.%x`, key[:16]))
	if err != nil {
		runtime.UnlockOSThread()
		return func() {}
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		runtime.UnlockOSThread()
		return func() {}
	}
	status, err := windows.WaitForSingleObject(h, 1000)
	if err != nil || status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return func() {}
	}
	return func() { windows.ReleaseMutex(h); windows.CloseHandle(h); runtime.UnlockOSThread() }
}
