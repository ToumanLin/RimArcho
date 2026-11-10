//go:build !windows && !linux

package gametest

import "os/exec"

func peakMemoryMB(pid int) int64 { return 0 }

func detach(*exec.Cmd) {}
