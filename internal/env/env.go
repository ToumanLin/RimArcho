// Package env models RimWorld environments (game install + user data) and detects them (02 §2).
package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Environment is one game install plus one user data directory (02 §2.1).
type Environment struct {
	ID           string
	Name         string
	GameRoot     string
	UserDataRoot string
	GameVersion  string // e.g. "1.6.4633 rev1261"
	Launch       string // "direct" or "steam"
	CreatedAt    time.Time
}

// MajorMinor returns the "1.6" part of a RimWorld version string, or "" if unknown.
func MajorMinor(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, " \t"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// ReadGameVersion reads <game_root>/Version.txt.
func ReadGameVersion(gameRoot string) string {
	b, err := os.ReadFile(filepath.Join(gameRoot, "Version.txt"))
	if err != nil {
		return ""
	}
	s := strings.TrimPrefix(string(b), string([]byte{0xEF, 0xBB, 0xBF}))
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

var executables = []string{"RimWorldWin64.exe", "RimWorld.exe", "RimWorldWin.exe", "RimWorldLinux", "RimWorldMac.app"}

// LooksLikeGameRoot reports whether dir contains a RimWorld executable or Data/Core.
func LooksLikeGameRoot(dir string) bool {
	for _, e := range executables {
		if _, err := os.Stat(filepath.Join(dir, e)); err == nil {
			return true
		}
	}
	st, err := os.Stat(filepath.Join(dir, "Data", "Core"))
	return err == nil && st.IsDir()
}

// ModsConfigPath is the user's ModsConfig.xml (read-only for RimArcho in M1).
func ModsConfigPath(userDataRoot string) string {
	return filepath.Join(userDataRoot, "Config", "ModsConfig.xml")
}

// PlayerLog locates Player.log per 02 §2.3: 1.7+ uses Logs/, older versions the root;
// with an unknown version both are checked and the newest wins. Returns "" if none exists.
func PlayerLog(userDataRoot, gameVersion string) string {
	newer := filepath.Join(userDataRoot, "Logs", "Player.log")
	older := filepath.Join(userDataRoot, "Player.log")
	var cands []string
	switch mm := MajorMinor(gameVersion); {
	case mm == "":
		cands = []string{newer, older}
	case VersionLess(mm, "1.7"):
		cands = []string{older}
	default:
		cands = []string{newer}
	}
	best, bestT := "", time.Time{}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && (best == "" || st.ModTime().After(bestT)) {
			best, bestT = c, st.ModTime()
		}
	}
	return best
}

// VersionLess compares dotted numeric versions ("1.10" > "1.9").
func VersionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			if len(pa[i]) != len(pb[i]) {
				return len(pa[i]) < len(pb[i])
			}
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// AppHome is RimArcho's own data directory (07 §3). RIMARCHO_HOME overrides it, which the
// tests use to stay inside a temporary directory.
func AppHome() string {
	if h := os.Getenv("RIMARCHO_HOME"); h != "" {
		return h
	}
	switch runtime.GOOS {
	case "windows":
		if a := os.Getenv("APPDATA"); a != "" {
			return filepath.Join(a, "RimArcho")
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", "RimArcho")
		}
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "rimarcho")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share", "rimarcho")
}
