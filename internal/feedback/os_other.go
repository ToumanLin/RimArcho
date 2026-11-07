//go:build !windows

package feedback

import "runtime"

// System is the operating system's name (16 §2.14 Feedback).
func System() string { return runtime.GOOS }
