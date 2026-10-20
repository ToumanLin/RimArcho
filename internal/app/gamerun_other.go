//go:build !windows

package app

// gameProcess is not implemented outside Windows yet: the game counts as not running.
func gameProcess(gameRoot string) *GameRun { return nil }
