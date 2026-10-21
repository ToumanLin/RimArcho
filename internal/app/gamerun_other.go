//go:build !windows

package app

// gameProcesses is not implemented outside Windows yet: the game counts as not running.
func gameProcesses() []*GameRun { return nil }
