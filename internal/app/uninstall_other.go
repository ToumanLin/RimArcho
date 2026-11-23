//go:build !windows

package app

// cleanupAtOnce: outside Windows a running program and an open database can be deleted, so
// the last two steps run in this process and their result is reported on their items
// (02 §1).
const cleanupAtOnce = true

// startAfterExit deletes at once and returns what could not be deleted.
func startAfterExit(x AfterExit, log string) ([]string, error) {
	return RunAfterExit(x), nil
}
