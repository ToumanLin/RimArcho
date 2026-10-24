//go:build !windows

package deps

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
