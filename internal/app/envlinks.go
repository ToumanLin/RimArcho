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
func (a *App) prepareEnvLinks(e *env.Environment) error {
	ix, err := a.loadIndex(e)
	if err != nil {
		return err
	}
	var tokens []string
	var selected []*store.Mod
	if e.UserDataRoot != "" {
		cfg, err := loadout.ReadModsConfig(env.ModsConfigPath(e.UserDataRoot))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if cfg != nil {
			tokens = append(tokens, cfg.Active...)
		}
	}
	if p, err := a.planFor(e.ID); err != nil {
		return err
	} else if p != nil {
		ws, err := a.Store.Workspace(p.ID)
		if err != nil {
			return err
		}
		if ws != nil {
			for _, entry := range loadout.Decode(ws.Entries) {
				if entry.Missing != "" {
					tokens = append(tokens, entry.Missing)
				} else if m := ix.ByID[entry.Instance]; m != nil {
					selected = append(selected, m)
				}
			}
		}
	}
	if len(tokens) == 0 && len(selected) == 0 {
		return nil
	}

	// Shared extra/SteamCMD sources (including Monitor) need no previous environment.
	_, aliases, cands, _, err := a.registry(ix)
	if err != nil {
		return err
	}
	mods := append([]*store.Mod(nil), ix.Mods...)
	if prev, err := a.Store.CurrentEnv(); err != nil {
		return err
	} else if prev != nil && prev.ID != e.ID {
		old, err := a.loadIndex(prev)
		if err != nil {
			return err
		}
		_, _, oldCands, _, err := a.registry(old)
		if err != nil {
			return err
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
	// On first use there may be no deployed version yet. An unambiguous shared mod
	// can still satisfy the config; for Monitor use its newest installed version.
	fallback := map[string][]*store.Mod{}
	for _, m := range ix.Mods {
		if !packages[m.PackageID] && deployable(ix, aliases, m) {
			fallback[m.PackageID] = append(fallback[m.PackageID], m)
		}
	}
	if ms := monitorInstances(ix); len(ms) > 0 && !packages[MonitorPackage] {
		fallback[MonitorPackage] = []*store.Mod{ms[0].Mod}
	}
	for _, ms := range fallback {
		if len(ms) == 1 {
			unique = append(unique, deployCandidate(ms[0]))
		}
	}
	reg := loadout.NewRegistry(unique)
	byID := map[string]*store.Mod{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	modsDir := modsDir(ix)
	if modsDir == "" {
		return nil
	}
	if err := a.checkRoots(modsDir); err != nil {
		return err
	}
	// Read the destination on disk too: a freshly added game may not be scanned yet.
	present := map[string]bool{}
	for _, c := range a.ModsCopies(ix) {
		present[c.PackageID] = true
	}
	want := map[string]LinkChange{}
	for _, token := range tokens {
		if m := byID[reg.Resolve(token).Instance]; m != nil {
			selected = append(selected, m)
		}
	}
	for _, m := range selected {
		if m == nil || m.Status != identity.Live || m.Residue || m.Unloadable != "" || present[m.PackageID] {
			continue
		}
		name := ""
		switch {
		case m.Kind == source.Local:
			if s := ix.Source(m.SourceID); s != nil {
				if !envLink(m.Rel) {
					continue // ordinary local mods belong to the destination
				}
				name = m.Rel // repair a removed cross-environment link
			} else {
				name = envLinkPrefix + m.ID
			}
		case deployable(ix, aliases, m):
			name = DeployName(m)
		default:
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
			return fmt.Errorf("%s already exists; keep it or remove it before switching environments", p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		want[name] = LinkChange{Op: "add", Name: name, Target: m.RealPath, Mod: m.ID}
		present[m.PackageID] = true
	}
	if len(want) == 0 {
		if sc, _, err := a.LastScan(e.ID); err != nil {
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
	// Scan only this game's folders, then sync/heal its list with the links present.
	oldName := a.EnvName
	a.EnvName = e.Name
	defer func() { a.EnvName = oldName }()
	if _, err := a.ScanWith(ScanOptions{GameOnly: true}); err != nil {
		return rollback(err)
	}
	return nil
}
