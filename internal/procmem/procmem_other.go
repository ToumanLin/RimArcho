//go:build !windows

// Package procmem reports the current process's memory use.
package procmem

// MB is not implemented outside Windows; it reports zeros.
func MB() (ws, private, peak float64) { return 0, 0, 0 }
