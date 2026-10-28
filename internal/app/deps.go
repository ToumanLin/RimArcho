package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/host"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Deps returns this application's dependency finder. Its probe cache survives page
// refreshes; settings and invocation overrides are read again on each call.
func (a *App) Deps() *deps.Finder {
	fake := a.TextureEncoder != nil
	if a.depFinder == nil || a.depFake != fake {
		a.depFinder = &deps.Finder{}
		a.depFake = fake
	}
	f := a.depFinder
	f.Home, f.LocalDir = a.Home, a.LocalDir()
	exe, _ := os.Executable()
	f.ProgramDir = filepath.Dir(exe)
	f.Setting = func(key string) string { return a.Store.Meta("cfg:" + key) }
	f.LookPath = func(name string) (string, error) {
		if name == "todds" && a.ToddsNoPath || name == "steamcmd" && a.SteamCMDNoPath {
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	}
	f.ProgramFiles = nil
	for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
		if dir := os.Getenv(key); dir != "" {
			if key == "LOCALAPPDATA" {
				dir = filepath.Join(dir, "Programs")
			}
			f.ProgramFiles = append(f.ProgramFiles, dir)
		}
	}
	if fake {
		f.Version = func(name, path string) (string, error) {
			if name == deps.Todds {
				return textures.ToddsVersion, nil
			}
			return "", nil
		}
	}
	return f
}

// InstallDep downloads a dependency into RimArcho's managed directory. Selection
// settings are preserved, so a configured external copy remains selected.
func (a *App) InstallDep(name string, progress func(done, total int64)) (path string, installErr error) {
	defer func() {
		if installErr == nil {
			a.recordToolInstalled(name)
		}
	}()
	if name == deps.WezTerm {
		unlock := accounts.Lock(a.Deps().Status(name).Dir)
		defer unlock()
		if err := toolIdle(name); err != nil {
			return "", err
		}
	}
	switch name {
	case deps.Todds:
		return a.InstallToddsProgress(progress)
	case deps.SteamCMD:
		if info := a.Deps().Status(name); info.Own {
			unlock := accounts.Lock(info.Path)
			defer unlock()
			if err := toolIdle(name); err != nil {
				return "", err
			}
			p, err := a.steamCMD().start(info.Path, []string{"+quit"}, false)
			if err != nil {
				return "", err
			}
			return info.Path, p.Wait()
		}
		return a.installSteamCMDProgress(a.Deps().Status(name).Dir, progress)
	case deps.WezTerm:
		api := a.WezTermAPI
		if api == "" {
			api = host.GitHubAPI
		}
		h := &host.Host{Home: a.Home, API: api, HTTP: a.HTTP, Out: io.Discard}
		return h.DownloadProgress(progress)
	case deps.Font:
		return "", fmt.Errorf("the font ships with RimArcho; reinstall the release zip")
	default:
		return "", fmt.Errorf("unknown dependency %q", name)
	}
}

// RemoveDep removes only the selected managed copy, never a configured external,
// PATH or Program Files copy. It leaves configuration and other copies unchanged.
func (a *App) RemoveDep(name string) error {
	i := a.Deps().Status(name)
	key := i.Dir
	if name == deps.SteamCMD {
		key = i.Path
	}
	unlock := accounts.Lock(key)
	defer unlock()
	if err := a.CheckDepRemoval(name); err != nil {
		return err
	}
	return os.RemoveAll(a.Deps().Status(name).Dir)
}

// CheckDepRemoval checks ownership and protected content without changing files.
// Settings use the same check as RemoveDep, which repeats it when the action runs.
func (a *App) CheckDepRemoval(name string) error {
	if err := toolIdle(name); err != nil {
		return err
	}
	i := a.Deps().Status(name)
	if !i.Installable || !i.Own {
		return fmt.Errorf("%s is not RimArcho's own copy; nothing removed", name)
	}
	// Recheck with filesystem metadata immediately before removal. This also rejects
	// a managed directory redirected to an external installation by a junction.
	parent, err := filepath.EvalSymlinks(filepath.Dir(i.Dir))
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(i.Dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Join(parent, filepath.Base(i.Dir)), root)
	if err != nil || rel != "." {
		return fmt.Errorf("%s is outside RimArcho's download directory; nothing removed", name)
	}
	if name == deps.SteamCMD {
		return a.steamCMDContentCheck(i.Dir)
	}
	return nil
}

// steamCMDContentCheck protects content stored beside the SteamCMD executable.
// It is also used by uninstall before scheduling deletion of the tool directory.
func (a *App) steamCMDContentCheck(dir string) error {
	ws := filepath.Join(dir, "steamapps", "workshop", "content", env.WorkshopAppID)
	entries, err := os.ReadDir(ws)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot check SteamCMD workshop content; nothing removed: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("SteamCMD holds downloaded Workshop content; nothing removed")
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return fmt.Errorf("cannot check SteamCMD game folders; nothing removed: %w", err)
	}
	var protected []string
	for _, e := range envs {
		protected = append(protected, e.GameRoot)
	}
	sources, err := a.Store.AllSources()
	if err != nil {
		return fmt.Errorf("cannot check SteamCMD mod folders; nothing removed: %w", err)
	}
	for _, s := range sources {
		protected = append(protected, s.Root)
	}
	protected = append(protected, a.LockDir())
	for _, path := range protected {
		if path != "" && (source.Within(path, dir) || source.Within(realOrAbs(path), realOrAbs(dir))) {
			return fmt.Errorf("SteamCMD contains a registered game folder, mod folder or lock library; nothing removed")
		}
	}
	return nil
}

// OfferDep runs only when the player requests installation, before confirmation.
func (a *App) OfferDep(name string) (*SteamCMDOffer, error) {
	dir := a.Deps().Status(name).Dir
	switch name {
	case deps.SteamCMD:
		offer, err := a.OfferSteamCMD()
		if offer != nil {
			offer.Dir = dir
		}
		return offer, err
	case deps.Todds:
		offer := a.OfferTodds()
		return &SteamCMDOffer{URL: offer.URL, Dir: dir, Bytes: offer.Bytes}, nil
	case deps.WezTerm:
		api := a.WezTermAPI
		if api == "" {
			api = host.GitHubAPI
		}
		h := &host.Host{Home: a.Home, API: api, HTTP: a.HTTP, Out: io.Discard}
		offer, err := h.Offer()
		if err != nil {
			return nil, err
		}
		return &SteamCMDOffer{URL: offer.URL, Dir: dir, Bytes: offer.Bytes}, nil
	default:
		return nil, fmt.Errorf("unknown installable dependency %q", name)
	}
}
