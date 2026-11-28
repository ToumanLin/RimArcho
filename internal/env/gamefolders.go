package env

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxGameFolderEntries bounds how many entries one games folder contributes, so a huge
// folder never slows detection down (03 §3.2).
const maxGameFolderEntries = 500

// GameParents are the folders whose RimWorld* children are candidate non-Steam installs
// (03 §3.2): `<drive>:\Games` and `<drive>:\GOG Games` on every fixed drive on Windows
// (never a network, removable or optical drive), `~/Games` and `~/GOG Games` elsewhere.
// Only folders that exist are returned.
func GameParents(goos, home string) []string {
	var cands []string
	switch goos {
	case "windows":
		for _, d := range fixedDrives() {
			cands = append(cands, d+`Games`, d+`GOG Games`)
		}
	case "darwin":
		return nil
	default:
		if home != "" {
			cands = append(cands, filepath.Join(home, "Games"), filepath.Join(home, "GOG Games"))
		}
	}
	var out []string
	for _, c := range cands {
		if isDir(c) {
			out = append(out, c)
		}
	}
	return out
}

// GameFoldersIn lists the folders named RimWorld* (any case: `RimWorld`, `RimWorld 1.7`,
// `rimworld-gog`) directly inside each parent that look like a game (LooksLikeGameRoot), in
// name order per parent. Each parent is listed once, at most maxGameFolderEntries entries.
func GameFoldersIn(parents []string) []string {
	var out []string
	for _, p := range parents {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		ents, _ := f.ReadDir(maxGameFolderEntries)
		f.Close()
		var names []string
		for _, e := range ents {
			if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "rimworld") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			if g := filepath.Join(p, n); LooksLikeGameRoot(g) {
				out = append(out, g)
			}
		}
	}
	return out
}

// CountModDirs is how many folders dir holds (a Mod folder's Mods, without reading them;
// hidden entries and RimArcho's own links are not counted). -1 when dir cannot be read.
func CountModDirs(dir string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return -1
	}
	n := 0
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(strings.ToLower(name), "_link_") {
			continue
		}
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			n++
		}
	}
	return n
}
