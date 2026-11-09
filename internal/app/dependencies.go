package app

import (
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/scan"
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
	activeByWS  map[string][]*store.Mod // built on first need
	activeByLin map[string][]*store.Mod // built on first need
	// replacement: Use This Instead's replacement for a package (08 §2.3); nil: none known.
	replacement func(workshopID, pkg string) *Replacement
	// replacedBy: the player's replaces rules, package → the lineages standing in (08 §2.1).
	replacedBy map[string][]string
}

// newDependencyIndex indexes the enabled mods (by packageId, as the game matches them).
func (a *App) newDependencyIndex(st *LoadoutState, activeByPkg map[string][]*store.Mod) *dependencyIndex {
	ix := &dependencyIndex{st: st, activeByPkg: activeByPkg, installed: installedDependencies(st), replacement: a.ReplacementFor}
	// Without its rules a stand-in only goes unseen: the dependency reads as missing again.
	if rs, err := a.Store.Rules(st.Ix.Env.ID); err == nil {
		for _, r := range rs {
			if r.Kind == order.RuleReplaces && r.A != "" && r.Target != "" {
				if ix.replacedBy == nil {
					ix.replacedBy = map[string][]string{}
				}
				ix.replacedBy[r.Target] = append(ix.replacedBy[r.Target], r.A)
			}
		}
	}
	return ix
}

// How an enabled mod stands in for a required package that is not enabled itself.
const (
	// StandInAlternative: the requiring mod lists it in alternativePackageIds; the game
	// counts the dependency as met.
	StandInAlternative = "alternative"
	// StandInReplacement: Use This Instead names it as the package's replacement. The game
	// still warns in its mod list, but loads both, and the replacement is what the player
	// meant to use (08 §1.1).
	StandInReplacement = "replacement"
	// StandInRule: the player's replaces rule names it (08 §2.1); like a replacement, the
	// game still warns in its mod list.
	StandInRule = "rule"
)

// standIns returns the enabled mods that satisfy dep although no mod with its packageId is
// enabled, and how (StandIn*). Alternatives come first, as the game reads them; then the
// player's own rules, then community data.
func (ix *dependencyIndex) standIns(dep scanDep) ([]*store.Mod, string) {
	for _, alt := range dep.alts {
		if ms := ix.activeByPkg[dependencyPackage(alt)]; len(ms) > 0 {
			return ms, StandInAlternative
		}
	}
	if lins := ix.replacedBy[dependencyPackage(dep.pkg)]; len(lins) > 0 {
		if ix.activeByLin == nil {
			ix.activeByLin = map[string][]*store.Mod{}
			for _, ms := range ix.activeByPkg {
				for _, m := range ms {
					ix.activeByLin[m.LineageID] = append(ix.activeByLin[m.LineageID], m)
				}
			}
		}
		var ms []*store.Mod
		for _, lin := range lins {
			ms = append(ms, ix.activeByLin[lin]...)
		}
		if len(ms) > 0 {
			return ms, StandInRule
		}
	}
	if ix.replacement == nil {
		return nil, ""
	}
	r := ix.replacement(workshopID(dep.url), dependencyPackage(dep.pkg))
	if r == nil {
		return nil, ""
	}
	if p := dependencyPackage(r.NewPackageID); p != "" {
		if ms := ix.activeByPkg[p]; len(ms) > 0 {
			return ms, StandInReplacement
		}
	}
	if r.NewWorkshopID == "" {
		return nil, ""
	}
	if ix.activeByWS == nil {
		ix.activeByWS = map[string][]*store.Mod{}
		for _, ms := range ix.activeByPkg {
			for _, m := range ms {
				if w := m.WorkshopID(); w != "" {
					ix.activeByWS[w] = append(ix.activeByWS[w], m)
				}
			}
		}
	}
	if ms := ix.activeByWS[r.NewWorkshopID]; len(ms) > 0 {
		return ms, StandInReplacement
	}
	return nil, ""
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

// DependencyStandIn is the enabled mod that meets dep in place of its package, and how
// (StandIn*); nil when the package itself is enabled or nothing stands in (16 §4.1).
func (a *App) DependencyStandIn(st *LoadoutState, dep scan.Dependency) (*store.Mod, string) {
	activeByPkg := map[string][]*store.Mod{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			p := dependencyPackage(m.PackageID)
			activeByPkg[p] = append(activeByPkg[p], m)
		}
	}
	d := toDeps([]scan.Dependency{dep})[0]
	if len(activeByPkg[dependencyPackage(d.pkg)]) > 0 {
		return nil, ""
	}
	ms, how := a.newDependencyIndex(st, activeByPkg).standIns(d)
	if len(ms) == 0 {
		return nil, ""
	}
	return ms[0], how
}

// missing uses only modDependencies and the current version, never loadAfter or
// community observations. A duplicate declaration can supply a missing name or URL. An
// enabled stand-in (alternative or replacement, standIns) meets the dependency.
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
		if ms, _ := ix.standIns(dep); len(ms) > 0 {
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
	activeByPkg := map[string][]*store.Mod{}
	var active []*store.Mod
	seen := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && !seen[m.ID] {
			seen[m.ID] = true
			active = append(active, m)
			p := dependencyPackage(m.PackageID)
			activeByPkg[p] = append(activeByPkg[p], m)
		}
	}
	ix := a.newDependencyIndex(st, activeByPkg)
	if err := a.Store.LoadDecls(active); err != nil {
		return nil, err
	}
	ignored, err := a.ignoredDependencies(st)
	if err != nil {
		return nil, err
	}
	out := []MissingDependency{}
	byPkg := map[string]int{}
	for _, m := range active {
		for _, dep := range ix.missing(m) {
			code := "dep.missing"
			if len(ix.candidates(dep.pkg)) > 0 {
				code = "dep.inactive"
			}
			if ignored(m, code, dep.pkg) {
				continue
			}
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

// ignoredDependencies says whether the player ignored m's dep.missing / dep.inactive finding
// for a package (05 §4.2): the same match Problems makes, so a dependency hidden in Can it
// run is neither offered after a sort nor enabled by completing dependencies. An ignore
// lapses when m changes.
func (a *App) ignoredDependencies(st *LoadoutState) (func(m *store.Mod, code, pkg string) bool, error) {
	gs, err := a.Store.Ignores(st.Ix.Env.ID)
	if err != nil {
		return nil, err
	}
	type key struct{ lin, code, pkg string }
	snap := map[key]int64{}
	for _, g := range gs {
		if g.Code == "dep.missing" || g.Code == "dep.inactive" {
			snap[key{g.LineageID, g.Code, g.Key}] = g.SnapshotID
		}
	}
	return func(m *store.Mod, code, pkg string) bool {
		s, ok := snap[key{m.LineageID, code, pkg}]
		return ok && s == m.SnapID
	}, nil
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

// DependentsOf lists the enabled mods that would be left without a required mod if the
// instances ids were disabled, in load order: first those that require one of them (with
// no other enabled mod or stand-in meeting it), then the mods that require those, and so
// on. A dependency already missing before does not count.
func (a *App) DependentsOf(st *LoadoutState, ids []string) ([]*store.Mod, error) {
	off := map[string]bool{}
	for _, id := range ids {
		off[id] = true
	}
	var active []*store.Mod
	seen := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && !seen[m.ID] {
			seen[m.ID] = true
			active = append(active, m)
		}
	}
	if err := a.Store.LoadDecls(active); err != nil {
		return nil, err
	}
	missing := func(off map[string]bool) map[string][]string {
		byPkg := map[string][]*store.Mod{}
		for _, m := range active {
			if !off[m.ID] {
				p := dependencyPackage(m.PackageID)
				byPkg[p] = append(byPkg[p], m)
			}
		}
		ix := a.newDependencyIndex(st, byPkg)
		out := map[string][]string{}
		for _, m := range active {
			if !off[m.ID] {
				for _, d := range ix.missing(m) {
					out[m.ID] = append(out[m.ID], d.pkg)
				}
			}
		}
		return out
	}
	base := missing(nil)
	var out []*store.Mod
	for {
		now, more := missing(off), false
		for _, m := range active {
			if off[m.ID] {
				continue
			}
			for _, p := range now[m.ID] {
				if !slices.Contains(base[m.ID], p) {
					out, off[m.ID], more = append(out, m), true, true
					break
				}
			}
		}
		if !more {
			return out, nil
		}
	}
}
