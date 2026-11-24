//go:build !windows

package app

import "errors"

// SteamCMD runs in the caller's terminal on Linux and macOS (11 §3): the player types there;
// RimArcho does not type into a terminal.
var errOwnConsole = errors.New("no SteamCMD console to type into on this platform")

func steamConsoleRead(uint32, bool) ([]string, int, error) { return nil, 0, errOwnConsole }

func steamConsoleType(uint32, bool, *SteamSecret) error { return errOwnConsole }
