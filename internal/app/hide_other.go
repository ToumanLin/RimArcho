//go:build !windows

package app

import (
	"os"
	"os/exec"
)

func hideWindow(*exec.Cmd) {}

// newConsole: elsewhere SteamCMD uses the caller's terminal.
func newConsole(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
}
