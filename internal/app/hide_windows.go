package app

import (
	"os/exec"
	"syscall"
)

// hideWindow: a child of the GUI (a GUI-subsystem process) opens no console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
