//go:build !windows

package env

import "path/filepath"

// PathKey normalises a path for comparison.
func PathKey(p string) string { return filepath.Clean(p) }

func registrySteamPath() string { return "" }
