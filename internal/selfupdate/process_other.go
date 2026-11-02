//go:build !windows

package selfupdate

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
