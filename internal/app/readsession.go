package app

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// ReadSession gives a slow page its own database connection and application caches.
// Call while the original App is locked; the returned App is used by one reader at a
// time and must be closed. Sessions share only read-only data: an index's contents
// (ReadIndex), list registries and game links (shared.go); Loadout and analysis caches
// stay per session.
func (a *App) ReadSession() (*App, error) {
	s, err := store.OpenSession(a.Store.Path)
	if err != nil {
		return nil, err
	}
	b := &App{Store: s, decl: a.decl}
	b.ReadOptions(a)
	s.DB.Exec("PRAGMA cache_size=-256")
	return b, nil
}

// WorkspaceSnapshot makes page reads use persisted lists without importing game
// files or taking the launch/edit lock. Commands still use normal ReadSession.
func (a *App) WorkspaceSnapshot() { a.workspaceSnapshot = true }

// ReadOptions refreshes the invocation settings without discarding a reader's caches.
// Neither App may be in use while its options are copied.
func (b *App) ReadOptions(a *App) {
	b.MemoryProbe = a.MemoryProbe
	b.Home, b.Config, b.Now, b.Rand, b.Loc = a.Home, a.Config, a.Now, a.Rand, a.Loc
	b.Detect, b.FreeSpace, b.Decompile = a.Detect, a.FreeSpace, a.Decompile
	b.EnvName, b.GameVersion = a.EnvName, a.GameVersion
	b.OpenPlan, b.PlanName = a.OpenPlan, a.PlanName
	b.AnalyzeRunner, b.AnalyzeExe, b.AnalyzeWait = a.AnalyzeRunner, a.AnalyzeExe, a.AnalyzeWait
	b.LazyLog, b.LogCacheOnly = a.LazyLog, a.LogCacheOnly
	b.GameProbe, b.LockBase, b.MonitorBundle = a.GameProbe, a.LockBase, a.MonitorBundle
	b.HTTP, b.SteamAPI = a.HTTP, a.SteamAPI
	b.FindTool, b.SteamWorker, b.TrashFunc = a.FindTool, a.SteamWorker, a.TrashFunc
	b.SteamWorkerStream = a.SteamWorkerStream
	b.LaunchWait = a.LaunchWait
	b.pageRepo = a.pageRepo
	b.SteamCMDZip, b.SteamCMDNoPath = a.SteamCMDZip, a.SteamCMDNoPath
	b.SteamCMDStart, b.SteamCMDRun, b.SteamCMDPoll = a.SteamCMDStart, a.SteamCMDRun, a.SteamCMDPoll
	b.SteamCMDWait = a.SteamCMDWait
	b.TextureEncoder, b.ToddsNoPath, b.ToddsZip = a.TextureEncoder, a.ToddsNoPath, a.ToddsZip
	b.UninstallExe, b.UninstallKeepBase, b.ArchoProbe = a.UninstallExe, a.UninstallKeepBase, a.ArchoProbe
	b.UninstallLog, b.AfterExitFunc = a.UninstallLog, a.AfterExitFunc
	b.SelfUpdateAPI, b.SelfVersion, b.SelfProgram = a.SelfUpdateAPI, a.SelfVersion, a.SelfProgram
	b.SelfVersionOf, b.SelfOthers, b.SelfWaitPoll = a.SelfVersionOf, a.SelfOthers, a.SelfWaitPoll
}

// AdoptReadState transfers a finished worker's index/workspace to the GUI session,
// avoiding a second large load while holding its page mutex (06 §8.4).
// Both sessions must use the same database and environment and be idle.
func (a *App) AdoptReadState(r *App) {
	rekey := func(stamp, key string) string {
		if i := strings.IndexByte(key, '|'); i >= 0 {
			return stamp + key[i:]
		}
		return ""
	}
	a.ixCache, a.ixKey = r.ixCache, rekey(a.Store.IndexKey(), r.ixKey)
	a.stCache, a.stKey = r.stCache, rekey(a.Store.ChangeKey(), r.stKey)
	if r.workspaceSnapshot {
		a.stCache, a.stKey = nil, ""
	}
	a.prCache, a.prKey = nil, ""
	if a.ixCache != nil {
		if d := a.declStore(); d != nil {
			d.BindMods(a.ixCache.Mods)
		} else {
			a.Store.BindMods(a.ixCache.Mods)
		}
		a.ixCache.store = a.Store
	}
	a.Store.MM = r.Store.MM
	// the worker may have chosen a list (HomeApply): its state is for that list
	a.OpenPlan, a.PlanName = r.OpenPlan, r.PlanName
}

// AdoptReadSnapshot keeps the reader usable by subsequent page requests. Only a
// detached index crosses sessions; a writer retains its own guarded loadout cache.
func (a *App) AdoptReadSnapshot(r *App) {
	a.ReadIndex(r)
	a.OpenPlan, a.PlanName = r.OpenPlan, r.PlanName
}

// ReadIndex gives an idle reader the warm index of a. Call with a locked. The reader gets
// its own Index (environment, session for foreign-list lookups) over the same Mods, maps,
// lineages and sources: they are not changed once built, and their lazy declarations are
// read through the process's long-lived declaration connection, so a source session that
// closes later does not leave them reading a closed connection (07 §8.3). Without that
// connection the reader loads its own index when it needs one. List edits on a's
// connection do not invalidate the shared index.
func (b *App) ReadIndex(a *App) {
	if !a.IndexCached() {
		return
	}
	if b.ixCache != nil && b.ixKey == a.ixKey {
		return
	}
	d := a.declStore()
	if d == nil {
		return
	}
	d.BindMods(a.ixCache.Mods)
	x := *a.ixCache
	x.store = b.Store
	e := *x.Env
	x.Env = &e
	b.ixCache, b.stCache = &x, nil
	// Keep the generation we actually copied. A concurrent writer may have
	// committed since then; stamping the copy with its newer key would hide it.
	b.ixKey = a.ixKey
}
