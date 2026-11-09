//go:build !windows

package app

// sharingViolation: Windows only; other systems do not lock a file against reading.
func sharingViolation(error) bool { return false }
