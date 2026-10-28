package env

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
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

// steamActiveUser is the account ID the running Steam client is signed in with (0: Steam is
// not running or nobody is signed in) and the account it signs in automatically.
func steamActiveUser() (uint32, string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam\ActiveProcess`, registry.QUERY_VALUE)
	if err != nil {
		return 0, ""
	}
	defer k.Close()
	id, _, err := k.GetIntegerValue("ActiveUser")
	if err != nil || id == 0 {
		return 0, ""
	}
	// Steam leaves ActiveUser set when it does not exit cleanly: its process must be there.
	pid, _, err := k.GetIntegerValue("pid")
	if err != nil || !processAlive(uint32(pid)) {
		return 0, ""
	}
	auto := ""
	if s, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		auto, _, _ = s.GetStringValue("AutoLoginUser")
		s.Close()
	}
	return uint32(id), auto
}

func processAlive(pid uint32) bool {
	if pid == 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}
