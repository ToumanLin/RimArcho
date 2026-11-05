package app

// Uninstall (02 §1's uninstall column, 13 §4.2 `archo uninstall`, 16 §2.14): undo every row of
// the write list. The plan is computed first and shown as a dry run; Uninstall carries it
// out. What holds the player's things (lock library copies, packs, saves, downloaded games)
// is a choice; RimArcho's own folders are deleted; the data folders and the program go last,
// by a script that waits for RimArcho to close (Windows cannot delete a running program or
// an open database).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The steps, in the order they run. 02 §1's uninstall column names them.
const (
	UnAccounts   = "accounts"   // forget sessions before removing tools or data
	UnLinks      = "links"      // _Archo_ and !ArchoPack_ links in each game's Mods/
	UnModsConfig = "modsconfig" // ModsConfig.xml in each user data folder RimArcho wrote
	UnTextures   = "textures"   // the DDS RimArcho wrote next to mods' PNGs (Texture optimisation)
	UnLocked     = "locked"     // the lock library's copies
	UnMonitor    = "monitor"    // Archo Monitor's folder
	UnPacks      = "packs"      // packs and the saves inside them
	UnSaves      = "saves"      // the "own" save folders and save backups in the data folder
	UnGames      = "games"      // game folders downloaded with SteamCMD
	UnSteamCMD   = "steamcmd"   // the SteamCMD RimArcho downloaded
	UnSource     = "source"     // relocated, regenerable source library
	UnData       = "data"       // %APPDATA%\RimArcho and %LOCALAPPDATA%\RimArcho
	UnProgram    = "program"    // the program files
)

// UninstallSteps lists every step in order.
var UninstallSteps = []string{UnAccounts, UnLinks, UnModsConfig, UnTextures, UnLocked, UnMonitor, UnPacks, UnSaves, UnGames, UnSteamCMD, UnSource, UnData, UnProgram}

// What happens to an item.
const (
	DoUnlink  = "unlink"     // the link is deleted, never its target
	DoClean   = "clean"      // ModsConfig.xml keeps the list without the mods that go away
	DoRestore = "restore"    // ModsConfig.xml.rimarcho.bak goes back
	DoTrash   = "trash"      // to the recycle bin; refused when there is none (02 §3)
	DoMove    = "move"       // to Item.To (renamed, or copied, checked and the original trashed)
	DoKeep    = "keep"       // stays where it is
	DoDelete  = "delete"     // RimArcho's own: deleted
	DoLater   = "after-exit" // deleted once RimArcho has closed
	DoLeave   = "leave"      // left alone; Note says why
	DoDDS     = "delete-dds" // the DDS RimArcho recorded in a mod folder are deleted, nothing else
)

// UninstallOptions are the player's answers; the zero value is the defaults.
type UninstallOptions struct {
	KeepLocked  bool   // move the lock library's copies into a game's Mods/ (default: recycle bin)
	LockedTo    string // the environment whose Mods/ gets them ("" the current one)
	RemovePacks bool   // packs to the recycle bin (default: kept)
	PacksTo     string // where kept packs inside the data folder go ("" the default)
	RemoveSaves bool   // the own save folders to the recycle bin (default: moved out)
	SavesTo     string // where they are moved ("" the default)
	RemoveGames bool   // downloaded game folders to the recycle bin (default: kept)
	// RestoreBackup puts ModsConfig.xml.rimarcho.bak back instead of cleaning the list.
	RestoreBackup bool
}

// UninstallItem is one thing a step acts on.
type UninstallItem struct {
	Path  string
	Name  string // a mod's or pack's name, an environment's ("" none)
	Do    string
	To    string // DoMove: the destination
	Bytes int64
	Note  string   // DoLeave: why; otherwise what changes
	Drop  []string // DoClean: the entries taken out of the list
	// Set by Uninstall.
	Done  bool
	Err   string
	Links *LinkLifecyclePlan
}

// UninstallStep is one row of the plan.
type UninstallStep struct {
	Key   string
	Items []*UninstallItem
}

// Bytes is the size of the items the step acts on (not those it keeps or leaves).
func (s *UninstallStep) Bytes() int64 {
	var n int64
	for _, it := range s.Items {
		if it.Do != DoKeep && it.Do != DoLeave {
			n += it.Bytes
		}
	}
	return n
}

// UninstallPlan is what Uninstall would do.
type UninstallPlan struct {
	Options  UninstallOptions
	Steps    []*UninstallStep // only the steps with something to do, in order
	Envs     []string         // environments with a game folder: where kept copies can go
	LockedTo string           // the environment the kept copies go to ("" none)
	SavesTo  string
	PacksTo  string
	Game     *RunningGame // a game runs: Uninstall refuses
	Others   []int        // other RimArcho processes from this program folder: refused
	Problem  string       // the answers cannot be carried out (a kept folder would be deleted…)
	Home     string
	Local    string
	// AfterExit is what the script deletes once RimArcho has closed.
	AfterExit AfterExit
	// Log is where the script lists anything it could not delete.
	Log string

	EstimatedSizes bool // preview bytes may come from a short-lived cache
}

// AfterExit is the last step: folders deleted whole, files, and folders removed only when
// they are empty (the program folder, %LOCALAPPDATA%\RimArcho when it keeps something).
type AfterExit struct {
	Dirs  []string
	Files []string
	Empty []string
}

// Step returns the plan's step key, nil when it has nothing to do.
func (p *UninstallPlan) Step(key string) *UninstallStep {
	for _, s := range p.Steps {
		if s.Key == key {
			return s
		}
	}
	return nil
}

// ErrUninstallBlocked marks a plan that cannot run now.
var ErrUninstallBlocked = errors.New("cannot uninstall now")

// Blocked says why the plan cannot run now (nil: it can).
func (p *UninstallPlan) Blocked() error {
	switch {
	case p.Game != nil:
		return fmt.Errorf("%w: a game is running: %s; close it first", ErrUninstallBlocked, p.Game.Describe(nil))
	case len(p.Others) > 0:
		return fmt.Errorf("%w: other RimArcho windows or commands are running (process %s); close them first", ErrUninstallBlocked, joinInts(p.Others))
	case p.Problem != "":
		return fmt.Errorf("%w: %s", ErrUninstallBlocked, p.Problem)
	}
	return nil
}

func joinInts(n []int) string {
	s := make([]string, len(n))
	for i, x := range n {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}

// LocalDir is %LOCALAPPDATA%\RimArcho: the default lock library, Monitor, SteamCMD and the
// Steam runtime folder live here.
func (a *App) LocalDir() string {
	base := a.LockBase
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "RimArcho")
}

// KeepBase is where the folders kept on uninstall go by default: the player's Documents.
func (a *App) KeepBase() string {
	if a.UninstallKeepBase != "" {
		return a.UninstallKeepBase
	}
	return documentsDir()
}

// DefaultSavesTo and DefaultPacksTo are the default destinations of kept saves and packs.
func (a *App) DefaultSavesTo() string { return filepath.Join(a.KeepBase(), "RimArcho saves") }
func (a *App) DefaultPacksTo() string { return filepath.Join(a.KeepBase(), "RimArcho packs") }

// releaseFiles are what the release zip puts next to the program (17 §5); only these are
// deleted from the program folder, and the folder only when nothing else is left in it.
// Releases before this one shipped fonts/, so uninstall still removes it if present;
// .archo-update is what RimArcho's own update leaves there (17 §5.1).
var releaseFiles = []string{"archo.exe", "RimArcho.exe", "README.md", "README.zh-CN.md", "CHANGELOG.md", "LICENSE", "fonts", "monitor", "skills", UpdateDirName}

// downloadedGame reports whether a game folder was downloaded with SteamCMD: force_install_dir
// leaves steamapps/appmanifest_294100.acf inside it (a Steam library keeps the manifest beside
// common/, never inside the game folder).
func downloadedGame(root string) bool {
	_, err := os.Stat(filepath.Join(root, "steamapps", "appmanifest_"+env.WorkshopAppID+".acf"))
	return root != "" && err == nil
}

// PlanUninstall works out what Uninstall would do with these answers.
func (a *App) PlanUninstall(o UninstallOptions) (*UninstallPlan, error) {
	return a.planUninstall(o, dirSize)
}

// CheckUninstall rebuilds all execution checks without recursively measuring folders.
// Sizes are display-only; the execution report omits these directory totals.
func (a *App) CheckUninstall(o UninstallOptions) (*UninstallPlan, error) {
	return a.planUninstall(o, func(string) int64 { return 0 })
}

// PlanUninstallPreview reuses only byte estimates. Every path, process and ownership
// check is recomputed; execution uses CheckUninstall, without this cache.
func (a *App) PlanUninstallPreview(o UninstallOptions, sizes *UninstallSizeCache) (*UninstallPlan, error) {
	p, err := a.planUninstall(o, sizes.sizeOf)
	if p != nil {
		p.EstimatedSizes = true
	}
	return p, err
}

func (a *App) planUninstall(o UninstallOptions, sizeOf func(string) int64) (*UninstallPlan, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	p := &UninstallPlan{Options: o, Home: a.Home, Local: a.LocalDir(), Log: a.UninstallLog}
	if p.Log == "" {
		p.Log = filepath.Join(os.TempDir(), "RimArcho-uninstall.log")
	}
	if p.Game, err = a.RunningGame(); err != nil {
		return nil, err
	}
	p.Others = a.otherArcho()
	byName := map[string]*env.Environment{}
	for _, e := range envs {
		if e.GameRoot != "" {
			p.Envs = append(p.Envs, e.Name)
			byName[e.Name] = e
		}
	}
	add := func(key string, items []*UninstallItem) {
		if len(items) > 0 {
			p.Steps = append(p.Steps, &UninstallStep{Key: key, Items: items})
		}
	}
	add(UnAccounts, []*UninstallItem{{Path: a.AccountsDir(), Do: "forget-accounts", Note: "Forget account sessions: delete site sessions and temporary login folders, and clear known authentication caches for all accounts in managed SteamCMD. External caches stay under their owner's control. This cannot be undone and does not sign out other devices."}})

	// Where kept lock library copies go: a game that is not itself removed.
	var lockEnv *env.Environment
	if o.KeepLocked {
		name := o.LockedTo
		if name == "" {
			if cur, err := a.Env(); err == nil && cur.GameRoot != "" {
				name = cur.Name
			} else if len(p.Envs) > 0 {
				name = p.Envs[0]
			}
		}
		lockEnv = byName[name]
		if lockEnv != nil {
			p.LockedTo = lockEnv.Name
		}
		switch {
		case lockEnv == nil && o.LockedTo != "":
			p.Problem = fmt.Sprintf("no environment named %q with a game folder (archo env list)", o.LockedTo)
		case lockEnv == nil:
			p.Problem = "no game folder to move the lock library's copies into; remove them instead"
		case o.RemoveGames && downloadedGame(lockEnv.GameRoot):
			p.Problem = fmt.Sprintf("the copies would go into %s, which is removed too; pick another game or keep the downloaded games", lockEnv.GameRoot)
		default:
			p.LockedTo = lockEnv.Name
		}
	}

	// 1. links, and the package IDs that disappear with them
	gone := map[string]map[string]bool{} // user data key → package IDs only reached through a removed link
	var links []*UninstallItem
	seenMods := map[string]bool{}
	observations, err := a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	owned := map[string]*GameLink{}
	for _, l := range observations {
		owned[env.PathKey(l.Path)] = l
	}
	roots, err := a.knownGameMods()
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		e := &env.Environment{GameRoot: root.game}
		for _, candidate := range envs {
			if env.PathKey(realOrAbs(candidate.GameRoot)) == env.PathKey(root.game) {
				e = candidate
				break
			}
		}
		mods := root.mods
		if mods == "" || seenMods[env.PathKey(mods)] {
			continue
		}
		seenMods[env.PathKey(mods)] = true
		ents, _ := os.ReadDir(mods)
		for _, d := range ents {
			name := d.Name()
			if !managedLink(name) && !packLink(name) {
				continue
			}
			path := filepath.Join(mods, name)
			if !source.IsLink(path) {
				links = append(links, &UninstallItem{Path: path, Do: DoLeave, Note: "not a link: left alone"})
				continue
			}
			it := &UninstallItem{Path: path, Do: DoUnlink}
			l := owned[env.PathKey(path)]
			if l == nil || !l.Owned || l.Code == CodeLinkForeign || l.Code == CodeLinkUnknown || l.Code == CodeLinkRecovery {
				it.Do, it.Note = DoLeave, "ownership or link health is unproven: left alone"
			}
			if tgt, err := source.RealPath(path); err == nil {
				it.To = tgt
				if pkg := scan.ReadDecl(tgt, false).PackageID; pkg != "" {
					k := store.UserDataKey(e.UserDataRoot)
					if gone[k] == nil {
						gone[k] = map[string]bool{}
					}
					gone[k][pkg] = true
				}
			}
			links = append(links, it)
		}
	}
	add(UnLinks, links)

	// 3. the lock library (planned before ModsConfig.xml: a kept copy stays in its game)
	var locked []*UninstallItem
	keptPkgs := map[string]bool{}
	lockRoot := a.LockDir()
	used := map[string]bool{}
	for _, dir := range lockCopies(lockRoot) {
		d := scan.ReadDecl(dir, false)
		it := &UninstallItem{Path: dir, Name: d.Name, Bytes: sizeOf(dir), Do: DoTrash}
		if o.KeepLocked && lockEnv != nil {
			mods := a.modsFolder(lockEnv)
			name := freeNameUsed(mods, filepath.Base(dir), used)
			it.Do, it.To = DoMove, filepath.Join(mods, name)
			if d.PackageID != "" {
				keptPkgs[d.PackageID] = true
			}
		}
		locked = append(locked, it)
	}

	// 2. ModsConfig.xml of each user data folder RimArcho wrote
	var mcs []*UninstallItem
	seenUD := map[string]bool{}
	for _, e := range envs {
		k := store.UserDataKey(e.UserDataRoot)
		if k == "" || seenUD[k] {
			continue
		}
		seenUD[k] = true
		path := env.ModsConfigPath(e.UserDataRoot)
		bak := path + loadout.BackupSuffix
		st, err := a.Store.ModsConfigStateAt(e.UserDataRoot)
		if err != nil {
			return nil, err
		}
		_, bakErr := os.Stat(bak)
		if st.WrittenAt.IsZero() && bakErr != nil {
			continue // RimArcho never wrote it
		}
		it := &UninstallItem{Path: path, Name: e.Name}
		if o.RestoreBackup && bakErr == nil {
			it.Do, it.Note = DoRestore, "the list from before RimArcho's last save"
			mcs = append(mcs, it)
			continue
		}
		drop := map[string]bool{MonitorPackage: true}
		for pkg := range gone[k] {
			drop[pkg] = true
		}
		present, err := a.presentPackages(envs, k)
		if err != nil {
			return nil, err
		}
		if lockEnv != nil && store.UserDataKey(lockEnv.UserDataRoot) == k {
			for pkg := range keptPkgs {
				present[pkg] = true
			}
		}
		if mc, err := loadout.ReadModsConfig(path); err == nil {
			for _, id := range mc.Active {
				if drop[id] && !present[id] {
					it.Drop = append(it.Drop, id)
				}
			}
		}
		if len(it.Drop) == 0 && bakErr != nil {
			continue // nothing to take out, nothing of RimArcho's left beside it
		}
		it.Do = DoClean
		if o.RestoreBackup {
			it.Note = "no backup to restore: cleaned instead"
		}
		mcs = append(mcs, it)
	}
	add(UnModsConfig, mcs)
	tex, err := a.uninstallTextures()
	if err != nil {
		return nil, err
	}
	add(UnTextures, tex)
	add(UnLocked, locked)

	// 4. Archo Monitor's folder (RimArcho's own)
	var mon []*UninstallItem
	monRoot := a.MonitorDir()
	if ents, err := os.ReadDir(monRoot); err == nil {
		for _, d := range ents {
			path := filepath.Join(monRoot, d.Name())
			if strings.HasPrefix(d.Name(), monitorFolder) || strings.HasSuffix(d.Name(), TmpSuffix) {
				mon = append(mon, &UninstallItem{Path: path, Do: DoDelete, Bytes: sizeOf(path)})
			}
		}
	}
	add(UnMonitor, mon)

	// 5. packs: kept where they are, or moved out of the data folder, or to the recycle bin
	var packs []*UninstallItem
	packsDir := a.PacksDir()
	inHome := source.Within(packsDir, a.Home)
	p.PacksTo = o.PacksTo
	if p.PacksTo == "" {
		p.PacksTo = a.DefaultPacksTo()
	}
	if ents, err := os.ReadDir(packsDir); err == nil {
		used := map[string]bool{}
		for _, d := range ents {
			if !d.IsDir() {
				continue
			}
			path := filepath.Join(packsDir, d.Name())
			if strings.HasSuffix(d.Name(), TmpSuffix) {
				packs = append(packs, &UninstallItem{Path: path, Do: DoDelete, Bytes: sizeOf(path), Note: "an unfinished build"})
				continue
			}
			pk, err := a.ReadPack(d.Name())
			if err != nil {
				continue // not a pack: inside the data folder it goes with it, elsewhere it stays
			}
			it := &UninstallItem{Path: path, Name: pk.Name, Bytes: sizeOf(path), Do: DoKeep}
			switch {
			case o.RemovePacks:
				it.Do = DoTrash
			case inHome:
				it.Do, it.To = DoMove, filepath.Join(p.PacksTo, freeNameUsed(p.PacksTo, d.Name(), used))
			}
			packs = append(packs, it)
		}
	}
	add(UnPacks, packs)

	// 6. the own save folders (<data>/userdata/<env>) and save backups
	var saves []*UninstallItem
	p.SavesTo = o.SavesTo
	if p.SavesTo == "" {
		p.SavesTo = a.DefaultSavesTo()
	}
	usedS := map[string]bool{}
	saveItem := func(path, name string) {
		it := &UninstallItem{Path: path, Name: name, Bytes: sizeOf(path), Do: DoTrash}
		if !o.RemoveSaves {
			it.Do, it.To = DoMove, filepath.Join(p.SavesTo, freeNameUsed(p.SavesTo, name, usedS))
		}
		saves = append(saves, it)
	}
	if ents, err := os.ReadDir(a.OwnSavesRoot()); err == nil {
		for _, d := range ents {
			if d.IsDir() {
				saveItem(filepath.Join(a.OwnSavesRoot(), d.Name()), d.Name())
			}
		}
	}
	if bk := filepath.Join(a.Home, "backups", "saves"); !emptyDir(bk) {
		saveItem(bk, "backups")
	}
	add(UnSaves, saves)

	// 7. game folders downloaded with SteamCMD
	var games []*UninstallItem
	for _, e := range envs {
		if downloadedGame(e.GameRoot) {
			it := &UninstallItem{Path: e.GameRoot, Name: e.Name, Bytes: sizeOf(e.GameRoot), Do: DoKeep}
			if o.RemoveGames {
				it.Do = DoTrash
			}
			games = append(games, it)
		}
	}
	add(UnGames, games)

	// Sources other than the lock library and Monitor never go: they hold the player's mods.
	var keep []string
	if all, err := a.Store.AllSources(); err == nil {
		for _, s := range all {
			if s.Status == source.StatusRetired || s.Root == "" || a.IsLockSource(s) || env.PathKey(s.Root) == env.PathKey(monRoot) {
				continue
			}
			keep = append(keep, s.Root)
		}
	}

	// 8. RimArcho's SteamCMD
	var scmd []*UninstallItem
	if dir := a.SteamCMDDir(); dirExists(dir) {
		it := &UninstallItem{Path: dir, Do: DoDelete, Bytes: sizeOf(dir)}
		switch {
		case a.Store.Meta("cfg:steamcmd.path") != "" && !source.Within(dir, a.LocalDir()):
			it.Do, it.Note = DoLeave, "SteamCMD is in a custom directory: left alone"
			keep = append(keep, dir)
		case a.steamCMDContentCheck(dir) != nil || holdsAny(dir, keep):
			it.Do, it.Note = DoLeave, "holds mods downloaded with SteamCMD: left alone"
			keep = append(keep, dir)
		}
		scmd = append(scmd, it)
	}
	add(UnSteamCMD, scmd)

	sourceRoot := a.SourceDir()

	// Kept and moved things must not land where the data folders are deleted.
	for _, dst := range []struct{ dir, what string }{{p.SavesTo, "saves"}, {p.PacksTo, "packs"}} {
		if source.Within(dst.dir, a.Home) || source.Within(dst.dir, p.Local) || source.Within(directoryReal(dst.dir), directoryReal(sourceRoot)) {
			if p.Problem == "" {
				p.Problem = fmt.Sprintf("the %s would be moved to %s, which is deleted too; pick a folder outside RimArcho's folders", dst.what, dst.dir)
			}
		}
	}

	// 9. the data folders
	var data []*UninstallItem
	if isArchoHome(a.Home) {
		data = append(data, &UninstallItem{Path: a.Home, Do: DoLater, Bytes: sizeOf(a.Home)})
		p.AfterExit.Dirs = append(p.AfterExit.Dirs, a.Home)
	} else {
		data = append(data, &UninstallItem{Path: a.Home, Do: DoLeave, Note: "not a RimArcho data folder (no archo.db): left alone"})
	}
	if dirExists(p.Local) {
		var kept []string
		for _, k := range keep {
			if source.Within(k, p.Local) {
				kept = append(kept, k)
			}
		}
		it := &UninstallItem{Path: p.Local, Do: DoLater, Bytes: sizeOf(p.Local)}
		if len(kept) == 0 {
			p.AfterExit.Dirs = append(p.AfterExit.Dirs, p.Local)
		} else {
			it.Note = "keeps " + strings.Join(kept, ", ")
			ents, _ := os.ReadDir(p.Local)
			for _, d := range ents {
				path := filepath.Join(p.Local, d.Name())
				if !holdsAny(path, kept) {
					p.AfterExit.Dirs = append(p.AfterExit.Dirs, path)
				}
			}
			p.AfterExit.Empty = append(p.AfterExit.Empty, p.Local)
		}
		data = append(data, it)
	}
	// Name only managed tools actually covered by the scheduled data deletions.
	for _, name := range []string{deps.Todds} {
		i := a.Deps().Status(name)
		if !i.Own {
			continue
		}
		deleted := false
		for _, dir := range p.AfterExit.Dirs {
			deleted = deleted || source.Within(i.Dir, dir)
		}
		if !deleted {
			continue
		}
		label := name
		for _, it := range data {
			if it.Do == DoLater && source.Within(i.Dir, it.Path) {
				if it.Note != "" {
					it.Note += "; "
				}
				it.Note += "includes the downloaded " + label
			}
		}
	}
	sourceCovered := false
	for _, dir := range p.AfterExit.Dirs {
		sourceCovered = sourceCovered || source.Within(sourceRoot, dir)
	}
	// A source library inside a deleted data folder goes with it; a relocated one is its own
	// step, and a link there blocks the uninstall with the reason on the page.
	if !sourceCovered && dirExists(sourceRoot) {
		if err := a.checkUninstallSource(sourceRoot); err != nil {
			if p.Problem == "" {
				p.Problem = err.Error()
			}
		} else {
			add(UnSource, []*UninstallItem{{Path: sourceRoot, Do: DoDelete, Bytes: sizeOf(sourceRoot)}})
		}
	}
	add(UnData, data)

	// 10. the program
	var prog []*UninstallItem
	exe := a.UninstallExe
	if exe == "" {
		exe, _ = os.Executable()
	}
	switch {
	case a.UninstallExe == "" && buildinfo.Version == "dev":
		prog = append(prog, &UninstallItem{Path: filepath.Dir(exe), Do: DoLeave, Note: "a development build: the program is not removed"})
	case exe != "":
		dir := filepath.Dir(exe)
		for _, n := range releaseFiles {
			path := filepath.Join(dir, n)
			fi, err := os.Lstat(path)
			if err != nil {
				continue
			}
			if fi.IsDir() {
				p.AfterExit.Dirs = append(p.AfterExit.Dirs, path)
			} else {
				p.AfterExit.Files = append(p.AfterExit.Files, path)
			}
		}
		p.AfterExit.Empty = append(p.AfterExit.Empty, dir)
		prog = append(prog, &UninstallItem{Path: dir, Do: DoLater, Bytes: sizeOf(dir), Note: "only RimArcho's own files; the folder goes when nothing else is in it"})
	}
	add(UnProgram, prog)
	var incoming *incomingLinkCheck
	for _, step := range p.Steps {
		if step.Key == UnSource {
			continue // Source removal never changes links outside the recorded library.
		}
		for _, it := range step.Items {
			if it.Do == DoTrash || it.Do == DoMove || it.Do == DoDelete {
				// All targets in this plan use one fresh incoming-link observation.
				if incoming == nil {
					incoming, err = a.readIncomingLinks()
					if err != nil {
						if p.Problem == "" {
							p.Problem = err.Error()
						}
						return p, nil
					}
				}
				it.Links, err = a.planIncomingLinks(it.Path, "", incoming)
				if err != nil && p.Problem == "" {
					p.Problem = err.Error()
				}
			}
		}
	}
	return p, nil
}

// modsFolder is an environment's Mods/ folder: its local source's root, else <game>/Mods.
func (a *App) modsFolder(e *env.Environment) string {
	if srcs, err := a.Store.Sources(e.ID); err == nil {
		for _, s := range srcs {
			if s.Kind == source.Local && s.Status != source.StatusRetired && s.EnvID == e.ID {
				return s.Root
			}
		}
	}
	if e.GameRoot == "" {
		return ""
	}
	return filepath.Join(e.GameRoot, "Mods")
}

// presentPackages are the package IDs the games of the environments using a user data
// folder still find after uninstall: official content, Workshop items and Mods/ folders that
// are not RimArcho's links.
func (a *App) presentPackages(envs []*env.Environment, udKey string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, e := range envs {
		if store.UserDataKey(e.UserDataRoot) != udKey {
			continue
		}
		mods, err := a.Store.UninstallMods(e.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range mods {
			if m.Status != identity.Live || m.PackageID == "" {
				continue
			}
			switch {
			case m.Kind == source.Official, m.Kind == source.Workshop:
			case m.Kind == source.Local && !managedLink(filepath.Base(m.Rel)) && !packLink(filepath.Base(m.Rel)):
			default:
				continue
			}
			out[m.PackageID] = true
		}
	}
	return out, nil
}

// lockCopies lists the mod folders of the lock library, including those in the
// per-environment folders (<lock.dir>/<env>/<mod>) older versions made.
func lockCopies(root string) []string {
	var out []string
	ents, _ := os.ReadDir(root)
	for _, d := range ents {
		path := filepath.Join(root, d.Name())
		if !d.IsDir() || source.IsLink(path) || strings.HasSuffix(d.Name(), TmpSuffix) {
			continue
		}
		if source.HasAbout(path) {
			out = append(out, path)
			continue
		}
		sub, _ := os.ReadDir(path)
		for _, s := range sub {
			if p := filepath.Join(path, s.Name()); s.IsDir() && !source.IsLink(p) && source.HasAbout(p) {
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// freeNameUsed is freeName that also avoids the names already planned in this run.
func freeNameUsed(root, base string, used map[string]bool) string {
	name := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil && !used[strings.ToLower(name)] {
			used[strings.ToLower(name)] = true
			return name
		}
		name = fmt.Sprintf("%s%d", base, i)
	}
}

// emptyDir reports whether a folder is missing or has nothing in it.
func emptyDir(p string) bool {
	ents, err := os.ReadDir(p)
	return err != nil || len(ents) == 0
}

// holdsAny reports whether dir is, or contains, one of paths.
func holdsAny(dir string, paths []string) bool {
	for _, k := range paths {
		if source.Within(k, dir) {
			return true
		}
	}
	return false
}

// isArchoHome reports whether a folder is a RimArcho data folder: it has the database.
func isArchoHome(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "archo.db"))
	return err == nil
}

// otherArcho lists the other RimArcho processes started from this program's folder: they
// hold the database open and the program files in use.
func (a *App) otherArcho() []int {
	if a.ArchoProbe != nil {
		return a.ArchoProbe()
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	var out []int
	for _, r := range archoProcesses() {
		if r.PID != os.Getpid() && env.PathKey(filepath.Dir(r.Exe)) == env.PathKey(filepath.Dir(exe)) {
			out = append(out, r.PID)
		}
	}
	return out
}

// UninstallProgress is one item of a running uninstall.
type UninstallProgress struct {
	Step string
	Item string
	I, N int // item i of n in the whole run
}

// Uninstall carries out the plan for these answers. A failed item is reported and the
// others go on; then the data folders and the program are left in place (run it again),
// because they hold what is needed to finish. Otherwise the script that deletes them once
// RimArcho has closed is started.
func (a *App) Uninstall(o UninstallOptions, progress func(UninstallProgress)) (*UninstallPlan, error) {
	p, err := a.CheckUninstall(o)
	if err != nil {
		return nil, err
	}
	if err := p.Blocked(); err != nil {
		return p, err
	}
	if progress == nil {
		progress = func(UninstallProgress) {}
	}
	n, i := 0, 0
	for _, s := range p.Steps {
		if s.Key != UnData && s.Key != UnProgram {
			n += len(s.Items)
		}
	}
	failed := false
	for _, s := range p.Steps {
		if s.Key == UnData || s.Key == UnProgram {
			continue
		}
		for _, it := range s.Items {
			i++
			progress(UninstallProgress{Step: s.Key, Item: it.Path, I: i, N: n})
			if err := a.uninstallItem(s.Key, it); err != nil {
				it.Err = err.Error()
				failed = true
				if s.Key == UnAccounts {
					return p, errors.New("could not forget account sessions; tools and data were kept; close the login window and retry")
				}
			} else if it.Do != DoKeep && it.Do != DoLeave {
				it.Done = true
			}
		}
		switch s.Key {
		case UnLocked:
			removeIfEmpty(a.LockDir())
		case UnMonitor:
			removeIfEmpty(a.MonitorDir())
		case UnPacks:
			if o.RemovePacks || source.Within(a.PacksDir(), a.Home) {
				removeIfEmpty(a.PacksDir())
			}
		}
	}
	last := []*UninstallStep{p.Step(UnData), p.Step(UnProgram)}
	if failed {
		for _, s := range last {
			if s == nil {
				continue
			}
			for _, it := range s.Items {
				if it.Do == DoLater {
					it.Do, it.Note = DoLeave, "kept because a step above failed: fix it, then uninstall again"
				}
			}
		}
		return p, nil
	}
	progress(UninstallProgress{Step: UnData, I: n, N: n})
	a.Uninstalled = true
	run := a.AfterExitFunc
	if run == nil {
		run = func(x AfterExit) error { return startAfterExit(x, p.Log) }
	}
	if err := run(p.AfterExit); err != nil {
		for _, s := range last {
			if s == nil {
				continue
			}
			for _, it := range s.Items {
				if it.Do == DoLater {
					it.Err = "the clean-up could not start: " + err.Error() + "; delete it yourself"
				}
			}
		}
	}
	return p, nil
}

// uninstallItem carries out one item.
func (a *App) uninstallItem(step string, it *UninstallItem) error {
	switch it.Do {
	case "forget-accounts":
		return a.ForgetAccounts()
	case DoKeep, DoLeave:
		return nil
	case DoUnlink:
		return a.applyLinks(filepath.Dir(it.Path), []LinkChange{{Op: "remove", Name: filepath.Base(it.Path)}})
	case DoClean:
		return cleanModsConfig(it.Path, it.Drop)
	case DoRestore:
		return restoreModsConfig(it.Path)
	case DoTrash:
		return a.ToTrash(it.Path)
	case DoDelete:
		if step == UnSource {
			return a.uninstallSource(it.Path)
		}
		return a.withIncomingLinks(it.Path, "", func() error { return removeTree(it.Path) })
	case DoDDS:
		return a.deleteTexturesIn(it.Path)
	case DoMove:
		if err := a.withIncomingLinks(it.Path, "", func() error { return a.moveDir(it.Path, it.To) }); err != nil {
			return err
		}
		if step == UnLocked {
			// The copy is an ordinary mod now: RimArcho's note in it goes.
			os.Remove(filepath.Join(it.To, ".rimarcho", "origin.json"))
			removeIfEmpty(filepath.Join(it.To, ".rimarcho"))
		}
		return nil
	}
	return fmt.Errorf("unknown action %q", it.Do)
}

// moveDir moves a folder: renamed on the same volume, else copied, checked, and the original
// moved to the recycle bin.
func (a *App) moveDir(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s already exists; nothing was moved", to)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if strings.EqualFold(filepath.VolumeName(from), filepath.VolumeName(to)) {
		if err := os.Rename(from, to); err == nil {
			return nil
		}
	}
	return a.copyMove(from, to)
}

// cleanModsConfig takes entries out of a ModsConfig.xml, keeping everything else, and
// removes RimArcho's backup beside it.
func cleanModsConfig(path string, drop []string) error {
	if len(drop) > 0 {
		mc, err := loadout.ReadModsConfig(path)
		if err != nil {
			return err
		}
		out := map[string]bool{}
		for _, d := range drop {
			out[d] = true
		}
		var keep []string
		for _, id := range mc.Active {
			if !out[id] {
				keep = append(keep, id)
			}
		}
		b, err := loadout.Render(mc.Raw, mc.Version, keep, mc.Known)
		if err != nil {
			return err
		}
		if err := replaceFile(path, b); err != nil {
			return err
		}
	}
	return removeBackup(path)
}

// restoreModsConfig puts ModsConfig.xml.rimarcho.bak back in place of ModsConfig.xml.
func restoreModsConfig(path string) error {
	b, err := os.ReadFile(path + loadout.BackupSuffix)
	if err != nil {
		return err
	}
	if err := replaceFile(path, b); err != nil {
		return err
	}
	return removeBackup(path)
}

func removeBackup(path string) error {
	os.Remove(path + ".rimarcho.tmp")
	removeSettingsBackups(filepath.Dir(path))
	if err := os.Remove(path + loadout.BackupSuffix); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// removeSettingsBackups removes the backups RimArcho made before changing Mod settings
// (16 §2.9) in a user data folder's Config folder (configDir): Config/Mod_*.xml.rimarcho.bak
// and HugsLib's. The changed values stay: they are the player's.
func removeSettingsBackups(configDir string) {
	ents, _ := os.ReadDir(configDir)
	for _, e := range ents {
		if n := e.Name(); strings.HasPrefix(n, "Mod_") && strings.HasSuffix(n, ".xml"+loadout.BackupSuffix) && e.Type().IsRegular() {
			os.Remove(filepath.Join(configDir, n))
		}
	}
	os.Remove(filepath.Join(filepath.Dir(configDir), HugsLibSettingsFile+loadout.BackupSuffix))
}

// replaceFile writes a temporary file beside path and renames it over (no backup: the
// backup is what uninstall removes).
func replaceFile(path string, b []byte) error {
	tmp := path + ".rimarcho.tmp"
	if err := writeSyncFile(tmp, b); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func writeSyncFile(path string, b []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// removeTree deletes a folder RimArcho owns. A link inside it is removed as a link; its
// target is never entered.
func removeTree(dir string) error {
	if source.IsLink(dir) {
		return source.RemoveLink(dir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var first error
	for _, d := range ents {
		p := filepath.Join(dir, d.Name())
		var err error
		switch {
		case source.IsLink(p):
			err = source.RemoveLink(p)
		case d.IsDir():
			err = removeTree(p)
		default:
			err = os.Remove(p)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	return os.Remove(dir)
}

func removeIfEmpty(dir string) {
	if dir != "" && emptyDir(dir) {
		os.Remove(dir)
	}
}

// RunAfterExit deletes what the script would, in this process: where a running program and
// an open file can be deleted (not Windows), and in tests.
func RunAfterExit(x AfterExit) []string {
	var left []string
	for _, d := range x.Dirs {
		if err := removeTree(d); err != nil {
			left = append(left, d)
		}
	}
	for _, f := range x.Files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			left = append(left, f)
		}
	}
	for _, d := range x.Empty {
		removeIfEmpty(d)
	}
	return left
}
