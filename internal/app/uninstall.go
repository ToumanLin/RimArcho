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
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/secret"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
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
	UnLooks      = "looks"      // the generated Mod _Archo_Looks in the looks directory (21 §4)
	UnPacks      = "packs"      // packs and the saves inside them
	UnSaves      = "saves"      // the "own" save folders and save backups in the data folder
	UnGames      = "games"      // game folders downloaded with SteamCMD
	UnSteamCMD   = "steamcmd"   // the SteamCMD RimArcho downloaded
	UnSource     = "source"     // relocated, regenerable source library
	UnData       = "data"       // %APPDATA%\RimArcho and %LOCALAPPDATA%\RimArcho
	UnProgram    = "program"    // the program files
)

// UninstallSteps lists every step in order.
var UninstallSteps = []string{UnAccounts, UnLinks, UnModsConfig, UnTextures, UnLocked, UnMonitor, UnLooks, UnPacks, UnSaves, UnGames, UnSteamCMD, UnSource, UnData, UnProgram}

// What happens to an item.
const (
	DoUnlink  = "unlink"      // the link is deleted, never its target
	DoClean   = "clean"       // ModsConfig.xml keeps the list without the mods that go away
	DoRestore = "restore"     // ModsConfig.xml.rimarcho.bak goes back
	DoTrash   = "trash"       // to the recycle bin; refused when there is none (02 §3)
	DoMove    = "move"        // to Item.To (renamed, or copied, checked and the original trashed)
	DoKeep    = "keep"        // stays where it is
	DoDelete  = "delete"      // RimArcho's own: deleted
	DoLater   = "after-exit"  // deleted once RimArcho has closed
	DoLeave   = "leave"       // left alone; Note says why
	DoAppID   = "remove-file" // RimArcho's steam_appid.txt in a game folder (09 §2)
	DoDDS     = "delete-dds"  // the DDS RimArcho recorded in a mod folder are deleted, nothing else
)

// NotALinkNote is the note of a folder under Mods/ with RimArcho's prefix that is no link.
const NotALinkNote = "not a link: left alone"

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
	// AtOnce: the last two steps run in this process, not after it exits (outside Windows;
	// a test that replaces the clean-up with AfterExitFunc gets the deferred plan). Their
	// items are then done or failed in the report, and Log is not used.
	AtOnce bool

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

// LocalDir resolves the persistent managed root (17 §3), including the lock library.
func (a *App) LocalDir() string {
	if p := a.storeMeta("cfg:local.dir"); p != "" {
		return p
	}
	base := a.LockBase
	if base == "" && runtime.GOOS != "windows" {
		base = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(base) {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		// A sibling of the data directory (17 §3), as %LOCALAPPDATA% is on Windows:
		// managed Mod folders must not lie inside the data directory.
		return filepath.Join(base, "rimarcho-local")
	}
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
// .archo-update is what RimArcho's own update leaves there (17 §5.1). Off Windows the
// tarball's programs are archo and rimarcho (the Linux GUI, 16 §8.1).
var releaseFiles = programFiles(runtime.GOOS)

func programFiles(goos string) []string {
	names := []string{"archo.exe", "RimArcho.exe", "README.md", "README.zh-CN.md", "CHANGELOG.md", "LICENSE", "fonts", "monitor", "skills", UpdateDirName}
	if goos != "windows" {
		names = append([]string{"archo", "rimarcho"}, names...)
	}
	return names
}

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

// uninstallPlanner holds what the steps of planUninstall share while the plan is built.
type uninstallPlanner struct {
	a         *App
	o         UninstallOptions
	p         *UninstallPlan
	envs      []*env.Environment
	sizeOf    func(string) int64
	byName    map[string]*env.Environment // environments with a game folder
	lockEnv   *env.Environment            // where kept lock library copies go (nil: none)
	gone      map[string]map[string]bool  // user data key → package IDs only reached through a removed link
	keptPkgs  map[string]bool             // package IDs of the lock library copies that are kept
	monRoot   string
	looksRoot string   // the looks directory ("" when none was ever chosen)
	keep      []string // folders that hold the player's things and must stay
}

func (a *App) planUninstall(o UninstallOptions, sizeOf func(string) int64) (*UninstallPlan, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	p := &UninstallPlan{Options: o, Home: a.Home, Local: a.LocalDir(), Log: a.UninstallLog, AtOnce: cleanupAtOnce && a.AfterExitFunc == nil}
	if p.Log == "" {
		p.Log = filepath.Join(os.TempDir(), "RimArcho-uninstall.log")
	}
	if p.Game, err = a.RunningGame(); err != nil {
		return nil, err
	}
	p.Others = a.otherArcho()
	pl := &uninstallPlanner{a: a, o: o, p: p, envs: envs, sizeOf: sizeOf, byName: map[string]*env.Environment{}}
	for _, e := range envs {
		if e.GameRoot != "" {
			p.Envs = append(p.Envs, e.Name)
			pl.byName[e.Name] = e
		}
	}
	pl.add(UnAccounts, pl.accountItems())
	pl.pickLockEnv()

	// 1. links, and the package IDs that disappear with them
	links, err := pl.linkItems()
	if err != nil {
		return nil, err
	}
	links = append(links, pl.appIDItems()...)
	pl.add(UnLinks, links)

	// 3. the lock library (planned before ModsConfig.xml: a kept copy stays in its game)
	locked := pl.lockedItems()

	// 2. ModsConfig.xml of each user data folder RimArcho wrote
	mcs, err := pl.modsConfigItems()
	if err != nil {
		return nil, err
	}
	pl.add(UnModsConfig, mcs)
	tex, err := a.uninstallTextures()
	if err != nil {
		return nil, err
	}
	pl.add(UnTextures, tex)
	pl.add(UnLocked, locked)

	pl.monRoot = a.MonitorDir()
	pl.add(UnMonitor, pl.monitorItems())
	pl.looksRoot = a.LooksDir()
	pl.add(UnLooks, pl.looksItems())
	pl.add(UnPacks, pl.packItems())
	pl.add(UnSaves, pl.saveItems())
	pl.add(UnGames, pl.gameItems())
	pl.keep = pl.keptSources()
	pl.add(UnSteamCMD, pl.steamCMDItems())

	sourceRoot := a.SourceDir()
	pl.checkKeptDestinations(sourceRoot)
	data := pl.dataItems()
	pl.noteManagedTools(data)
	pl.addSourceStep(sourceRoot)
	pl.add(UnData, data)
	pl.add(UnProgram, pl.programItems())
	pl.planIncomingLinks()
	return p, nil
}

// add appends a step to the plan when it has something to do.
func (pl *uninstallPlanner) add(key string, items []*UninstallItem) {
	if len(items) > 0 {
		pl.p.Steps = append(pl.p.Steps, &UninstallStep{Key: key, Items: items})
	}
}

// setProblem records why the plan cannot run, keeping the first reason.
func (pl *uninstallPlanner) setProblem(s string) {
	if pl.p.Problem == "" {
		pl.p.Problem = s
	}
}

// accountItems is the accounts step: forget sessions, and the agent key when there is one.
func (pl *uninstallPlanner) accountItems() []*UninstallItem {
	a := pl.a
	accountItems := []*UninstallItem{{Path: a.AccountsDir(), Do: "forget-accounts", Note: "Forget account sessions: delete site sessions and temporary login folders, and clear known authentication caches for all accounts in managed SteamCMD. External caches stay under their owner's control. This cannot be undone and does not sign out other devices."}}
	if a.AgentStatus().HasKey {
		accountItems = append(accountItems, &UninstallItem{Path: "Windows Credential Manager: " + secret.Prefix + AgentKeyName, Do: "forget-agent-key", Note: "Remove the Archo Agent API key from Windows Credential Manager."})
	}
	return accountItems
}

// pickLockEnv chooses where kept lock library copies go: a game that is not itself removed.
func (pl *uninstallPlanner) pickLockEnv() {
	o, p := pl.o, pl.p
	if !o.KeepLocked {
		return
	}
	name := o.LockedTo
	if name == "" {
		if cur, err := pl.a.Env(); err == nil && cur.GameRoot != "" {
			name = cur.Name
		} else if len(p.Envs) > 0 {
			name = p.Envs[0]
		}
	}
	lockEnv := pl.byName[name]
	pl.lockEnv = lockEnv
	if lockEnv != nil {
		p.LockedTo = lockEnv.Name
	}
	switch {
	case lockEnv == nil && o.LockedTo != "":
		p.Problem = fmt.Sprintf("no Game Config named %q with a game folder (archo env list)", o.LockedTo)
	case lockEnv == nil:
		p.Problem = "no game folder to move the lock library's copies into; remove them instead"
	case o.RemoveGames && downloadedGame(lockEnv.GameRoot):
		p.Problem = fmt.Sprintf("the copies would go into %s, which is removed too; pick another game or keep the downloaded games", lockEnv.GameRoot)
	default:
		p.LockedTo = lockEnv.Name
	}
}

// linkItems lists the _Archo_ and !ArchoPack_ links of every known game's Mods/ and
// records in pl.gone the package IDs reached through them.
func (pl *uninstallPlanner) linkItems() ([]*UninstallItem, error) {
	pl.gone = map[string]map[string]bool{}
	var links []*UninstallItem
	seenMods := map[string]bool{}
	observations, err := pl.a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	owned := map[string]*GameLink{}
	for _, l := range observations {
		owned[env.PathKey(l.Path)] = l
	}
	roots, err := pl.a.knownGameMods()
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		e := &env.Environment{GameRoot: root.game}
		for _, candidate := range pl.envs {
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
		links = append(links, pl.modsFolderLinks(mods, store.UserDataKey(e.UserDataRoot), owned)...)
	}
	return links, nil
}

// modsFolderLinks lists the RimArcho links in one Mods/ folder whose game keeps its
// user data under key udKey.
func (pl *uninstallPlanner) modsFolderLinks(mods, udKey string, owned map[string]*GameLink) []*UninstallItem {
	var links []*UninstallItem
	ents, _ := os.ReadDir(mods)
	for _, d := range ents {
		name := d.Name()
		if !managedLink(name) && !packLink(name) {
			continue
		}
		path := filepath.Join(mods, name)
		if !source.IsLink(path) {
			links = append(links, &UninstallItem{Path: path, Do: DoLeave, Note: NotALinkNote})
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
				if pl.gone[udKey] == nil {
					pl.gone[udKey] = map[string]bool{}
				}
				pl.gone[udKey][pkg] = true
			}
		}
		links = append(links, it)
	}
	return links
}

// appIDItems lists the steam_appid.txt that direct launches wrote (02 §1): only RimArcho's,
// and only while it still holds the game's app ID.
func (pl *uninstallPlanner) appIDItems() []*UninstallItem {
	var links []*UninstallItem
	seenGames := map[string]bool{}
	for _, e := range pl.envs {
		f := filepath.Join(e.GameRoot, AppIDFile)
		if e.GameRoot == "" || seenGames[env.PathKey(f)] || pl.a.Store.Meta(appIDMeta(e.GameRoot)) == "" {
			continue
		}
		seenGames[env.PathKey(f)] = true
		if b, err := os.ReadFile(f); err == nil && strings.TrimSpace(string(b)) == env.WorkshopAppID {
			links = append(links, &UninstallItem{Path: f, Do: DoAppID})
		}
	}
	return links
}

// lockedItems lists the lock library's copies, to the recycle bin or moved into the kept
// game's Mods/, and records the kept ones' package IDs in pl.keptPkgs.
func (pl *uninstallPlanner) lockedItems() []*UninstallItem {
	var locked []*UninstallItem
	pl.keptPkgs = map[string]bool{}
	lockRoot := pl.a.LockDir()
	used := map[string]bool{}
	for _, dir := range lockCopies(lockRoot) {
		d := scan.ReadDecl(dir, false)
		it := &UninstallItem{Path: dir, Name: d.Name, Bytes: pl.sizeOf(dir), Do: DoTrash}
		if pl.o.KeepLocked && pl.lockEnv != nil {
			mods := pl.a.modsFolder(pl.lockEnv)
			name := freeNameUsed(mods, filepath.Base(dir), used)
			it.Do, it.To = DoMove, filepath.Join(mods, name)
			if d.PackageID != "" {
				pl.keptPkgs[d.PackageID] = true
			}
		}
		locked = append(locked, it)
	}
	return locked
}

// modsConfigItems lists the ModsConfig.xml of each user data folder RimArcho wrote: restored
// from the backup or cleaned of the mods that go away.
func (pl *uninstallPlanner) modsConfigItems() ([]*UninstallItem, error) {
	var mcs []*UninstallItem
	seenUD := map[string]bool{}
	for _, e := range pl.envs {
		k := store.UserDataKey(e.UserDataRoot)
		if k == "" || seenUD[k] {
			continue
		}
		seenUD[k] = true
		it, err := pl.modsConfigItem(e, k)
		if err != nil {
			return nil, err
		}
		if it != nil {
			mcs = append(mcs, it)
		}
	}
	return mcs, nil
}

// modsConfigItem plans one user data folder's ModsConfig.xml (nil: nothing to do).
func (pl *uninstallPlanner) modsConfigItem(e *env.Environment, k string) (*UninstallItem, error) {
	o := pl.o
	path := env.ModsConfigPath(e.UserDataRoot)
	bak := path + loadout.BackupSuffix
	st, err := pl.a.Store.ModsConfigStateAt(e.UserDataRoot)
	if err != nil {
		return nil, err
	}
	_, bakErr := os.Stat(bak)
	if st.WrittenAt.IsZero() && bakErr != nil {
		return nil, nil // RimArcho never wrote it
	}
	it := &UninstallItem{Path: path, Name: e.Name}
	if o.RestoreBackup && bakErr == nil {
		it.Do, it.Note = DoRestore, "the list from before RimArcho's last save"
		return it, nil
	}
	drop := map[string]bool{MonitorPackage: true, LooksPackage: true}
	for pkg := range pl.gone[k] {
		drop[pkg] = true
	}
	present, err := pl.a.presentPackages(pl.envs, k)
	if err != nil {
		return nil, err
	}
	if pl.lockEnv != nil && store.UserDataKey(pl.lockEnv.UserDataRoot) == k {
		for pkg := range pl.keptPkgs {
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
		return nil, nil // nothing to take out, nothing of RimArcho's left beside it
	}
	it.Do = DoClean
	if o.RestoreBackup {
		it.Note = "no backup to restore: cleaned instead"
	}
	return it, nil
}

// monitorItems lists Archo Monitor's folders (RimArcho's own).
func (pl *uninstallPlanner) monitorItems() []*UninstallItem {
	var mon []*UninstallItem
	if ents, err := os.ReadDir(pl.monRoot); err == nil {
		for _, d := range ents {
			path := filepath.Join(pl.monRoot, d.Name())
			if strings.HasPrefix(d.Name(), monitorFolder) || strings.HasSuffix(d.Name(), TmpSuffix) {
				mon = append(mon, &UninstallItem{Path: path, Do: DoDelete, Bytes: pl.sizeOf(path)})
			}
		}
	}
	return mon
}

// looksItems lists the generated Mod _Archo_Looks and what an interrupted rebuild left
// beside it (RimArcho's own; its textures are hard links and copies, the Mods' files stay).
func (pl *uninstallPlanner) looksItems() []*UninstallItem {
	var out []*UninstallItem
	if pl.looksRoot == "" {
		return nil
	}
	for _, name := range []string{looksFolder, looksFolder + TmpSuffix, looksFolder + looksOldSuffix} {
		path := filepath.Join(pl.looksRoot, name)
		if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
			out = append(out, &UninstallItem{Path: path, Do: DoDelete, Bytes: pl.sizeOf(path)})
		}
	}
	return out
}

// packItems lists the packs: kept where they are, or moved out of the data folder, or to
// the recycle bin.
func (pl *uninstallPlanner) packItems() []*UninstallItem {
	a, o, p := pl.a, pl.o, pl.p
	var packs []*UninstallItem
	packsDir := a.PacksDir()
	inHome := source.Within(packsDir, a.Home) || source.Within(packsDir, p.Local)
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
				packs = append(packs, &UninstallItem{Path: path, Do: DoDelete, Bytes: pl.sizeOf(path), Note: "an unfinished build"})
				continue
			}
			pk, err := a.ReadPack(d.Name())
			if err != nil {
				continue // not a pack: inside the data folder it goes with it, elsewhere it stays
			}
			it := &UninstallItem{Path: path, Name: pk.Name, Bytes: pl.sizeOf(path), Do: DoKeep}
			switch {
			case o.RemovePacks:
				it.Do = DoTrash
			case inHome:
				it.Do, it.To = DoMove, filepath.Join(p.PacksTo, freeNameUsed(p.PacksTo, d.Name(), used))
			}
			packs = append(packs, it)
		}
	}
	return packs
}

// saveItems lists the own save folders (<data>/userdata/<env>) and save backups.
func (pl *uninstallPlanner) saveItems() []*UninstallItem {
	a, o, p := pl.a, pl.o, pl.p
	var saves []*UninstallItem
	p.SavesTo = o.SavesTo
	if p.SavesTo == "" {
		p.SavesTo = a.DefaultSavesTo()
	}
	usedS := map[string]bool{}
	saveItem := func(path, name string) {
		it := &UninstallItem{Path: path, Name: name, Bytes: pl.sizeOf(path), Do: DoTrash}
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
	return saves
}

// gameItems lists the game folders downloaded with SteamCMD.
func (pl *uninstallPlanner) gameItems() []*UninstallItem {
	var games []*UninstallItem
	for _, e := range pl.envs {
		if downloadedGame(e.GameRoot) {
			it := &UninstallItem{Path: e.GameRoot, Name: e.Name, Bytes: pl.sizeOf(e.GameRoot), Do: DoKeep}
			if pl.o.RemoveGames {
				it.Do = DoTrash
			}
			games = append(games, it)
		}
	}
	return games
}

// keptSources lists the roots of the sources other than the lock library, Monitor and looks: they
// never go, since they hold the player's mods.
func (pl *uninstallPlanner) keptSources() []string {
	var keep []string
	if all, err := pl.a.Store.AllSources(); err == nil {
		for _, s := range all {
			if s.Status == source.StatusRetired || s.Root == "" || pl.a.IsLockSource(s) || env.PathKey(s.Root) == env.PathKey(pl.monRoot) || pl.looksRoot != "" && env.PathKey(s.Root) == env.PathKey(pl.looksRoot) {
				continue
			}
			keep = append(keep, s.Root)
		}
	}
	return keep
}

// steamCMDItems plans RimArcho's SteamCMD: deleted, or left alone (and kept) when it is
// custom or holds mods.
func (pl *uninstallPlanner) steamCMDItems() []*UninstallItem {
	a := pl.a
	var scmd []*UninstallItem
	if dir := a.SteamCMDDir(); dirExists(dir) {
		it := &UninstallItem{Path: dir, Do: DoDelete, Bytes: pl.sizeOf(dir)}
		switch {
		case a.Store.Meta("cfg:steamcmd.path") != "" && !source.Within(dir, a.LocalDir()):
			it.Do, it.Note = DoLeave, "SteamCMD is in a custom directory: left alone"
			pl.keep = append(pl.keep, dir)
		case a.steamCMDContentCheck(dir) != nil || holdsAny(dir, pl.keep):
			it.Do, it.Note = DoLeave, "holds mods downloaded with SteamCMD: left alone"
			pl.keep = append(pl.keep, dir)
		}
		scmd = append(scmd, it)
	}
	return scmd
}

// checkKeptDestinations refuses kept and moved things that would land where the data
// folders are deleted.
func (pl *uninstallPlanner) checkKeptDestinations(sourceRoot string) {
	a, p := pl.a, pl.p
	for _, dst := range []struct{ dir, what string }{{p.SavesTo, "saves"}, {p.PacksTo, "packs"}} {
		if source.Within(dst.dir, a.Home) || source.Within(dst.dir, p.Local) || source.Within(directoryReal(dst.dir), directoryReal(sourceRoot)) {
			pl.setProblem(fmt.Sprintf("the %s would be moved to %s, which is deleted too; pick a folder outside RimArcho's folders", dst.what, dst.dir))
		}
	}
}

// dataItems plans the data folders and schedules them for after exit.
func (pl *uninstallPlanner) dataItems() []*UninstallItem {
	a, p := pl.a, pl.p
	var data []*UninstallItem
	if isArchoHome(a.Home) {
		data = append(data, &UninstallItem{Path: a.Home, Do: DoLater, Bytes: pl.sizeOf(a.Home)})
		p.AfterExit.Dirs = append(p.AfterExit.Dirs, a.Home)
	} else {
		data = append(data, &UninstallItem{Path: a.Home, Do: DoLeave, Note: "not a RimArcho data folder (no archo.db): left alone"})
	}
	if dirExists(p.Local) {
		var kept []string
		for _, k := range pl.keep {
			if source.Within(k, p.Local) {
				kept = append(kept, k)
			}
		}
		it := &UninstallItem{Path: p.Local, Do: DoLater, Bytes: pl.sizeOf(p.Local)}
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
	return data
}

// noteManagedTools names on the data items only the managed tools actually covered by the
// scheduled data deletions.
func (pl *uninstallPlanner) noteManagedTools(data []*UninstallItem) {
	for _, name := range []string{deps.Todds} {
		i := pl.a.Deps().Status(name)
		if !i.Own {
			continue
		}
		deleted := false
		for _, dir := range pl.p.AfterExit.Dirs {
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
}

// addSourceStep plans the source library. One inside a deleted data folder goes with it; a
// relocated one is its own step, and a link there blocks the uninstall with the reason on
// the page.
func (pl *uninstallPlanner) addSourceStep(sourceRoot string) {
	sourceCovered := false
	for _, dir := range pl.p.AfterExit.Dirs {
		sourceCovered = sourceCovered || source.Within(sourceRoot, dir)
	}
	if !sourceCovered && dirExists(sourceRoot) {
		if err := pl.a.checkUninstallSource(sourceRoot); err != nil {
			pl.setProblem(err.Error())
		} else {
			pl.add(UnSource, []*UninstallItem{{Path: sourceRoot, Do: DoDelete, Bytes: pl.sizeOf(sourceRoot)}})
		}
	}
}

// programItems plans the program: only RimArcho's own files, deleted after exit.
func (pl *uninstallPlanner) programItems() []*UninstallItem {
	a, p := pl.a, pl.p
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
		names := slices.Clone(releaseFiles)
		for _, n := range selfupdate.Installed(dir) { // entries later releases added (17 §5.1)
			if !slices.ContainsFunc(names, func(m string) bool { return strings.EqualFold(m, n) }) {
				names = append(names, n)
			}
		}
		for _, n := range names {
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
		prog = append(prog, &UninstallItem{Path: dir, Do: DoLater, Bytes: pl.sizeOf(dir), Note: "only RimArcho's own files; the folder goes when nothing else is in it"})
	}
	return prog
}

// planIncomingLinks plans, for every item that is trashed, moved or deleted, the links
// elsewhere that point into it. All targets use one fresh incoming-link observation.
func (pl *uninstallPlanner) planIncomingLinks() {
	var incoming *incomingLinkCheck
	var err error
	for _, step := range pl.p.Steps {
		if step.Key == UnSource {
			continue // Source removal never changes links outside the recorded library.
		}
		for _, it := range step.Items {
			if it.Do == DoTrash || it.Do == DoMove || it.Do == DoDelete {
				if incoming == nil {
					incoming, err = pl.a.readIncomingLinks()
					if err != nil {
						pl.setProblem(err.Error())
						return
					}
				}
				it.Links, err = pl.a.planIncomingLinks(it.Path, "", incoming)
				if err != nil {
					pl.setProblem(err.Error())
				}
			}
		}
	}
}

// modsFolder is an environment's Mods/ folder: its local source's root, else <game>/Mods.
func (a *App) modsFolder(e *env.Environment) string {
	if srcs, err := a.Store.Sources(e.GameID); err == nil {
		for _, s := range srcs {
			if s.Kind == source.Local && s.Status != source.StatusRetired && s.EnvID == e.GameID {
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
		mods, err := a.Store.UninstallMods(e.GameID)
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
// RimArcho has closed is started (Windows), or they are deleted at once and marked done or
// failed (elsewhere: AtOnce).
//
// The operation log (13 §1.3) gets one parent row, with the recycle-bin moves, unlinked links
// and failed items under it. It goes with the data folder when that is deleted.
func (a *App) Uninstall(o UninstallOptions, progress func(UninstallProgress)) (p *UninstallPlan, err error) {
	run := a.opStart(false, func() oplog.Row { return oplog.Row{Op: oplog.Uninstall, Subject: "RimArcho", Target: a.Home} })
	failed := false
	defer func() {
		outcome := err
		if outcome == nil && failed {
			outcome = errPartial
		}
		run.end(outcome, "")
	}()
	p, err = a.CheckUninstall(o)
	if err != nil {
		return nil, err
	}
	if err := p.Blocked(); err != nil {
		return p, err
	}
	run.begin()
	h := run.parent()
	if progress == nil {
		progress = func(UninstallProgress) {}
	}
	n, i := 0, 0
	for _, s := range p.Steps {
		if s.Key != UnData && s.Key != UnProgram {
			n += len(s.Items)
		}
	}
	for _, s := range p.Steps {
		if s.Key == UnData || s.Key == UnProgram {
			continue
		}
		for _, it := range s.Items {
			i++
			progress(UninstallProgress{Step: s.Key, Item: it.Path, I: i, N: n})
			if err := a.uninstallItem(h, s.Key, it); err != nil {
				it.Err = err.Error()
				failed = true
				if it.Do != DoTrash && it.Do != DoUnlink { // those write their own rows
					a.opAdd(h, oplog.Row{Op: oplog.Uninstall, Subject: s.Key + ": " + it.Do, Target: it.Path, Result: oplog.Failed, Detail: err.Error()})
				}
				if it.Do == "forget-agent-key" {
					return p, errors.New("could not remove the Archo Agent API key from Windows Credential Manager; tools and data were kept; retry")
				}
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
		case UnLooks:
			if dir := a.LooksDir(); dir != "" {
				removeIfEmpty(dir)
				if up := filepath.Dir(dir); filepath.Base(up) == "RimArcho" { // <volume root>/RimArcho/looks
					removeIfEmpty(up)
				}
			}
		case UnSteamCMD:
			// the junctions a non-English profile path needed to start SteamCMD (17 §2.1)
			removeSteamCMDAliases(a.LocalDir())
		case UnPacks:
			if o.RemovePacks || source.Within(a.PacksDir(), a.Home) || source.Within(a.PacksDir(), a.LocalDir()) {
				removeIfEmpty(a.PacksDir())
			}
		}
	}
	if failed {
		p.eachLater(func(it *UninstallItem) {
			it.Do, it.Note = DoLeave, "kept because a step above failed: fix it, then uninstall again"
		})
		return p, nil
	}
	progress(UninstallProgress{Step: UnData, I: n, N: n})
	a.Uninstalled = true
	a.cleanUp(p)
	return p, nil
}

// eachLater calls f with every item of the last two steps that is deleted after the others.
func (p *UninstallPlan) eachLater(f func(*UninstallItem)) {
	for _, key := range []string{UnData, UnProgram} {
		if s := p.Step(key); s != nil {
			for _, it := range s.Items {
				if it.Do == DoLater {
					f(it)
				}
			}
		}
	}
}

// cleanUp deletes the data folders and the program: the script after exit on Windows,
// at once elsewhere, where each item is then done or names what was left.
func (a *App) cleanUp(p *UninstallPlan) {
	var left []string
	var err error
	if a.AfterExitFunc != nil {
		err = a.AfterExitFunc(p.AfterExit)
	} else {
		left, err = startAfterExit(p.AfterExit, p.Log)
	}
	p.eachLater(func(it *UninstallItem) {
		switch {
		case err != nil:
			it.Err = "the clean-up could not start: " + err.Error() + "; delete it yourself"
		case p.AtOnce:
			it.Done = true
			for _, l := range left {
				if source.Within(l, it.Path) {
					it.Done, it.Err = false, "could not delete "+l+"; delete it yourself"
					break
				}
			}
		}
	})
}

// uninstallItem carries out one item.
func (a *App) uninstallItem(h *oplog.Handle, step string, it *UninstallItem) error {
	switch it.Do {
	case "forget-accounts":
		return a.ForgetAccounts()
	case "forget-agent-key":
		return SetAgentKey("")
	case DoKeep, DoLeave:
		return nil
	case DoAppID:
		if err := os.Remove(it.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case DoUnlink:
		return a.applyLinksIn(h, filepath.Dir(it.Path), []LinkChange{{Op: "remove", Name: filepath.Base(it.Path)}})
	case DoClean:
		return cleanModsConfig(it.Path, it.Drop)
	case DoRestore:
		return restoreModsConfig(it.Path)
	case DoTrash:
		return a.toTrash(h, it.Path)
	case DoDelete:
		if step == UnSource {
			return a.uninstallSource(it.Path)
		}
		return a.withIncomingLinks(it.Path, "", func() error { return a.deleteTree(h, it.Path) })
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
	same := a.SameVolume
	if same == nil {
		same = source.SameVolume
	}
	if same(from, to) {
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
	var n int
	return removeTreeCounted(dir, &n)
}

// removeTreeCounted is removeTree that counts the entries it removed, so a caller can tell
// a folder left whole from one left in part.
func removeTreeCounted(dir string, removed *int) error {
	if source.IsLink(dir) {
		if err := source.RemoveLink(dir); err != nil {
			return err
		}
		*removed++
		return nil
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
			if err = source.RemoveLink(p); err == nil {
				*removed++
			}
		case d.IsDir():
			err = removeTreeCounted(p, removed)
		default:
			if err = os.Remove(p); err == nil {
				*removed++
			}
		}
		if err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	if err := os.Remove(dir); err != nil {
		return err
	}
	*removed++
	return nil
}

// partlyRemovedError is a deletion that failed after removing part of the folder. The link
// bracket then leaves the game links removed instead of linking a game to what is left
// (abortLinkLifecycle).
type partlyRemovedError struct {
	path string
	err  error
}

func (e *partlyRemovedError) Error() string {
	return fmt.Sprintf("%s: only part of it was deleted (%s); the game links to it were removed, delete the rest yourself", e.path, e.err)
}

func (e *partlyRemovedError) Unwrap() error { return e.err }

// removeModTree deletes a Mod folder inside the link bracket: a failure after anything was
// removed is a partlyRemovedError.
func removeModTree(path string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		return os.Remove(path)
	}
	var n int
	err := removeTreeCounted(path, &n)
	if err != nil && n > 0 {
		return &partlyRemovedError{path: path, err: err}
	}
	return err
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

// storeMeta reads a meta value, or "" before the database is open.
func (a *App) storeMeta(key string) string {
	if a.Store == nil {
		return ""
	}
	return a.Store.Meta(key)
}
