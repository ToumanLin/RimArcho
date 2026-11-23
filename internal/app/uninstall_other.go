//go:build !windows

package app

// startAfterExit deletes at once: outside Windows a running program and an open database
// can be deleted.
func startAfterExit(x AfterExit, log string) error {
	RunAfterExit(x)
	return nil
}
