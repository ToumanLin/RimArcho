//go:build !windows && !linux

package app

// gameProcesses is not implemented outside Windows yet: the game counts as not running.
func gameProcesses() []*GameRun { return nil }

// steamClientRunning is not checked outside Windows and Linux: Steam counts as running,
// so a Steam launch stays one.
func steamClientRunning() bool { return true }
