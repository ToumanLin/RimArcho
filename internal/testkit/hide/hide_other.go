//go:build !windows

package hide

import "os/exec"

// Cmd returns cmd: only Windows opens a console window for a child.
func Cmd(cmd *exec.Cmd) *exec.Cmd { return cmd }
