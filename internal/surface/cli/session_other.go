//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// ownSession starts c in a new session: the terminal's hangup (SIGHUP) on close goes to the
// terminal's session, not to the window started from it.
func ownSession(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
