package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// WorkshopAppID is RimWorld's Steam app ID.
const WorkshopAppID = "294100"

// DetectOptions makes detection testable; nil/zero values mean "use this machine".
type DetectOptions struct {
	SteamRoots   []string // candidate Steam client installs
	ManualRoots  []string // candidate non-Steam game installs
	Home         string
	GOOS         string
	SkipRegistry bool
}

// Detected is a suggestion only; nothing is written until the player confirms (02 §2.2).
type Detected struct {
	GameRoot     string
	UserDataRoot string
	WorkshopRoot string
	GameVersion  string
	Launch       string
	SteamLibrary string
	Tried        []string // locations that were checked, in order
}

// Detect runs the 02 §2.2 search order; each step stops at the first hit.
func Detect(o DetectOptions) Detected {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	if o.SteamRoots == nil {
		o.SteamRoots = defaultSteamRoots(o)
	}
	if o.ManualRoots == nil {
		o.ManualRoots = defaultManualRoots(o.GOOS, o.Home)
	}
	var d Detected
	for _, lib := range SteamLibraries(o.SteamRoots) {
		g := filepath.Join(lib, "steamapps", "common", "RimWorld")
		d.Tried = append(d.Tried, g)
		if LooksLikeGameRoot(g) {
			d.GameRoot, d.Launch, d.SteamLibrary = g, "steam", lib
			ws := filepath.Join(lib, "steamapps", "workshop", "content", WorkshopAppID)
			if isDir(ws) {
				d.WorkshopRoot = ws
			}
			break
		}
	}
	if d.GameRoot == "" {
		for _, g := range o.ManualRoots {
			d.Tried = append(d.Tried, g)
			if LooksLikeGameRoot(g) {
				d.GameRoot, d.Launch = g, "direct"
				break
			}
		}
	}
	if d.GameRoot != "" {
		d.GameVersion = ReadGameVersion(d.GameRoot)
	}
	for _, u := range userDataCandidates(o.GOOS, o.Home) {
		d.Tried = append(d.Tried, u)
		if isDir(u) {
			d.UserDataRoot = u
			break
		}
	}
	return d
}

// SteamLibraries returns each Steam root plus every library listed in its libraryfolders.vdf,
// de-duplicated, in file order.
func SteamLibraries(steamRoots []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" {
			return
		}
		k := PathKey(p)
		if !seen[k] {
			seen[k] = true
			out = append(out, filepath.Clean(p))
		}
	}
	for _, root := range steamRoots {
		var b []byte
		for _, f := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
			if bb, err := os.ReadFile(f); err == nil {
				b = bb
				break
			}
		}
		if b == nil {
			if isDir(filepath.Join(root, "steamapps")) {
				add(root)
			}
			continue
		}
		add(root)
		n, err := vdf.Parse(b)
		if err != nil {
			continue
		}
		lf := n.Get("libraryfolders")
		if lf == nil {
			continue
		}
		for _, c := range lf.Children {
			if c.Children == nil { // very old format: "1" "D:\\SteamLibrary"
				if c.Key != "" && c.Key[0] >= '0' && c.Key[0] <= '9' {
					add(c.Value)
				}
			} else {
				add(c.Str("path"))
			}
		}
	}
	return out
}

func defaultSteamRoots(o DetectOptions) []string {
	var roots []string
	if !o.SkipRegistry {
		if r := registrySteamPath(); r != "" {
			roots = append(roots, filepath.FromSlash(r))
		}
	}
	switch o.GOOS {
	case "windows":
		roots = append(roots, `C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`)
	case "darwin":
		roots = append(roots, filepath.Join(o.Home, "Library", "Application Support", "Steam"))
	default:
		roots = append(roots, filepath.Join(o.Home, ".steam", "steam"), filepath.Join(o.Home, ".local", "share", "Steam"),
			filepath.Join(o.Home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"))
	}
	return roots
}

func defaultManualRoots(goos, home string) []string {
	switch goos {
	case "windows":
		return []string{`C:\Games\RimWorld`, `D:\Games\RimWorld`, `C:\GOG Games\RimWorld`, `D:\GOG Games\RimWorld`, `C:\Program Files\RimWorld`, `C:\Program Files (x86)\RimWorld`}
	case "darwin":
		return []string{"/Applications/RimWorld.app", "/Applications/RimWorld"}
	default:
		return []string{filepath.Join(home, "GOG Games", "RimWorld"), filepath.Join(home, "Games", "RimWorld")}
	}
}

func userDataCandidates(goos, home string) []string {
	switch goos {
	case "windows":
		return []string{filepath.Join(home, "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")}
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "RimWorld")}
	default:
		return []string{
			filepath.Join(home, ".config", "unity3d", "Ludeon Studios", "RimWorld by Ludeon Studios"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".config", "unity3d", "Ludeon Studios", "RimWorld by Ludeon Studios"),
		}
	}
}

// WorkshopRootFor returns the workshop content dir of the Steam library holding gameRoot
// (…/steamapps/common/RimWorld → …/steamapps/workshop/content/294100), or "".
func WorkshopRootFor(gameRoot string) string {
	common := filepath.Dir(filepath.Clean(gameRoot))
	steamapps := filepath.Dir(common)
	if !strings.EqualFold(filepath.Base(common), "common") || !strings.EqualFold(filepath.Base(steamapps), "steamapps") {
		return ""
	}
	return filepath.Join(steamapps, "workshop", "content", WorkshopAppID)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// DefaultUserDataRoot is where the game keeps user data when not told otherwise.
func DefaultUserDataRoot() string {
	h, _ := os.UserHomeDir()
	return userDataCandidates(runtime.GOOS, h)[0]
}

// SteamExe locates the Steam client executable ("" if not found).
func SteamExe(o DetectOptions) string {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	roots := o.SteamRoots
	if roots == nil {
		roots = defaultSteamRoots(o)
	}
	for _, r := range roots {
		for _, name := range []string{"steam.exe", "steam.sh", "steam"} {
			p := filepath.Join(r, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}
