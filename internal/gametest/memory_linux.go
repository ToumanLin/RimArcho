package gametest

import (
	"os/exec"
)

func peakMemoryMB(pid int) int64 { return peakMemoryAt("/proc", pid) }

func detach(*exec.Cmd) {}
