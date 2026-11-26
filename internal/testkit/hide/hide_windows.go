// Package hide starts a test's console children without a console window. It imports
// nothing of RimArcho's, so any package's tests can use it.
package hide

import (
	"os/exec"
	"syscall"
)

// Cmd gives cmd a console with no window (CREATE_NO_WINDOW). A test binary started without
// a console of its own (go test from an agent's shell) otherwise gives each console child a
// new console, and with Windows Terminal as the default terminal each one stayed on screen
// as an error tab (17 §4). archlint's TestArchTestChildrenHidden checks every call is wrapped.
func Cmd(cmd *exec.Cmd) *exec.Cmd {
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	return cmd
}
