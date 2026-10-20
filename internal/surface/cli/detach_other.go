//go:build !windows

package cli

import "syscall"

// detachedAttr puts the child in its own session, so closing archo's terminal does not end it.
func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
