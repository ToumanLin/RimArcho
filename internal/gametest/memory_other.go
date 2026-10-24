//go:build !windows

package gametest

func peakMemoryMB(pid int) int64 { return 0 }
