package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// ManagerFolder is a Mod folder another manager on this computer uses (07 §6.2): RimSort's
// local Mods folder of an instance. The first-run guide offers it as a Mod folder (16 §2.15).
type ManagerFolder struct {
	Manager string `json:"manager"`
	Path    string `json:"path"`
}

// ManagerModFolders reads the Mod folders RimSort's settings.json names in each of the given
// RimSort data folders: the legacy top-level `local_folder` and every instance's. Only
// folders that exist are returned, once each, in path order. Nothing is written.
func ManagerModFolders(rimSort []string) []ManagerFolder {
	var out []ManagerFolder
	seen := map[string]bool{}
	for _, root := range rimSort {
		p := filepath.Join(root, "settings.json")
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() || st.Size() > 4<<20 {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var settings struct {
			Local     string `json:"local_folder"`
			Instances map[string]struct {
				Local string `json:"local_folder"`
			} `json:"instances"`
		}
		if json.Unmarshal(trimBOM(b), &settings) != nil {
			continue
		}
		dirs := []string{settings.Local}
		for _, in := range settings.Instances {
			dirs = append(dirs, in.Local)
		}
		for _, d := range dirs {
			if d == "" || !filepath.IsAbs(d) {
				continue
			}
			d = filepath.Clean(d)
			if k := env.PathKey(d); !seen[k] && dirExists(d) {
				seen[k] = true
				out = append(out, ManagerFolder{Manager: "RimSort", Path: d})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// FindManagerModFolders looks in RimSort's data folders on this computer (07 §6.2:
// %LOCALAPPDATA%\RimSort, %APPDATA%\RimSort); it reads two small files and never searches.
func FindManagerModFolders() []ManagerFolder {
	var roots []string
	for _, v := range []string{"LOCALAPPDATA", "APPDATA"} {
		if d := os.Getenv(v); d != "" {
			roots = append(roots, filepath.Join(d, "RimSort"))
		}
	}
	return ManagerModFolders(roots)
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
