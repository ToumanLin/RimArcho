package app

import (
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

// DeployPrefix marks the Mods/ links RimArcho manages (09 §8). It only ever creates or
// removes entries with this prefix.
const DeployPrefix = "_Archo_"

// DeployName is the Mods/ link name of a deployed instance.
func DeployName(m *store.Mod) string {
	r := strings.NewReplacer("/", "_", `\`, "_", ":", "_")
	return DeployPrefix + r.Replace(m.SourceLabel) + "_" + r.Replace(m.Rel)
}

func managedLink(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(DeployPrefix))
}

func gameVisible(k source.Kind) bool {
	return k == source.Official || k == source.Local || k == source.Workshop
}

// deployable reports whether m lives where the game does not look (SteamCMD or an extra
// folder) and could be linked into Mods/ (09 §8.1). An instance the game already sees
// through someone else's link is not deployable: it is an ordinary candidate.
func deployable(ix *Index, aliases map[string][]store.Alias, m *store.Mod) bool {
	if m.Status != identity.Live || m.Residue || m.Unloadable != "" || (m.Kind != source.SteamCMD && m.Kind != source.Extra) {
		return false
	}
	kinds := map[string]source.Kind{}
	for _, s := range ix.Sources {
		kinds[s.ID] = s.Kind
	}
	for _, al := range aliases[m.ID] {
		if gameVisible(kinds[al.SourceID]) && !managedLink(al.Rel) {
			return false
		}
	}
	return true
}

// Deployable reports whether m can be enabled through a link RimArcho creates on save.
func (st *LoadoutState) Deployable(m *store.Mod) bool { return deployable(st.Ix, st.Aliases, m) }

// Deployed reports whether m is enabled through a RimArcho link (existing or due on save).
func (st *LoadoutState) Deployed(m *store.Mod) bool { return st.deploy[m.ID] }

// regWith is the registry as it would be with id deployed as well.
func (st *LoadoutState) regWith(id string) *loadout.Registry {
	if st.Reg.Known(id) {
		return st.Reg
	}
	m := st.Ix.ByID[id]
	if m == nil || !st.Deployable(m) {
		return st.Reg
	}
	return loadout.NewRegistry(append(st.cands[:len(st.cands):len(st.cands)], deployCandidate(m)))
}

func deployCandidate(m *store.Mod) loadout.Candidate {
	return loadout.Candidate{ID: m.ID, PackageID: m.PackageID, Lineage: m.LineageID, Kind: source.Local, Folder: DeployName(m)}
}

// deploySet picks the deployable instances the registry should treat as present in Mods/:
// those in the workspace (they will be linked on save) and those whose RimArcho link is on
// disk now (the game sees them until the next save removes the link).
func (a *App) deploySet(ix *Index, aliases map[string][]store.Alias) (map[string]bool, error) {
	ws, err := a.Store.Workspace(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	if ws != nil {
		for _, e := range loadout.Decode(ws.Entries) {
			if m := ix.ByID[e.Instance]; m != nil && deployable(ix, aliases, m) {
				set[m.ID] = true
			}
		}
	}
	if mods := modsDir(ix); mods != "" {
		for _, m := range ix.Mods {
			if !set[m.ID] && deployable(ix, aliases, m) {
				if _, err := os.Lstat(filepath.Join(mods, DeployName(m))); err == nil {
					set[m.ID] = true
				}
			}
		}
	}
	return set, nil
}

// modsDir is the game's Mods/ folder: the root of the environment's local source.
func modsDir(ix *Index) string {
	for _, s := range ix.Sources {
		if s.Kind == source.Local {
			return s.Root
		}
	}
	if ix.Env.GameRoot != "" {
		return filepath.Join(ix.Env.GameRoot, "Mods")
	}
	return ""
}

// LinkChange is one Mods/ link a save creates or removes (09 §8.1).
type LinkChange struct {
	Op     string // "add" or "remove"
	Name   string // link name in Mods/
	Target string // the instance's real folder ("" for a dangling link)
	Mod    string // instance ID ("" for a link to nothing RimArcho knows)
}

// planLinks compares the RimArcho links in Mods/ with the deployed instances of the list.
func planLinks(st *LoadoutState) (string, []LinkChange, error) {
	mods := modsDir(st.Ix)
	want := map[string]*store.Mod{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && st.Deployable(m) {
			want[strings.ToLower(DeployName(m))] = m
		}
	}
	if mods == "" {
		if len(want) > 0 {
			return "", nil, fmt.Errorf("the environment has no Mods/ folder to link %d mod(s) into; set the game folder with archo env set --game <dir>", len(want))
		}
		return "", nil, nil
	}
	var out []LinkChange
	have := map[string]bool{}
	ents, err := os.ReadDir(mods)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	for _, d := range ents {
		name := d.Name()
		if packLink(name) && source.IsLink(filepath.Join(mods, name)) {
			// A pack's link (10 §8.3): saving the normal list ends the pack's run.
			tgt, _ := source.RealPath(filepath.Join(mods, name))
			out = append(out, LinkChange{Op: "remove", Name: name, Target: tgt})
			continue
		}
		if !managedLink(name) {
			continue
		}
		p := filepath.Join(mods, name)
		if !source.IsLink(p) {
			return "", nil, fmt.Errorf("%s starts with %s but is not a link; RimArcho does not touch it (rename or remove it yourself)", p, DeployPrefix)
		}
		m := want[strings.ToLower(name)]
		if m != nil && sameDir(p, m.RealPath) {
			have[strings.ToLower(name)] = true
			continue
		}
		tgt, _ := source.RealPath(p)
		c := LinkChange{Op: "remove", Name: name, Target: tgt}
		if m != nil {
			c.Mod = m.ID
		}
		out = append(out, c)
	}
	for key, m := range want {
		if !have[key] {
			if p := filepath.Join(mods, DeployName(m)); !source.IsLink(p) {
				if _, err := os.Lstat(p); err == nil {
					return "", nil, fmt.Errorf("%s already exists and is not a link; RimArcho does not touch it", p)
				}
			}
			out = append(out, LinkChange{Op: "add", Name: DeployName(m), Target: m.RealPath, Mod: m.ID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Op != out[j].Op {
			return out[i].Op > out[j].Op // removals first
		}
		return out[i].Name < out[j].Name
	})
	return mods, out, nil
}

func sameDir(link, target string) bool {
	a, err1 := source.RealPath(link)
	b, err2 := source.RealPath(target)
	return err1 == nil && err2 == nil && env.PathKey(a) == env.PathKey(b)
}

// applyLinks carries out a link plan: removals first (a stale link of the same name is
// replaced), then additions. Only links are deleted; their targets are never entered.
func applyLinks(mods string, plan []LinkChange) error {
	for _, c := range plan {
		p := filepath.Join(mods, c.Name)
		switch c.Op {
		case "remove":
			if err := source.RemoveLink(p); err != nil {
				return err
			}
		case "add":
			if _, err := os.Stat(c.Target); err != nil {
				return fmt.Errorf("cannot link %s: %v", c.Name, err)
			}
			if err := source.CreateLink(p, c.Target); err != nil {
				return fmt.Errorf("cannot link %s into %s: %v (the Mods/ volume must support junctions, e.g. NTFS)", c.Target, mods, err)
			}
		}
	}
	return nil
}
