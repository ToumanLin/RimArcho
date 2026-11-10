package app

import (
	"errors"
	"syscall"
)

// sharingViolation: another program holds the file (ERROR_SHARING_VIOLATION,
// ERROR_LOCK_VIOLATION).
func sharingViolation(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
