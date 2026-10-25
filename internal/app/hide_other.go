//go:build !windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
)

func hideWindow(*exec.Cmd) {}

// startConsole uses the caller's terminal on other platforms.
func startConsole(exe string, args []string) (SteamCMDProc, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
