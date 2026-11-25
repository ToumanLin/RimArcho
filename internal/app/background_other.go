//go:build !windows

package app

import "syscall"

// lowerPriority lowers this process's scheduling priority (nice 10). Best effort.
func lowerPriority() { _ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 10) }
