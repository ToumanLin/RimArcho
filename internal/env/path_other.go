//go:build !windows

package env

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// PathKey normalises a path for comparison.
func PathKey(p string) string { return filepath.Clean(p) }

func registrySteamPath() string { return "" }

func steamActiveUser() (uint32, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".steam", "registry.vdf"))
	if err != nil {
		return 0, ""
	}
	n, err := vdf.Parse(b)
	if err != nil {
		return 0, ""
	}
	steam := n.Get("Registry").Get("HKCU").Get("Software").Get("Valve").Get("Steam")
	id, err := strconv.ParseUint(steam.Str("ActiveProcess", "ActiveUser"), 10, 32)
	if err != nil || id == 0 {
		return 0, ""
	}
	pid, err := strconv.Atoi(steam.Str("ActiveProcess", "pid"))
	if err != nil || pid <= 0 {
		return 0, ""
	}
	if err := syscall.Kill(pid, 0); err != nil && err != syscall.EPERM {
		return 0, ""
	}
	return uint32(id), steam.Str("AutoLoginUser")
}
