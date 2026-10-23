package app

import "github.com/ToumanLin/RimArcho/internal/store"

// ReadSession gives a slow page its own database connection and application caches.
// Call while the original App is locked; the returned App is used by one reader at a
// time and must be closed. No Index, Loadout or analysis cache is shared between them.
func (a *App) ReadSession() (*App, error) {
	s, err := store.Open(a.Store.Path)
	if err != nil {
		return nil, err
	}
	b := &App{Store: s}
	b.ReadOptions(a)
	s.DB.Exec("PRAGMA cache_size=-256")
	return b, nil
}

// ReadOptions refreshes the invocation settings without discarding a reader's caches.
// Neither App may be in use while its options are copied.
func (b *App) ReadOptions(a *App) {
	b.Home, b.Config, b.Now, b.Rand, b.Loc = a.Home, a.Config, a.Now, a.Rand, a.Loc
	b.Detect, b.FreeSpace, b.Decompile = a.Detect, a.FreeSpace, a.Decompile
	b.EnvName, b.GameVersion = a.EnvName, a.GameVersion
	b.AnalyzeRunner, b.AnalyzeExe, b.AnalyzeWait = a.AnalyzeRunner, a.AnalyzeExe, a.AnalyzeWait
	b.LazyLog, b.LogCacheOnly = a.LazyLog, a.LogCacheOnly
	b.GameProbe, b.LockBase, b.MonitorBundle = a.GameProbe, a.LockBase, a.MonitorBundle
	b.HTTP, b.SteamAPI = a.HTTP, a.SteamAPI
	b.TextureEncoder, b.ToddsNoPath, b.ToddsZip = a.TextureEncoder, a.ToddsNoPath, a.ToddsZip
	b.UninstallExe, b.UninstallKeepBase, b.ArchoProbe = a.UninstallExe, a.UninstallKeepBase, a.ArchoProbe
	b.UninstallLog = a.UninstallLog
}
