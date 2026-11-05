package app

import (
	"encoding/json"
	"log"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Archo Monitor's settings records (06 §7.1) name, for one run, the folder each Mod class
// was seen under and the settings file it read, and what each settings window the player
// opened drew, and HugsLib's settings packs. ArchoMonitor.log keeps only two runs, so RimArcho keeps what it has read per
// save-data root in meta "mod-settings:<root>": a folder name used long ago still leads to
// its settings (09 §3), and a window's labels and HugsLib's titles and defaults stay after
// their run is gone (16 §2.9).

// seenSessionsMax bounds the session ids remembered as read; seenWindowsMax the windows kept
// (the newest).
const (
	seenSessionsMax = 10
	seenWindowsMax  = 80
	seenPacksMax    = 200
)

type settingsSeen struct {
	Sessions []string             `json:"sessions"`       // whose settings record was merged, oldest first
	Read     map[string]int64     `json:"read,omitempty"` // session → bytes of its log read
	Files    []SeenSettingsFile   `json:"files"`
	Windows  []SeenSettingsWindow `json:"windows,omitempty"`
	HugsLib  []SeenHugsLibPack    `json:"hugslib,omitempty"`
}

// SeenSettingsFile is a settings file Archo Monitor recorded, with the newest run it was
// recorded in.
type SeenSettingsFile struct {
	logs.SettingsFile
	Game string    `json:"game,omitempty"`
	Seen time.Time `json:"seen"`
}

// SeenSettingsWindow is the newest record of what a Mod class's settings window drew.
type SeenSettingsWindow struct {
	logs.SettingsWindow
	Game string    `json:"game,omitempty"`
	Seen time.Time `json:"seen"`
}

// SeenHugsLibPack is the newest record of a HugsLib settings pack.
type SeenHugsLibPack struct {
	logs.HugsLibPack
	Game string    `json:"game,omitempty"`
	Seen time.Time `json:"seen"`
}

func settingsSeenKey(root string) string { return "mod-settings:" + env.PathKey(root) }

// MonitorSettings is every settings file Archo Monitor recorded in the save-data root, after
// reading what its logs added since the last read.
func (a *App) MonitorSettings(root string) []SeenSettingsFile { return a.monitorSeen(root).Files }

// monitorSeen reads the records Archo Monitor's logs added since the last read into the kept
// history and returns it.
func (a *App) monitorSeen(root string) settingsSeen {
	var seen settingsSeen
	if root == "" {
		return seen
	}
	key := settingsSeenKey(root)
	if v := a.Store.Meta(key); v != "" {
		_ = json.Unmarshal([]byte(v), &seen)
	}
	if seen.Read == nil {
		seen.Read = map[string]int64{}
	}
	changed := false
	live := map[string]bool{}
	for _, name := range []string{logs.MonitorPrevLog, logs.MonitorLog} {
		mr, err := logs.ReadMonitorRecords(filepath.Join(root, name), seen.Read)
		if err != nil || mr == nil || mr.Session == "" {
			continue
		}
		live[mr.Session] = true
		if mr.End == seen.Read[mr.Session] {
			continue
		}
		if mr.Files != nil && !slices.Contains(seen.Sessions, mr.Session) {
			seen.merge(mr)
			seen.Sessions = append(seen.Sessions, mr.Session)
			if n := len(seen.Sessions); n > seenSessionsMax {
				seen.Sessions = seen.Sessions[n-seenSessionsMax:]
			}
		}
		for _, w := range mr.Windows {
			seen.mergeWindow(w, mr.Game)
		}
		for _, p := range mr.HugsLib {
			seen.mergePack(p, mr.Game, mr.Started)
		}
		seen.Read[mr.Session] = mr.End
		changed = true
	}
	for s := range seen.Read {
		if !live[s] { // its log is gone: it is never read again
			delete(seen.Read, s)
			changed = true
		}
	}
	if changed {
		if b, err := json.Marshal(seen); err == nil {
			if err := a.Store.SetMeta(key, string(b)); err != nil {
				log.Printf("keeping Archo Monitor's settings record: %v", err)
			}
		}
	}
	return seen
}

func (s *settingsSeen) merge(mr *logs.MonitorRecords) {
	at := map[string]int{}
	for i, f := range s.Files {
		at[seenKey(f.SettingsFile)] = i
	}
	for _, f := range mr.Files {
		if f.PackageID == "" || f.Folder == "" || f.Class == "" {
			continue
		}
		rec := SeenSettingsFile{SettingsFile: f, Game: mr.Game, Seen: mr.Started}
		if i, ok := at[seenKey(f)]; ok {
			if !mr.Started.Before(s.Files[i].Seen) {
				s.Files[i] = rec
			}
			continue
		}
		at[seenKey(f)] = len(s.Files)
		s.Files = append(s.Files, rec)
	}
}

// mergeWindow keeps w as its Mod class's window unless a newer one is kept, and at most
// seenWindowsMax windows.
func (s *settingsSeen) mergeWindow(w logs.RecordedWindow, game string) {
	if w.PackageID == "" || w.Class == "" {
		return
	}
	rec := SeenSettingsWindow{SettingsWindow: w.SettingsWindow, Game: game, Seen: w.At}
	k := windowKey(w.PackageID, w.Class)
	if i := slices.IndexFunc(s.Windows, func(o SeenSettingsWindow) bool { return windowKey(o.PackageID, o.Class) == k }); i >= 0 {
		if !w.At.Before(s.Windows[i].Seen) {
			s.Windows[i] = rec
		}
		return
	}
	s.Windows = append(s.Windows, rec)
	if len(s.Windows) > seenWindowsMax {
		sort.SliceStable(s.Windows, func(i, j int) bool { return s.Windows[i].Seen.After(s.Windows[j].Seen) })
		s.Windows = s.Windows[:seenWindowsMax]
	}
}

// mergePack keeps p as its pack's record unless a newer one is kept, and at most seenPacksMax
// packs.
func (s *settingsSeen) mergePack(p logs.HugsLibPack, game string, at time.Time) {
	if p.ID == "" {
		return
	}
	rec := SeenHugsLibPack{HugsLibPack: p, Game: game, Seen: at}
	if i := slices.IndexFunc(s.HugsLib, func(o SeenHugsLibPack) bool { return strings.EqualFold(o.ID, p.ID) }); i >= 0 {
		if !at.Before(s.HugsLib[i].Seen) {
			s.HugsLib[i] = rec
		}
		return
	}
	s.HugsLib = append(s.HugsLib, rec)
	if len(s.HugsLib) > seenPacksMax {
		sort.SliceStable(s.HugsLib, func(i, j int) bool { return s.HugsLib[i].Seen.After(s.HugsLib[j].Seen) })
		s.HugsLib = s.HugsLib[:seenPacksMax]
	}
}

func seenKey(f logs.SettingsFile) string {
	return strings.ToLower(f.PackageID + "\x00" + f.Folder + "\x00" + f.Class)
}

// windowKey is a Mod class across its folders: the window is the Mod's, wherever it was seen.
func windowKey(pkg, class string) string {
	return packageBase(strings.ToLower(pkg)) + "\x00" + strings.ToLower(class)
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
