package app

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ReadSession gives a slow page its own database connection and application caches.
// Call while the original App is locked; the returned App is used by one reader at a
// time and must be closed. No Index, Loadout or analysis cache is shared between them.
func (a *App) ReadSession() (*App, error) {
	s, err := store.OpenSession(a.Store.Path)
	if err != nil {
		return nil, err
	}
	b := &App{Store: s}
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
	b.TextureEncoder, b.ToddsNoPath, b.ToddsZip = a.TextureEncoder, a.ToddsNoPath, a.ToddsZip
	b.WezTermAPI = a.WezTermAPI
	b.UninstallExe, b.UninstallKeepBase, b.ArchoProbe = a.UninstallExe, a.UninstallKeepBase, a.ArchoProbe
	b.UninstallLog = a.UninstallLog
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
		a.Store.BindMods(a.ixCache.Mods)
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

// ReadIndex copies a warm index into an idle reader. Call with the source App locked.
// Only the immutable shorteners are shared; lazy Mod declarations use the reader's DB.
// List edits on the source connection do not invalidate this detached index.
func (b *App) ReadIndex(a *App) {
	if !a.IndexCached() {
		return
	}
	if b.ixCache != nil && b.ixKey == a.ixKey {
		return
	}
	if b.ixKey != a.ixKey || b.ixCache == nil {
		x := *a.ixCache
		x.store = b.Store
		e := *x.Env
		x.Env = &e
		x.Mods = make([]*store.Mod, 0, len(x.Mods))
		x.ByID = make(map[string]*store.Mod, len(a.ixCache.ByID))
		x.ByLineage = make(map[string][]*store.Mod, len(a.ixCache.ByLineage))
		for _, m := range a.ixCache.Mods {
			c := b.Store.ReadCopy(m)
			x.Mods = append(x.Mods, c)
			x.ByID[c.ID] = c
			x.ByLineage[c.LineageID] = append(x.ByLineage[c.LineageID], c)
		}
		x.own = make(map[string]*store.Mod, len(a.ixCache.own))
		for pkg, m := range a.ixCache.own {
			x.own[pkg] = x.ByID[m.ID]
		}
		x.Lineages = make(map[string]*store.Lineage, len(a.ixCache.Lineages))
		for id, l := range a.ixCache.Lineages {
			c := *l
			x.Lineages[id] = &c
		}
		x.Sources = make([]*source.Source, 0, len(a.ixCache.Sources))
		for _, src := range a.ixCache.Sources {
			c := *src
			x.Sources = append(x.Sources, &c)
		}
		b.ixCache, b.stCache = &x, nil
	}
	// Keep the generation we actually copied. A concurrent writer may have
	// committed since then; stamping the copy with its newer key would hide it.
	b.ixKey = a.ixKey
}
