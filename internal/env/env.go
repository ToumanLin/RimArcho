// Package env models RimWorld environments (game install + user data) and detects them (02 §2).
package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// Version retains the full game version; unknown numeric components are -1.
type Version struct {
	Full                     string
	Major, Minor, Build, Rev int
}

// ParseVersion accepts a major.minor version with an optional build and revision.
func ParseVersion(s string) Version {
	v := Version{Full: strings.TrimSpace(s), Major: -1, Minor: -1, Build: -1, Rev: -1}
	fields := strings.Fields(v.Full)
	if len(fields) == 0 {
		return v
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) < 2 {
		return v
	}
	// major.minor stands on its own (MajorMinor has always read only those two);
	// build and revision count only in the exact "X.Y.B revR" form
	mj, err1 := strconv.Atoi(parts[0])
	mn, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || mj < 0 || mn < 0 {
		return v
	}
	v.Major, v.Minor = mj, mn
	if len(parts) > 3 || len(fields) > 2 {
		return v
	}
	nums := []int{-1, -1, -1, -1}
	if len(fields) == 2 {
		if len(parts) != 3 || !strings.HasPrefix(fields[1], "rev") {
			return v
		}
		parts = append(parts, strings.TrimPrefix(fields[1], "rev"))
	}
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return v
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v
		}
		nums[i] = n
	}
	v.Build, v.Rev = nums[2], nums[3]
	return v
}

// Short returns the major.minor version, or "" if unknown.
func (v Version) Short() string {
	if v.Major < 0 || v.Minor < 0 {
		return ""
	}
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

func (v Version) HasBuild() bool { return v.Build >= 0 }

// SameBuild compares build and revision only when both builds are recorded.
func (v Version) SameBuild(o Version) (same bool, known bool) {
	if !v.HasBuild() || !o.HasBuild() {
		return false, false
	}
	return v.Build == o.Build && v.Rev == o.Rev, true
}

func (v Version) String() string { return v.Full }

// MajorMinor returns the "1.6" part of a RimWorld version string, or "" if unknown.
func MajorMinor(v string) string { return ParseVersion(v).Short() }

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

// ManagedDir is the folder holding the game's own managed assemblies: the first of
// <game_root>/RimWorldWin64_Data/Managed and the game folder itself (1.7) that contains
// Assembly-CSharp.dll, or "" when neither does (03 §5.3, §5.4).
func ManagedDir(gameRoot string) string {
	for _, d := range []string{filepath.Join(gameRoot, "RimWorldWin64_Data", "Managed"), gameRoot} {
		if _, err := os.Stat(filepath.Join(d, "Assembly-CSharp.dll")); err == nil {
			return d
		}
	}
	return ""
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
