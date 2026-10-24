package app

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

func lockGameLinks(mods string) (func(), error) {
	// Windows mutex ownership belongs to the OS thread, not the goroutine.
	runtime.LockOSThread()
	acquired := false
	defer func() {
		if !acquired {
			runtime.UnlockOSThread()
		}
	}()
	key := sha256.Sum256([]byte(strings.ToLower(mods)))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Local\RimArcho.links.%x`, key))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, err
	}
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("another link repair is running for %s", mods)
	}
	acquired = true
	return func() { windows.ReleaseMutex(h); windows.CloseHandle(h); runtime.UnlockOSThread() }, nil
}
