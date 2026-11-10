//go:build !windows

package app

import (
	"os/exec"
)

func hideWindow(*exec.Cmd) {}

// startConsole uses the caller's terminal on other platforms.
func startConsole(exe string, args []string) (SteamCMDProc, error) {
	return startWorkshopCMD(exe, args, true)
}
