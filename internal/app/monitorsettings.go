package app

import (
	"encoding/json"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Archo Monitor's settings record (03 §6.7.1) names, for one run, the folder each Mod class
// was seen under and the settings file it read. ArchoMonitor.log keeps only two runs, so
// RimArcho keeps every record it has read per save-data root in meta "mod-settings:<root>":
// a folder name used long ago still leads to its settings (08 §3).

// seenSessionsMax bounds the session ids remembered as read.
const seenSessionsMax = 10

type settingsSeen struct {
	Sessions []string           `json:"sessions"` // read, oldest first
	Files    []SeenSettingsFile `json:"files"`
}

// SeenSettingsFile is a settings file Archo Monitor recorded, with the newest run it was
// recorded in.
type SeenSettingsFile struct {
	logs.SettingsFile
	Game string    `json:"game,omitempty"`
	Seen time.Time `json:"seen"`
}

func settingsSeenKey(root string) string { return "mod-settings:" + env.PathKey(root) }

// MonitorSettings is everything Archo Monitor recorded about Mod settings in the save-data
// root, after reading the records of its logs not read before.
func (a *App) MonitorSettings(root string) []SeenSettingsFile {
	if root == "" {
		return nil
	}
	key := settingsSeenKey(root)
	var seen settingsSeen
	if v := a.Store.Meta(key); v != "" {
		_ = json.Unmarshal([]byte(v), &seen)
	}
	changed := false
	for _, name := range []string{logs.MonitorPrevLog, logs.MonitorLog} {
		ms, err := logs.ReadMonitorSettings(filepath.Join(root, name))
		if err != nil || ms == nil || ms.Session == "" || slices.Contains(seen.Sessions, ms.Session) {
			continue
		}
		seen.merge(ms)
		seen.Sessions = append(seen.Sessions, ms.Session)
		if n := len(seen.Sessions); n > seenSessionsMax {
			seen.Sessions = seen.Sessions[n-seenSessionsMax:]
		}
		changed = true
	}
	if changed {
		if b, err := json.Marshal(seen); err == nil {
			if err := a.Store.SetMeta(key, string(b)); err != nil {
				log.Printf("keeping Archo Monitor's settings record: %v", err)
			}
		}
	}
	return seen.Files
}

func (s *settingsSeen) merge(ms *logs.MonitorSettings) {
	at := map[string]int{}
	for i, f := range s.Files {
		at[seenKey(f.SettingsFile)] = i
	}
	for _, f := range ms.Files {
		if f.PackageID == "" || f.Folder == "" || f.Class == "" {
			continue
		}
		rec := SeenSettingsFile{SettingsFile: f, Game: ms.Game, Seen: ms.Started}
		if i, ok := at[seenKey(f)]; ok {
			if !ms.Started.Before(s.Files[i].Seen) {
				s.Files[i] = rec
			}
			continue
		}
		at[seenKey(f)] = len(s.Files)
		s.Files = append(s.Files, rec)
	}
}

func seenKey(f logs.SettingsFile) string {
	return strings.ToLower(f.PackageID + "\x00" + f.Folder + "\x00" + f.Class)
}

// addSeenFolders adds the folders Archo Monitor saw each package under to folders (per
// package, lower case, without "_steam", as settingsFolders keys them).
func addSeenFolders(folders map[string][]string, files []SeenSettingsFile) {
	for _, f := range files {
		pkg := packageBase(strings.ToLower(f.PackageID))
		if !slices.ContainsFunc(folders[pkg], func(o string) bool { return strings.EqualFold(o, f.Folder) }) {
			folders[pkg] = append(folders[pkg], f.Folder)
		}
	}
}
