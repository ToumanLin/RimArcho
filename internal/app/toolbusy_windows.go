package app

import (
	"errors"
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

// Refuse replacement even when a matching process cannot be inspected. Never
// truncate a running executable and rely on a later write error to protect it.
func toolIdle(name, _ string) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return errors.New("cannot check whether the tool is in use")
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		exe := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if strings.TrimSuffix(exe, ".exe") == name {
			return errors.New("tool is in use; close it before updating or removing it")
		}
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return errors.New("cannot check whether the tool is in use")
	}
	return nil
}
