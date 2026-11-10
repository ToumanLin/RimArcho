package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// loadingSaveRe is the game's line before it reads a save: "Loading game from file
// <name> with mods:" (the mod list follows as continuation lines). <name> is the file
// without ".rws".
var loadingSaveRe = regexp.MustCompile(`^Loading game from file (.+?) with mods\b`)

// logLoadedSaves lists the saves the logged run loaded, in order and each once (06 §3).
func logLoadedSaves(lf *logs.File) []string {
	var out []string
	for _, e := range lf.Entries {
		m := loadingSaveRe.FindStringSubmatch(e.Message)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[1])
		dup := false
		for _, x := range out {
			dup = dup || x == name
		}
		if name != "" && !dup {
			out = append(out, name)
		}
	}
	return out
}

// MissingSaves are the saves the log's run loaded that are not in the environment's
// Saves/ (nor in the Saves/ beside the log), by file name. A test run cannot reproduce
// errors that need such a save (20 §2). Pasted text has no Saves/ to compare with: nothing
// is reported for it.
func (a *App) MissingSaves(st *LoadoutState, r *LogReport) []string {
	if len(r.LoadedSaves) == 0 || r.Path == "" {
		return nil
	}
	var dirs []string
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		root = a.DefaultUserData()
	}
	if root != "" {
		dirs = append(dirs, filepath.Join(root, "Saves"))
	}
	// 1.7 keeps the log in <user data>/Logs: its Saves/ is the parent's.
	dir := filepath.Dir(r.Path)
	if strings.EqualFold(filepath.Base(dir), "Logs") {
		dir = filepath.Dir(dir)
	}
	dirs = append(dirs, filepath.Join(dir, "Saves"))
	var out []string
	for _, name := range r.LoadedSaves {
		found := false
		for _, d := range dirs {
			if fi, err := os.Stat(filepath.Join(d, name+".rws")); err == nil && !fi.IsDir() {
				found = true
				break
			}
		}
		if !found {
			out = append(out, name)
		}
	}
	return out
}
