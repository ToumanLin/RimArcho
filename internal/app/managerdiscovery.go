package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManagerImport is one read-only source offered by the migration flow.
type ManagerImport struct {
	Manager string `json:"manager"`
	Kind    string `json:"kind"` // list, rimcrow, rimsort-rules
	Path    string `json:"path"`
}

// ManagerLocations separates discovery from machine locations for synthetic tests.
type ManagerLocations struct{ RimSort, Installs []string }

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
					add(manager, "list", filepath.Join(dir, f.Name()))
				}
			}
		}
	}

	rootsSeen := map[string]bool{}
	sortRoots := append([]string(nil), loc.RimSort...)
	for _, root := range loc.Installs {
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
		var settings struct {
			Config    string `json:"config_folder"`
			Instances map[string]struct {
				Config string `json:"config_folder"`
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
		for _, instance := range settings.Instances {
			configs = append(configs, instance.Config)
		}
		for _, config := range configs {
			if config == "" {
				continue
			}
			if !filepath.IsAbs(config) {
				config = filepath.Join(root, config)
			}
			add("RimSort", "list", filepath.Join(config, "ModsConfig.xml"))
			add("RimSort", "list", filepath.Join(config, "Config", "ModsConfig.xml"))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manager != out[j].Manager {
			return out[i].Manager < out[j].Manager
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func FindManagerImports() []ManagerImport {
	local, roaming := os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")
	loc := ManagerLocations{}
	if local != "" {
		loc.RimSort = append(loc.RimSort, filepath.Join(local, "RimSort"))
	}
	if roaming != "" {
		loc.RimSort = append(loc.RimSort, filepath.Join(roaming, "RimSort"))
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
	loc.Installs = append(loc.Installs, managerShortcutFolders()...)
	return DiscoverManagerImports(loc)
}
