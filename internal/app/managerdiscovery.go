package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// ManagerImport is one read-only source offered by the migration flow.
type ManagerImport struct {
	Manager string `json:"manager"`
	Kind    string `json:"kind"` // list, rimcrow, rimsort-rules, rimsort-aux
	Path    string `json:"path"`
}

// ManagerLocations separates discovery from machine locations for synthetic tests.
// RimPy is RimPy's data folders (AppData\LocalLow\RimPy Mod Manager): its saved lists
// are in ModLists. GameMods are the games' Mods folders: a portable RimCrow, which keeps
// everything beside its exe, links its Mods in as `_Link_<name>` → <install>\mods\<name>,
// so each such link names an install folder wherever it is (07 §6.2).
type ManagerLocations struct{ RimSort, Installs, RimPy, GameMods []string }

// maxListBytes bounds a file read to check that it is a Mod list.
const maxListBytes = 4 << 20

// isModList reports whether a found file reads as a Mod list with at least one Mod, so that
// unrelated .xml, .json or .txt files beside a manager are not offered as its lists.
func isModList(path string) bool {
	if st, err := os.Stat(path); err != nil || st.Size() > maxListBytes {
		return false
	}
	_, err := readListFile(path)
	return err == nil
}

// linkedInstalls are the install folders RimCrow's links in the games' Mods folders point
// into: the folder holding the `mods` folder a `_Link_` entry targets (at most 2,000 entries
// per Mods folder).
func linkedInstalls(modsDirs []string) []string {
	var out []string
	seen := map[string]bool{}
	var links source.LinkReader
	for _, dir := range modsDirs {
		ents, _ := os.ReadDir(dir)
		for i, e := range ents {
			if i >= 2000 {
				break
			}
			if !strings.HasPrefix(strings.ToLower(e.Name()), "_link_") {
				continue
			}
			// RimCrow makes junctions on Windows, which Go does not follow as symlinks
			li, err := links.Read(filepath.Join(dir, e.Name()))
			if err != nil || li.Target == "" {
				continue
			}
			target := li.Target
			if !strings.EqualFold(filepath.Base(filepath.Dir(target)), "mods") {
				continue
			}
			root := filepath.Dir(filepath.Dir(target))
			if k := strings.ToLower(root); !seen[k] {
				seen[k] = true
				out = append(out, root)
			}
		}
	}
	return out
}

// DiscoverManagerImports checks bounded, explicit locations, never whole drives.
func DiscoverManagerImports(loc ManagerLocations) []ManagerImport {
	var out []ManagerImport
	seen := map[string]bool{}
	add := func(manager, kind, path string) {
		path = filepath.Clean(path)
		key := kind + ":" + strings.ToLower(path)
		st, err := os.Stat(path)
		if err == nil && st.Mode().IsRegular() && !seen[key] {
			seen[key] = true
			out = append(out, ManagerImport{manager, kind, path})
		}
	}
	lists := func(manager, dir string) {
		files, _ := os.ReadDir(dir)
		for i, f := range files {
			if i >= 100 {
				break
			}
			if !f.IsDir() {
				switch strings.ToLower(filepath.Ext(f.Name())) {
				case ".xml", ".rml", ".json", ".txt":
					if p := filepath.Join(dir, f.Name()); isModList(p) {
						add(manager, "list", p)
					}
				}
			}
		}
	}

	rootsSeen := map[string]bool{}
	sortRoots := append([]string(nil), loc.RimSort...)
	for _, root := range append(append([]string(nil), loc.Installs...), linkedInstalls(loc.GameMods)...) {
		add("RimCrow", "rimcrow", filepath.Join(root, "data", "mod_manager.db"))

		add("RimCrow", "rimcrow", filepath.Join(root, "mod_manager.db"))
		backups := filepath.Join(root, "backups")
		lists("RimCrow", backups)
		for _, sub := range []string{"today", "earlier", "other"} {
			lists("RimCrow", filepath.Join(backups, sub))
		}
		profiles, _ := os.ReadDir(filepath.Join(backups, "profile"))
		for i, profile := range profiles {
			if i >= 16 {
				break
			}
			if profile.IsDir() {
				lists("RimCrow", filepath.Join(backups, "profile", profile.Name()))
			}
		}

		sortRoots = append(sortRoots, root, filepath.Join(root, "data"))
	}
	for _, root := range sortRoots {
		key := strings.ToLower(filepath.Clean(root))
		if rootsSeen[key] {
			continue
		}
		rootsSeen[key] = true
		add("RimSort", "rimsort-rules", filepath.Join(root, "dbs", "userRules.json"))
		lists("RimSort", filepath.Join(root, "modlists"))
		// each instance's local database: notes, tags, colours (07 §6.4)
		instances, _ := os.ReadDir(filepath.Join(root, "instances"))
		for i, instance := range instances {
			if i >= 16 {
				break
			}
			if instance.IsDir() {
				add("RimSort", "rimsort-aux", filepath.Join(root, "instances", instance.Name(), "aux_metadata.db"))
			}
		}
		add("RimSort", "rimsort-aux", filepath.Join(root, "aux_metadata.db")) // before instances
		var settings struct {
			Config    string `json:"config_folder"`
			Instances map[string]struct {
				Config   string `json:"config_folder"`
				Override string `json:"instance_folder_override"`
			} `json:"instances"`
		}

		settingsPath := filepath.Join(root, "settings.json")
		if st, err := os.Stat(settingsPath); err != nil || st.Size() > 4<<20 {
			continue
		}
		b, err := os.ReadFile(settingsPath)
		if err != nil || len(b) > 4<<20 || json.Unmarshal(b, &settings) != nil {
			continue
		}
		configs := []string{settings.Config}
		for name, instance := range settings.Instances {
			configs = append(configs, instance.Config)
			if instance.Override != "" && name != "Default" {
				add("RimSort", "rimsort-aux", filepath.Join(instance.Override, name, "aux_metadata.db"))
			}
		}
		for _, config := range configs {
			if config == "" {
				continue
			}
			if !filepath.IsAbs(config) {
				config = filepath.Join(root, config)
			}
			for _, p := range []string{filepath.Join(config, "ModsConfig.xml"), filepath.Join(config, "Config", "ModsConfig.xml")} {
				if isModList(p) {
					add("RimSort", "list", p)
				}
			}
		}
	}
	for _, root := range loc.RimPy {
		lists("RimPy", filepath.Join(root, "ModLists"))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manager != out[j].Manager {
			return out[i].Manager < out[j].Manager
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// FindManagerImports looks in this machine's locations (07 §6.2); gameMods are the games'
// Mods folders, whose RimCrow links lead to portable installs.
func FindManagerImports(gameMods []string) []ManagerImport {
	local, roaming := os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")
	loc := ManagerLocations{GameMods: gameMods}
	if local != "" {
		loc.RimSort = append(loc.RimSort, filepath.Join(local, "RimSort"))
	}
	if roaming != "" {
		loc.RimSort = append(loc.RimSort, filepath.Join(roaming, "RimSort"))
	}
	if roaming != "" {
		// RimPy keeps its data beside Roaming, in LocalLow
		loc.RimPy = append(loc.RimPy, filepath.Join(filepath.Dir(roaming), "LocalLow", "RimPy Mod Manager"))
	}
	home, _ := os.UserHomeDir()
	roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(local, "Programs"), local, filepath.Join(home, "Downloads"), filepath.Join(home, "Desktop"), filepath.Join(home, "Games")}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(filepath.Dir(exe)))
	}
	for _, root := range roots {
		if root != "" {
			for _, name := range []string{"RimCrow", "RimModManager", "RimSort"} {
				loc.Installs = append(loc.Installs, filepath.Join(root, name))
			}
		}
	}
	loc.Installs = append(loc.Installs, managerProcessFolders()...)
	loc.Installs = append(loc.Installs, managerRanFolders()...)
	loc.Installs = append(loc.Installs, managerShortcutFolders()...)
	return DiscoverManagerImports(loc)
}

// ManagerSourcesIn is what a folder the player chose holds: a manager's install folder (the
// folder of its .exe when path is one) read like a found install, or a single data file.
func ManagerSourcesIn(path string) []ManagerImport {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	dir := path
	if !st.IsDir() {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		dir = filepath.Dir(path)
	}
	return DiscoverManagerImports(ManagerLocations{Installs: []string{dir}})
}

// GameModsDirs are the Mods folders of every environment's game.
func (a *App) GameModsDirs() []string {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range envs {
		if e.GameRoot != "" {
			out = append(out, filepath.Join(e.GameRoot, "Mods"))
		}
	}
	return out
}
