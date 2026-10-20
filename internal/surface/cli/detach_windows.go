//go:build windows

package cli

import "syscall"

// detachedAttr starts the child without archo's console (DETACHED_PROCESS) and in its own
// process group, so closing that console does not close it.
func detachedAttr() *syscall.SysProcAttr {
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	return &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup}
}
