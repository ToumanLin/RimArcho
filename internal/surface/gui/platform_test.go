//go:build windows || cgo

package gui

import (
	"runtime"
	"testing"
)

// windowsOnly skips a test of behaviour that exists only on Windows, such as the managed
// todds download or account sessions; why names the Linux behaviour and where it is tested.
func windowsOnly(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows only: " + why)
	}
}
