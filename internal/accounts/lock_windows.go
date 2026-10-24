//go:build windows

package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/sys/windows"
	"runtime"
)

func platformLock(instance string) func() {
	runtime.LockOSThread()
	h := sha256.Sum256([]byte(instance))
	name, _ := windows.UTF16PtrFromString(`Local\RimArcho-account-` + hex.EncodeToString(h[:]))
	m, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		runtime.UnlockOSThread()
		panic("cannot lock account instance")
	}
	result, err := windows.WaitForSingleObject(m, windows.INFINITE)
	if err != nil || (result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED) {
		windows.CloseHandle(m)
		runtime.UnlockOSThread()
		panic("cannot lock account instance")
	}
	return func() { windows.ReleaseMutex(m); windows.CloseHandle(m); runtime.UnlockOSThread() }
}
