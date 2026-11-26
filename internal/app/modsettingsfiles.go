package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The Config folder as a whole (16 §2.9 "Mod settings": all files, cleanup, copy from
// another Game Config). Nothing ever deletes a Mod's settings file: uninstalling a Mod leaves
// it behind. RimArcho lists every file, tells which Mod a Mod_*.xml file belongs to, moves
// the player's choice of them to the recycle bin (02 §1, §3) and copies settings between
// Game Configs, recycling a file it replaces first.

// Where a file in Config/ stands.
const (
	ConfigFileListed   = "listed"   // a Mod of the current list reads it
	ConfigFileUnlisted = "unlisted" // an installed Mod's, not in the current list
	ConfigFileOrphan   = "orphan"   // no installed Mod has this folder name
	ConfigFileGame     = "game"     // not a Mod settings file: the game's own (Prefs.xml, ModsConfig.xml…)
)

// ConfigFile is one file in the Config folder.
type ConfigFile struct {
	File     string
	Path     string
	Size     int64
	Modified time.Time
	Status   string // ConfigFile*
	Folder   string // for a Mod settings file of a known folder: that folder name
	Class    string // the Mod class (the name's last part) when the folder is known
	Mod      string // the Mod's name when known
	Instance string // an installed Mod with this folder name ("" for an orphan)
}

// Recyclable: a Mod settings file (Mod_*.xml), which RimArcho may move to the recycle bin.
func (f ConfigFile) Recyclable() bool { return f.Status != ConfigFileGame }

// ConfigFiles is every file of the current Game Config's Config folder.
type ConfigFiles struct {
	Plan   string
	Config string
	Files  []ConfigFile
}

// isModSettingsName: the game's Mod settings file name, Mod_<folder>_<class>.xml.
func isModSettingsName(name string) bool {
	low := strings.ToLower(name)
	return strings.HasPrefix(low, "mod_") && strings.HasSuffix(low, ".xml") && len(low) > len("mod_.xml") &&
		!strings.ContainsAny(name, `/\`)
}

// ModSettingsFiles lists every file in the current Game Config's Config folder and where each
// stands: read by the current list, an installed Mod's not in the list, an orphan (no
// installed Mod has its folder name), or the game's own.
func (a *App) ModSettingsFiles() (*ConfigFiles, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return nil, errors.New("the Game Config has no user data directory; set one with archo env set --user-data <dir>")
	}
	cfg := filepath.Join(root, "Config")
	out := &ConfigFiles{Plan: PlanRef(st.Plan), Config: cfg}
	ents, err := os.ReadDir(cfg)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	// folder (lower case) → the Mod the game sees under it; listed folders win.
	type owner struct {
		folder string
		mod    *store.Mod
		listed bool
	}
	owners := map[string]owner{}
	stems := map[string]bool{}
	addOwner := func(folder string, m *store.Mod, listed bool) {
		if folder == "" {
			return
		}
		k := strings.ToLower(settingsStem(folder))
		stems[k] = true
		if o, ok := owners[k]; ok && (o.listed || !listed) {
			return
		}
		owners[k] = owner{folder: folder, mod: m, listed: listed}
	}
	for _, en := range st.List {
		if m := st.Ix.ByID[en.Instance]; m != nil && m.Kind != source.Official && st.Reg.Loadable(m.ID) {
			addOwner(st.Reg.Folder(m.ID), m, true)
		}
	}
	// Every folder an installed copy is or was seen under, per package: the package's first
	// installed Mod names it.
	byPkg := map[string]*store.Mod{}
	for _, m := range st.Ix.Mods {
		if m.PackageID == "" || m.Kind == source.Official {
			continue
		}
		pkg := packageBase(strings.ToLower(m.PackageID))
		if byPkg[pkg] == nil {
			byPkg[pkg] = m
		}
	}
	for pkg, folders := range settingsFolders(st.Ix, st.Aliases) {
		for _, f := range folders {
			addOwner(f, byPkg[pkg], false)
		}
	}
	for _, e := range ents {
		if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		f := ConfigFile{File: name, Path: filepath.Join(cfg, name), Size: fi.Size(), Modified: fi.ModTime(), Status: ConfigFileGame}
		if isModSettingsName(name) {
			f.Status = ConfigFileOrphan
			base := strings.ToLower(name[:len(name)-len(".xml")])
			stem := ""
			for i := len("mod_"); i < len(base); i++ {
				if base[i] == '_' && stems[base[:i+1]] {
					stem = base[:i+1]
				}
			}
			if o, ok := owners[stem]; ok && stem != "" && len(stem) < len(base) {
				f.Folder, f.Class = o.folder, name[len(stem):len(name)-len(".xml")]
				f.Status = ConfigFileUnlisted
				if o.listed {
					f.Status = ConfigFileListed
				}
				if o.mod != nil {
					f.Mod, f.Instance = o.mod.DisplayName(), o.mod.ID
				}
			}
		}
		out.Files = append(out.Files, f)
	}
	sort.SliceStable(out.Files, func(i, j int) bool { return strings.ToLower(out.Files[i].File) < strings.ToLower(out.Files[j].File) })
	return out, nil
}

// SettingsRecycle is what RecycleModSettings moved (or would move) to the recycle bin.
type SettingsRecycle struct {
	DryRun bool
	Files  []ConfigFile // moved, or to move in a dry run
	Failed []SettingsFailure
}

// SettingsFailure is one file a write could not handle.
type SettingsFailure struct {
	File string
	Err  string
}

// errGameRunning refuses a Config change while a game runs: the game keeps its settings and
// writes them back when it closes.
func (a *App) settingsGameClosed() error {
	g, err := a.RunningGame()
	if err != nil {
		return err
	}
	if g != nil {
		return inputErr("running", "", "the game is running ("+g.Describe(a.Loc)+"): close it first; a running game keeps its settings and writes them back")
	}
	return nil
}

// RecycleModSettings moves the named Mod settings files (names in the current Game Config's
// Config folder) to the recycle bin (02 §1, §3). Only Mod_*.xml files there; refused while a
// game runs. A file that fails is reported and the rest still move.
func (a *App) RecycleModSettings(names []string, dryRun bool) (*SettingsRecycle, error) {
	if len(names) == 0 {
		return nil, errors.New("no settings file named")
	}
	cf, err := a.ModSettingsFiles()
	if err != nil {
		return nil, err
	}
	byName := map[string]ConfigFile{}
	for _, f := range cf.Files {
		byName[strings.ToLower(f.File)] = f
	}
	var pick []ConfigFile
	seen := map[string]bool{}
	for _, n := range names {
		n = filepath.Base(strings.TrimSpace(n))
		f, ok := byName[strings.ToLower(n)]
		switch {
		case seen[strings.ToLower(n)]:
			continue
		case !ok:
			return nil, fmt.Errorf("%s is not in %s", n, cf.Config)
		case !f.Recyclable():
			return nil, fmt.Errorf("%s is not a Mod options file (Mod_<folder>_<class>.xml); RimArcho leaves the game's own files alone", n)
		}
		seen[strings.ToLower(n)] = true
		pick = append(pick, f)
	}
	return a.recycleSettings(pick, dryRun)
}

func (a *App) recycleSettings(pick []ConfigFile, dryRun bool) (*SettingsRecycle, error) {
	out := &SettingsRecycle{DryRun: dryRun}
	if dryRun {
		out.Files = pick
		return out, nil
	}
	if err := a.settingsGameClosed(); err != nil {
		return nil, err
	}
	for _, f := range pick {
		if err := a.ToTrash(f.Path); err != nil {
			out.Failed = append(out.Failed, SettingsFailure{File: f.File, Err: err.Error()})
			continue
		}
		out.Files = append(out.Files, f)
	}
	return out, nil
}

// ResetModSettings is "reset to defaults" for one Mod (16 §2.9): its Config settings files
// under the folder the game sees it under go to the recycle bin, so the game starts it with
// its defaults. HugsLib packs are not touched (they share one file with other Mods; reset
// their values one by one).
func (a *App) ResetModSettings(m *store.Mod, dryRun bool) (*SettingsRecycle, error) {
	sm, err := a.ModSettingsOf(m)
	if err != nil {
		return nil, err
	}
	var pick []ConfigFile
	for _, f := range sm.Files {
		if f.Kind == "" && f.Exists {
			pick = append(pick, ConfigFile{File: f.File, Path: f.Path, Size: f.Size, Modified: f.Modified, Status: ConfigFileListed,
				Folder: sm.Folder, Class: f.Class, Mod: sm.Name, Instance: m.ID})
		}
	}
	if len(pick) == 0 {
		return nil, inputErr("defaults", "", sm.Name+" has no settings file: it already uses its defaults")
	}
	return a.recycleSettings(pick, dryRun)
}

// SettingsCopy is one file "copy settings from…" writes (16 §2.9).
type SettingsCopy struct {
	Mod      string
	Instance string
	From     string // the file in the other Game Config's Config folder
	To       string // the file name the current list's Mod reads
	Replace  bool   // the current Config folder has To: it goes to the recycle bin first
	Same     bool   // To already has the same bytes: nothing to do
	Err      string
	src, dst string
}

// SettingsCopyPlan is CopyModSettingsFrom's preview or result.
type SettingsCopyPlan struct {
	From   string // the other Game Config's name
	Config string // its Config folder
	DryRun bool
	Copies []SettingsCopy
}

// CopyModSettingsFrom copies the current list's Mods' settings from another Game Config's
// Config folder (16 §2.9 "copy settings from…"): per Mod (refs: instance IDs) or, with none,
// every Mod of the list that has a file there. A file is matched by package across folder
// names (09 §3), the newest per Mod class, and written under the folder the game sees the
// Mod under here. A file already here is replaced only after it went to the recycle bin;
// one with the same bytes is left. Refused while a game runs; dryRun only plans.
func (a *App) CopyModSettingsFrom(from string, instances []string, dryRun bool) (*SettingsCopyPlan, error) {
	src, err := a.Store.EnvByName(from)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, fmt.Errorf("no Game Config named %q (archo env list)", from)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if st.Ix.Env.UserDataRoot == "" {
		return nil, errors.New("the Game Config has no user data directory; set one with archo env set --user-data <dir>")
	}
	srcRoot := src.UserDataRoot
	if srcRoot == "" {
		srcRoot = a.DefaultUserData(src.GameRoot)
	}
	srcCfg := filepath.Join(srcRoot, "Config")
	cfg := filepath.Join(st.Ix.Env.UserDataRoot, "Config")
	if env.PathKey(realOrAbs(srcCfg)) == env.PathKey(realOrAbs(cfg)) {
		return nil, fmt.Errorf("%s shares this Game Config's user data folder: its settings are already these", src.Name)
	}
	want := map[string]bool{}
	for _, id := range instances {
		want[id] = true
	}
	folders := settingsFolders(st.Ix, st.Aliases)
	// the other Game Config's Monitor records name folders its game saw
	addSeenFolders(folders, a.MonitorSettings(srcRoot))
	job := settingsJob{read: []string{srcCfg}, have: cfg, write: cfg, known: allFolders(folders)}
	plan := &SettingsCopyPlan{From: src.Name, Config: srcCfg, DryRun: dryRun}
	var mods []settingsMod
	var ids []string
	for _, en := range st.List {
		m := st.Ix.ByID[en.Instance]
		if m == nil || m.Kind == source.Official || m.PackageID == "" || !st.Reg.Loadable(m.ID) {
			continue
		}
		if len(want) > 0 && !want[m.ID] {
			continue
		}
		delete(want, m.ID)
		mods = append(mods, settingsMod{name: m.DisplayName(), folder: st.Reg.Folder(m.ID),
			old: folders[packageBase(strings.ToLower(m.PackageID))]})
		ids = append(ids, m.ID)
	}
	for id := range want {
		name := id
		if m := st.Ix.ByID[id]; m != nil {
			name = m.DisplayName()
		}
		return nil, fmt.Errorf("%s is not a Mod the current list loads", name)
	}
	job.mods = mods
	stems := map[string]bool{}
	for _, f := range job.known {
		stems[strings.ToLower(settingsStem(f))] = true
	}
	for _, m := range mods {
		stems[strings.ToLower(settingsStem(m.folder))] = true
	}
	files := readSettingsFiles(srcCfg, stems)
	for i, m := range mods {
		// newest file per class across the package's folders there, the same folder included
		found := map[string]settingsFileInfo{}
		for _, o := range append([]string{m.folder}, m.old...) {
			for class, f := range files[strings.ToLower(settingsStem(o))] {
				if cur, ok := found[class]; !ok || f.mod.After(cur.mod) {
					found[class] = f
				}
			}
		}
		classes := make([]string, 0, len(found))
		for c := range found {
			classes = append(classes, c)
		}
		sort.Strings(classes)
		for _, c := range classes {
			f := found[c]
			to := settingsStem(m.folder) + f.class + ".xml"
			cp := SettingsCopy{Mod: m.name, Instance: ids[i], From: f.name, To: to,
				src: filepath.Join(f.dir, f.name), dst: filepath.Join(cfg, to)}
			if fi, err := os.Lstat(cp.dst); err == nil {
				cp.Replace = true
				if fi.Mode().IsRegular() && sameFileBytes(cp.src, cp.dst) {
					cp.Same, cp.Replace = true, false
				}
			}
			plan.Copies = append(plan.Copies, cp)
		}
	}
	if dryRun || len(plan.Copies) == 0 {
		return plan, nil
	}
	if err := a.settingsGameClosed(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return nil, err
	}
	for i := range plan.Copies {
		cp := &plan.Copies[i]
		if cp.Same {
			continue
		}
		if cp.Replace {
			if err := a.ToTrash(cp.dst); err != nil {
				cp.Err = err.Error()
				continue
			}
		}
		if err := writeCopy(cp.src, cp.dst); err != nil {
			cp.Err = err.Error()
		}
	}
	return plan, nil
}

func sameFileBytes(p, q string) bool {
	a, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	b, err := os.ReadFile(q)
	return err == nil && string(a) == string(b)
}

// writeCopy copies src to dst (absent) through a temporary file, keeping the time.
func writeCopy(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	tmp := dst + ".rimarcho-tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Chtimes(tmp, fi.ModTime(), fi.ModTime())
	if _, err := os.Lstat(dst); err == nil {
		os.Remove(tmp)
		return fmt.Errorf("%s appeared meanwhile; left as it is", filepath.Base(dst))
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// settingsFileTrashable: a Mod settings file directly in an environment's user data Config/
// folder (02 §3 "Mod settings file").
// HugsLib's settings file there (HugsLib/ModSettings.xml) counts too: applying a list's
// HugsLib settings rewrites it after the old one went to the recycle bin (07 §7.3).
func (a *App) settingsFileTrashable(p string) bool {
	hugs := strings.EqualFold(filepath.Base(p), filepath.Base(HugsLibSettingsFile)) && strings.EqualFold(filepath.Base(filepath.Dir(p)), "HugsLib")
	if !hugs && !isModSettingsName(filepath.Base(p)) {
		return false
	}
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	dir := filepath.Dir(p)
	if !hugs && !strings.EqualFold(filepath.Base(dir), "Config") {
		return false
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return false
	}
	for _, e := range envs {
		root := e.UserDataRoot
		if root == "" {
			root = a.DefaultUserData(e.GameRoot)
		}
		if root != "" && samePath(dir, filepath.Join(root, filepath.Base(dir))) {
			return true
		}
	}
	return false
}
