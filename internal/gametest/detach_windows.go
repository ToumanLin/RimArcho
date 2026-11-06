package gametest

import (
	"os/exec"
	"syscall"
)

// detach starts the game without our console: no console window, and Ctrl+C in the
// terminal reaches only RimArcho, which then kills the game itself (20 §2).
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess}
}
