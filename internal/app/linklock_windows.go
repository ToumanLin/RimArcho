package app

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// heldLinks is the mutex names this process holds. A Windows mutex is re-entrant on its
// owner thread, so a second App session on the same goroutine would otherwise take a lock
// its caller holds; flock on Linux refuses that, as another process would be refused.
var heldLinks sync.Map

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
	if _, held := heldLinks.LoadOrStore(key, true); held {
		return nil, fmt.Errorf("another link repair is running for %s", mods)
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		heldLinks.Delete(key)
		return nil, err
	}
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		heldLinks.Delete(key)
		return nil, fmt.Errorf("another link repair is running for %s", mods)
	}
	acquired = true
	return func() {
		windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		heldLinks.Delete(key)
		runtime.UnlockOSThread()
	}, nil
}
