package app

import (
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// MissingDependency is one required About.xml package that has no enabled instance.
// Kind is installed (enable Instance), workshop (subscribe WorkshopID), or unknown.
type MissingDependency struct {
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	PackageID  string   `json:"packageId"`
	RequiredBy []string `json:"requiredBy"`
	Instance   string   `json:"instance,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	WorkshopID string   `json:"workshopId,omitempty"`
}

type dependencyIndex struct {
	st          *LoadoutState
	activeByPkg map[string][]*store.Mod
	installed   map[string][]*store.Mod
	loadable    map[string][]*store.Mod
}

func dependencyPackage(p string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(p)), "_steam")
}

func installedDependencies(st *LoadoutState) map[string][]*store.Mod {
	installed := map[string][]*store.Mod{}
	// Preserve Index.Mods order: the same first instance the needs page offers.
	for _, m := range st.Ix.Mods {
		if m.TombstoneAt.IsZero() && !m.Residue && m.Unloadable == "" && (st.Reg.Known(m.ID) || st.Deployable(m)) {
			p := dependencyPackage(m.PackageID)
			installed[p] = append(installed[p], m)
		}
	}
	return installed
}

func (ix *dependencyIndex) candidates(p string) []*store.Mod {
	if ix.loadable == nil {
		ix.loadable = map[string][]*store.Mod{}
	}
	if c, ok := ix.loadable[p]; ok {
		return c
	}
	var out []*store.Mod
	for _, m := range ix.installed[p] {
		if whyNotLoadable(ix.st, m) == "" {
			out = append(out, m)
		}
	}
	ix.loadable[p] = out
	return out
}

// missing uses only modDependencies and the current version, never loadAfter or
// community observations. A duplicate declaration can supply a missing name or URL.
func (ix *dependencyIndex) missing(m *store.Mod) []scanDep {
	d := m.Decl()
	deps := append(toDeps(d.Dependencies), toDeps(d.DependenciesByVersion["v"+ix.st.GameMM()])...)
	var out []scanDep
	seen := map[string]int{}
	for _, dep := range deps {
		dep.pkg = dependencyPackage(dep.pkg)
		if dep.pkg == "" || len(ix.activeByPkg[dep.pkg]) > 0 {
			continue
		}
		if i, ok := seen[dep.pkg]; ok {
			if out[i].name == "" {
				out[i].name = dep.name
			}
			if out[i].url == "" {
				out[i].url = dep.url
			}
			continue
		}
		seen[dep.pkg] = len(out)
		out = append(out, dep)
	}
	return out
}

func (a *App) dependencyWorkshop(dep scanDep) (id, name string) {
	// Official content is obtained with the game, never from a Workshop substitute.
	if dep.pkg == "ludeon.rimworld" || strings.HasPrefix(dep.pkg, "ludeon.rimworld.") {
		return "", ""
	}
	if id = workshopID(dep.url); id != "" {
		return id, dep.name
	}
	if w, ok := a.WorkshopFor(dep.pkg); ok {
		return w.WorkshopID, w.Name
	}
	return "", ""
}

// MissingDependencies reads cached declarations in bulk; it never scans files or
// invokes the full problem/analysis pipeline. Missing list entries do not satisfy deps.
func (a *App) MissingDependencies(st *LoadoutState) ([]MissingDependency, error) {
	ix := dependencyIndex{st: st, activeByPkg: map[string][]*store.Mod{}, installed: installedDependencies(st)}
	var active []*store.Mod
	seen := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && !seen[m.ID] {
			seen[m.ID] = true
			active = append(active, m)
			p := dependencyPackage(m.PackageID)
			ix.activeByPkg[p] = append(ix.activeByPkg[p], m)
		}
	}
	if err := a.Store.LoadDecls(active); err != nil {
		return nil, err
	}
	out := []MissingDependency{}
	byPkg := map[string]int{}
	for _, m := range active {
		for _, dep := range ix.missing(m) {
			i, ok := byPkg[dep.pkg]
			if !ok {
				i = len(out)
				byPkg[dep.pkg] = i
				item := MissingDependency{Kind: "unknown", Name: dep.pkg, PackageID: dep.pkg, RequiredBy: []string{}}
				if c := ix.candidates(dep.pkg); len(c) > 0 {
					item.Kind, item.Instance, item.Ref, item.Name = "installed", c[0].ID, st.Ix.Ref(c[0].ID), c[0].DisplayName()
				}
				out = append(out, item)
			}
			item := &out[i]
			if item.Kind != "installed" {
				if item.Name == item.PackageID && dep.name != "" {
					item.Name = dep.name
				}
				if id, name := a.dependencyWorkshop(dep); id != "" && item.WorkshopID == "" {
					item.Kind, item.WorkshopID = "workshop", id
					if item.Name == item.PackageID && name != "" {
						item.Name = name
					}
				}
			}
			if !slices.Contains(item.RequiredBy, m.DisplayName()) {
				item.RequiredBy = append(item.RequiredBy, m.DisplayName())
			}
		}
	}
	return out, nil
}

// DependencyCompletion enables the available dependency closure and sorts in one edit.
// WorkshopIDs are deduplicated subscription inputs, not a subscription side effect.
type DependencyCompletion struct {
	Edit                *EditResult
	Enabled             []string
	WorkshopIDs         []string
	MissingDependencies []MissingDependency
}

func (a *App) CompleteDependencies() (*DependencyCompletion, error) {
	out := &DependencyCompletion{Enabled: []string{}, WorkshopIDs: []string{}}
	r, err := a.edit("complete dependencies and sort", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		state := *st
		for {
			state.List = l
			deps, err := a.MissingDependencies(&state)
			if err != nil {
				return nil, nil, err
			}
			var refs []string
			for _, dep := range deps {
				if dep.Kind == "installed" && !slices.Contains(out.Enabled, dep.Instance) {
					refs = append(refs, dep.Ref)
					out.Enabled = append(out.Enabled, dep.Instance)
				}
			}
			if len(refs) == 0 {
				break
			}
			l, _, err = a.enableList(&state, l, refs, Placement{})
			if err != nil {
				return nil, nil, err
			}
		}
		sorted, err := a.sortList(&state, l)
		if err != nil {
			return nil, nil, err
		}
		out.MissingDependencies = sorted.MissingDependencies
		for _, dep := range out.MissingDependencies {
			if dep.WorkshopID != "" && !slices.Contains(out.WorkshopIDs, dep.WorkshopID) {
				out.WorkshopIDs = append(out.WorkshopIDs, dep.WorkshopID)
			}
		}
		return sorted.Sorted, nil, nil
	})
	if err != nil {
		return nil, err
	}
	out.Edit = r
	return out, nil
}
