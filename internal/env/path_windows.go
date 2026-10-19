package env

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// PathKey normalises a path for comparison: Windows paths compare case-insensitively.
func PathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

func registrySteamPath() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("SteamPath")
	if err != nil {
		return ""
	}
	return v
}
