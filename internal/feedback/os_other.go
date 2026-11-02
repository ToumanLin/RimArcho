//go:build !windows

package feedback

import "runtime"

// System is the operating system's name (06 §2.14 Feedback).
func System() string { return runtime.GOOS }
