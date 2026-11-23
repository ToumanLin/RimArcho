package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Cross-environment local links belong to the installed library, not one plan. Saving
// a plan keeps them, including when disabled, just as it keeps ordinary local folders.
const envLinkPrefix = DeployPrefix + "env_"

func envLink(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(envLinkPrefix))
}

// prepareEnvLinks makes mods named by the destination's config/workspace available
// before its first loadout import. It never saves a list or changes another game's links.
// scan false leaves the scan after new links, or of a game never scanned, to the caller.
func (a *App) prepareEnvLinks(e *env.Environment, scan bool) error {
	ix, err := a.loadIndex(e)
	if err != nil {
		return err
	}
	tokens, other, selected, err := a.envLinkSelection(e, ix)
	if err != nil {
		return err
	}
	if len(tokens) == 0 && len(selected) == 0 && len(other) == 0 {
		return nil
	}
	reg, byID, aliases, err := a.envLinkRegistry(e, ix)
	if err != nil {
		return err
	}
	modsDir := modsDir(ix)
	if modsDir == "" {
		return nil
	}
	if err := a.checkRoots(modsDir); err != nil {
		return err
	}
	for _, token := range tokens {
		if m := byID[reg.Resolve(token).Instance]; m != nil {
			selected = append(selected, m)
		}
	}
	for _, id := range other {
		if m := byID[id]; m != nil {
			selected = append(selected, m)
		}
	}
	want, err := a.wantedEnvLinks(ix, aliases, modsDir, selected)
	if err != nil {
		return err
	}
	if len(want) == 0 {
		if sc, _, err := a.LastScan(e.GameID); err != nil {
			return err
		} else if sc != nil {
			return nil
		}
	}
	if g := a.probe()(e.GameRoot); len(want) > 0 && g != nil {
		return fmt.Errorf("env:%s's game is running (pid %d); close it before adding Mods/ links", e.Name, g.PID)
	}
	if len(want) > 0 {
		if err := os.MkdirAll(modsDir, 0o755); err != nil {
			return err
		}
	}
	return a.addEnvLinks(e, modsDir, want, scan)
}

// envLinkSelection reads what the Game Config's ModsConfig.xml and current list select: the
// tokens to resolve, the instances of other games' folders (lists are shared) and the
// instances of this game.
func (a *App) envLinkSelection(e *env.Environment, ix *Index) (tokens, other []string, selected []*store.Mod, err error) {
	if e.UserDataRoot != "" {
		cfg, err := loadout.ReadModsConfig(env.ModsConfigPath(e.UserDataRoot))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil, err
		}
		if cfg != nil {
			tokens = append(tokens, cfg.Active...)
		}
	}
	p, err := a.planFor(e.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if p == nil {
		return tokens, other, selected, nil
	}
	ws, err := a.Store.Workspace(p.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if ws != nil {
		for _, entry := range loadout.Decode(ws.Entries) {
			if entry.Missing != "" {
				tokens = append(tokens, entry.Missing)
			} else if m := ix.ByID[entry.Instance]; m != nil {
				selected = append(selected, m)
			} else {
				// Lists are shared: this may be the previous game's own Mods/ folder.
				other = append(other, entry.Instance)
			}
		}
	}
	return tokens, other, selected, nil
}

// envLinkRegistry builds the registry the selection resolves against: this game's
// candidates, the previous Game Config's, and a fallback for shared mods with no deployed
// version yet. byID holds the instances of both indexes.
func (a *App) envLinkRegistry(e *env.Environment, ix *Index) (*loadout.Registry, map[string]*store.Mod, map[string][]store.Alias, error) {
	// Shared extra/SteamCMD sources (including Monitor) need no previous environment.
	_, aliases, cands, _, err := a.registry(ix)
	if err != nil {
		return nil, nil, nil, err
	}
	mods := append([]*store.Mod(nil), ix.Mods...)
	if prev, err := a.Store.CurrentEnv(); err != nil {
		return nil, nil, nil, err
	} else if prev != nil && prev.ID != e.ID {
		old, err := a.loadIndex(prev)
		if err != nil {
			return nil, nil, nil, err
		}
		_, _, oldCands, _, err := a.registry(old)
		if err != nil {
			return nil, nil, nil, err
		}
		mods = append(mods, old.Mods...)
		// Previous local candidates must precede Workshop copies of the same package.
		cands = append(cands, oldCands...)
	}
	// Shared candidates occur in both indexes. Keep each instance once so Workshop
	// suffixes and the selected deployed version retain their normal registration.
	seen, packages := map[string]bool{}, map[string]bool{}
	unique := make([]loadout.Candidate, 0, len(cands))
	for _, c := range cands {
		if !seen[c.ID] {
			unique = append(unique, c)
			seen[c.ID], packages[c.PackageID] = true, true
		}
	}
	unique = append(unique, envLinkFallback(ix, aliases, packages)...)
	reg := loadout.NewRegistry(unique)
	byID := map[string]*store.Mod{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	return reg, byID, aliases, nil
}

// envLinkFallback is the candidates for packages no candidate has. On first use there may
// be no deployed version yet: an unambiguous shared mod can still satisfy the config; for
// Monitor its newest installed version.
func envLinkFallback(ix *Index, aliases map[string][]store.Alias, packages map[string]bool) []loadout.Candidate {
	fallback := map[string][]*store.Mod{}
	for _, m := range ix.Mods {
		if !packages[m.PackageID] && deployable(ix, aliases, m) {
			fallback[m.PackageID] = append(fallback[m.PackageID], m)
		}
	}
	if ms := monitorInstances(ix); len(ms) > 0 && !packages[MonitorPackage] && deployable(ix, aliases, ms[0].Mod) {
		fallback[MonitorPackage] = []*store.Mod{ms[0].Mod}
	}
	var out []loadout.Candidate
	for _, ms := range fallback {
		if len(ms) == 1 {
			out = append(out, deployCandidate(ms[0]))
		}
	}
	return out
}

// wantedEnvLinks is the links modsDir needs for the selected mods, by name. A package
// already in the folder (read on disk too: a freshly added game may not be scanned yet)
// needs none; a different folder under a link's name is refused.
func (a *App) wantedEnvLinks(ix *Index, aliases map[string][]store.Alias, modsDir string, selected []*store.Mod) (map[string]LinkChange, error) {
	present := map[string]bool{}
	for _, c := range a.ModsCopies(ix) {
		present[c.PackageID] = true
	}
	want := map[string]LinkChange{}
	for _, m := range selected {
		if m == nil || m.Status != identity.Live || m.Residue || m.Unloadable != "" || present[m.PackageID] {
			continue
		}
		name := envLinkName(ix, aliases, m)
		if name == "" {
			continue
		}
		if !source.HasAbout(m.RealPath) {
			continue // stale inventory is not an installed mod
		}
		p := filepath.Join(modsDir, name)
		if _, err := os.Lstat(p); err == nil {
			if sameDir(p, m.RealPath) {
				continue
			}
			return nil, fmt.Errorf("%s already exists; keep it or remove it before switching Game Configs", p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		want[name] = LinkChange{Op: "add", Name: name, Target: m.RealPath, Mod: m.ID}
		present[m.PackageID] = true
	}
	return want, nil
}

// envLinkName is the name of m's link in a game's Mods/ ("" none needed).
func envLinkName(ix *Index, aliases map[string][]store.Alias, m *store.Mod) string {
	switch {
	case m.Kind == source.Local:
		if s := ix.Source(m.SourceID); s != nil && s.Kind == source.Local {
			if !envLink(m.Rel) {
				return "" // ordinary local mods belong to the destination
			}
			return m.Rel // repair a removed cross-environment link
		}
		return envLinkPrefix + m.ID
	case deployable(ix, aliases, m):
		return DeployName(m)
	}
	return ""
}

// addEnvLinks adds the wanted links in name order and, when scan is set, scans the game's
// own folders; an error removes the links added again.
func (a *App) addEnvLinks(e *env.Environment, modsDir string, want map[string]LinkChange, scan bool) error {
	var links []LinkChange
	for _, c := range want {
		links = append(links, c)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Name < links[j].Name })
	var added []LinkChange
	rollback := func(cause error) error {
		for i := len(added) - 1; i >= 0; i-- {
			p := filepath.Join(modsDir, added[i].Name)
			if err := a.applyLinks(modsDir, []LinkChange{{Op: "remove", Name: filepath.Base(p)}}); err != nil {
				cause = errors.Join(cause, err)
			}
		}
		return cause
	}
	for _, c := range links {
		if err := a.applyLinks(modsDir, []LinkChange{c}); err != nil {
			return rollback(err)
		}
		added = append(added, c)
	}
	if !scan {
		return nil
	}
	// Scan only this game's folders, then sync/heal its list with the links present.
	oldName := a.EnvName
	a.EnvName = e.Name
	defer func() { a.EnvName = oldName }()
	if _, err := a.ScanWith(ScanOptions{GameOnly: true}); err != nil {
		return rollback(err)
	}
	return nil
}
