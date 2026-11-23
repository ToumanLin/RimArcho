package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/updatechannel"
)

// Anonymous Monitor fetch (19 §6): a player who does not subscribe to the Archo Monitor
// Workshop item still gets its weekly corpus data. SteamCMD downloads the item with
// `+login anonymous` (never the signed-in account: that would end the player's Steam
// session) into a scratch folder, and the folder is installed into RimArcho's own Monitor
// directory under the usual rules: a new modVersion is a new version folder, the same
// version takes newer corpus data in place.

// monitorFetchKey stores the item's time_updated of the last installed download.
const monitorFetchKey = "monitor:anonymous.updated"

// MonitorFetch is what one anonymous check did.
type MonitorFetch struct {
	Item       string `json:"item,omitempty"`
	Updated    int64  `json:"updated,omitempty"` // the item's time_updated
	Skipped    string `json:"skipped,omitempty"` // why nothing was downloaded
	Downloaded bool   `json:"downloaded,omitempty"`
	Version    string `json:"version,omitempty"` // the downloaded Monitor's modVersion
	Copied     bool   `json:"copied,omitempty"`  // a new version folder
	Corpus     string `json:"corpus,omitempty"`  // corpus data refreshed in place
}

// FetchMonitorAnonymous runs the anonymous check. It does nothing unless automatic checks are
// on (16 §2.16), no game runs, no Workshop-subscribed Monitor is present, SteamCMD is
// installed and the item changed since the last download.
func (a *App) FetchMonitorAnonymous(ctx context.Context) (*MonitorFetch, error) {
	f := &MonitorFetch{}
	opts := updatechannel.Options{HTTP: a.HTTP, SteamAPI: a.SteamAPI}
	f.Item = updatechannel.MonitorItem(opts)
	switch {
	case !a.AutomaticEnabled():
		f.Skipped = "automatic checks are off"
	case f.Item == "":
		f.Skipped = "the Monitor Workshop item is not assigned"
	}
	if f.Skipped != "" {
		return f, nil
	}
	if g, _ := a.RunningGame(); g != nil {
		f.Skipped = "a game is running"
		return f, nil
	}
	e, err := a.Env()
	if err != nil && !errors.Is(err, ErrNoEnv) {
		return f, err
	}
	if e != nil && len(a.workshopMonitorPaths(e)) > 0 {
		f.Skipped = "a Workshop-subscribed Archo Monitor is present"
		return f, nil
	}
	exe := a.SteamCMDPath()
	if exe == "" {
		f.Skipped = "SteamCMD is not installed"
		return f, nil
	}
	if err := a.managedRootReady("monitor", a.MonitorDir()); err != nil {
		f.Skipped = err.Error()
		return f, nil
	}
	f.Updated, err = updatechannel.MonitorUpdated(ctx, opts)
	if err != nil {
		return f, err
	}
	if last, _ := strconv.ParseInt(a.Store.Meta(monitorFetchKey), 10, 64); f.Updated <= last {
		f.Skipped = "up to date"
		return f, nil
	}
	if ctx.Err() != nil {
		return f, ctx.Err()
	}
	base := filepath.Join(a.LocalDir(), "monitor-download")
	os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return f, err
	}
	defer os.RemoveAll(base) // RimArcho's own scratch folder
	unlock := accounts.Lock(exe)
	out, runErr := a.steamCMD().run(exe, []string{"+force_install_dir", base, "+login", "anonymous",
		"+workshop_download_item", env.WorkshopAppID, f.Item, "+quit"})
	unlock()
	folder := filepath.Join(base, "steamapps", "workshop", "content", env.WorkshopAppID, f.Item)
	v, err := monitorVersion(folder)
	if err != nil {
		if runErr != nil {
			return f, fmt.Errorf("SteamCMD anonymous download of %s: %w", f.Item, runErr)
		}
		return f, fmt.Errorf("SteamCMD anonymous download of %s gave no Archo Monitor folder: %v (%s)", f.Item, err, lastLine(out))
	}
	f.Downloaded, f.Version = true, v
	if g, _ := a.RunningGame(); g != nil {
		f.Skipped = "a game started during the download; installed next time"
		return f, nil
	}
	r := &MonitorResult{Bundled: &MonitorBundled{Dir: folder, Version: v}}
	read, err := a.prepareMonitorFrom(r, r.Bundled, false)
	if err != nil {
		return f, err
	}
	f.Copied, f.Corpus = r.Copied || r.Replaced, r.Corpus
	if read != "" {
		if _, err := a.ScanWith(ScanOptions{Sources: []string{read}}); err != nil {
			return f, err
		}
	}
	if !r.stop {
		if err := a.finishMonitor(r); err != nil {
			return f, err
		}
	}
	a.forgetCorpus()
	return f, a.Store.SetMeta(monitorFetchKey, strconv.FormatInt(f.Updated, 10))
}

func lastLine(s string) string {
	for i := len(s) - 1; i > 0; i-- {
		if s[i] == '\n' && i < len(s)-1 {
			return s[i+1:]
		}
	}
	return s
}
