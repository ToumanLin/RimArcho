package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// UnityDataDir locates the Unity data directory beside the game executable.
func UnityDataDir(gameRoot string) string {
	for _, name := range []string{"RimWorldWin64_Data", "RimWorldLinux_Data"} {
		if p := filepath.Join(gameRoot, name); isDir(p) {
			return p
		}
	}
	return filepath.Join(gameRoot, "RimWorldWin64_Data")
}

// IsProtonGame identifies a Windows game in a Steam library with a Proton prefix.
// A native executable takes precedence when both builds are present (03 §3.2).
func IsProtonGame(gameRoot string) bool {
	if _, err := os.Stat(filepath.Join(gameRoot, "RimWorldLinux")); err == nil {
		return false
	}
	if st, err := os.Stat(filepath.Join(gameRoot, "RimWorldWin64.exe")); err != nil || st.IsDir() {
		return false
	}
	return WorkshopRootFor(gameRoot) != "" && isDir(filepath.Join(filepath.Dir(filepath.Dir(gameRoot)), "compatdata", WorkshopAppID, "pfx"))
}

// DefaultUserDataForGame resolves the default for this install without storing a
// build kind in the database; Steam can change a game between native and Proton.
func DefaultUserDataForGame(o DetectOptions, gameRoot string) string {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	return gameUserDataCandidates(o, gameRoot)[0]
}

func gameUserDataCandidates(o DetectOptions, gameRoot string) []string {
	if o.GOOS != "linux" || gameRoot == "" {
		return userDataCandidates(o.GOOS, o.Home)
	}
	if IsProtonGame(gameRoot) {
		return []string{filepath.Join(filepath.Dir(filepath.Dir(gameRoot)), "compatdata", WorkshopAppID, "pfx",
			"drive_c", "users", "steamuser", "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")}
	}
	roots := o.SteamRoots
	if roots == nil {
		roots = defaultSteamRoots(o)
	}
	for _, root := range roots {
		if !flatpakSteamRoot(o.Home, root) {
			continue
		}
		for _, lib := range SteamLibraries([]string{root}) {
			game := filepath.Join(lib, "steamapps", "common", "RimWorld")
			if real, err := filepath.EvalSymlinks(game); err == nil {
				game = real
			}
			candidate := gameRoot
			if real, err := filepath.EvalSymlinks(candidate); err == nil {
				candidate = real
			}
			if PathKey(candidate) == PathKey(game) {
				return userDataCandidates(o.GOOS, o.Home)[1:]
			}
		}
	}
	return userDataCandidates(o.GOOS, o.Home)[:1]
}

func flatpakSteamRoot(home, root string) bool {
	base := filepath.Join(home, ".var", "app", "com.valvesoftware.Steam")
	if real, err := filepath.EvalSymlinks(base); err == nil {
		base = real
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	rel, err := filepath.Rel(base, root)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
