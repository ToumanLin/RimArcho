package app

import (
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SortMod identifies a list entry or a replacement. Missing instances omit Instance/Ref.
type SortMod struct {
	Name       string `json:"name"`
	PackageID  string `json:"packageId"`
	WorkshopID string `json:"workshopId,omitempty"`
	Instance   string `json:"instance,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func sortMod(st *LoadoutState, m *store.Mod) SortMod {
	return SortMod{Name: m.DisplayName(), PackageID: dependencyPackage(m.PackageID), WorkshopID: numericWorkshopID(m.WorkshopID()), Instance: m.ID, Ref: st.Ix.Ref(m.ID)}
}

func numericWorkshopID(id string) string {
	if id == "" || id == "0" {
		return ""
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return id
}

// MissingMod is unresolved in the current list. Installed copies in another
// game's Mods/ or a deployable source may still exist; CanEnable names a usable one.
type MissingMod struct {
	SortMod
	Installed bool `json:"installed"`
	CanEnable bool `json:"canEnable"`
}

// MissingMods preserves imported provenance, then consults stored lineage instances
// (including tombstones), then the locally cached public Workshop index. No network.
func (a *App) MissingMods(st *LoadoutState) []MissingMod {
	byPkg := map[string][]*store.Mod{}
	for _, m := range st.Ix.Mods {
		p := dependencyPackage(m.PackageID)
		byPkg[p] = append(byPkg[p], m)
	}
	var out []MissingMod
	seen := map[string]int{}
	for _, e := range st.List {
		if e.Missing == "" {
			continue
		}
		p := dependencyPackage(e.Missing)
		item := MissingMod{SortMod: SortMod{Name: e.Name, PackageID: p, WorkshopID: numericWorkshopID(e.WorkshopID)}}
		candidates := byPkg[p]
		if e.Lineage != "" {
			candidates = append(append([]*store.Mod{}, st.Ix.ByLineage[e.Lineage]...), candidates...)
		}
		for _, m := range candidates {
			if m.Status == identity.Live && dependencyPackage(m.PackageID) == p {
				item.Installed = true
				if !item.CanEnable && whyNotLoadable(st, m) == "" {
					item.CanEnable = true
					item.Instance, item.Ref = m.ID, st.Ix.Ref(m.ID)
				}
			}
			if item.Name == "" {
				item.Name = m.DisplayName()
			}
			if item.WorkshopID == "" {
				item.WorkshopID = numericWorkshopID(m.WorkshopID())
			}
		}
		if w, ok := a.WorkshopFor(p); ok {
			if item.Name == "" {
				item.Name = w.Name
			}
			if item.WorkshopID == "" {
				item.WorkshopID = numericWorkshopID(w.WorkshopID)
			}
		}
		if order.IsOfficial(p) {
			item.WorkshopID = ""
		}
		if item.Name == "" {
			item.Name = p
		}
		if i, ok := seen[p]; ok {
			if out[i].Name == p && item.Name != p {
				out[i].Name = item.Name
			}
			if out[i].WorkshopID == "" {
				out[i].WorkshopID = item.WorkshopID
			}
			continue
		}
		seen[p] = len(out)
		out = append(out, item)
	}
	return out
}

// ReplacementSuggestion reuses Use This Instead and the current game's version filter.
type ReplacementSuggestion struct {
	Original      SortMod `json:"original"`
	Replacement   SortMod `json:"replacement"`
	Installed     bool    `json:"installed"`
	Enabled       bool    `json:"enabled"`
	CanReplace    bool    `json:"canReplace"`
	BlockedReason string  `json:"blockedReason,omitempty"`
	Reason        string  `json:"reason"`
	Source        string  `json:"source"`
}

func (a *App) replacementFor(st *LoadoutState, m *store.Mod) *Replacement {
	r := a.ReplacementFor(m.WorkshopID(), dependencyPackage(m.PackageID))
	if r == nil || (len(r.NewVersions) > 0 && !slices.Contains(r.NewVersions, st.GameMM())) {
		return nil
	}
	samePkg := dependencyPackage(r.NewPackageID) == dependencyPackage(m.PackageID)
	if samePkg && (r.NewWorkshopID == "" || r.NewWorkshopID == m.WorkshopID()) {
		return nil
	}
	if r.NewPackageID == "" && r.NewWorkshopID == "" {
		return nil
	}
	return r
}

func replacementCandidate(st *LoadoutState, old *store.Mod, r *Replacement) *store.Mod {
	var fallback, blocked *store.Mod
	for _, m := range st.Ix.Mods {
		if m.ID == old.ID || m.Status != identity.Live {
			continue
		}
		pkgMatch := r.NewPackageID != "" && dependencyPackage(m.PackageID) == dependencyPackage(r.NewPackageID)
		wsMatch := r.NewWorkshopID != "" && m.WorkshopID() == r.NewWorkshopID
		if !pkgMatch && !wsMatch {
			continue
		}
		if pkgMatch && r.NewWorkshopID != "" && m.WorkshopID() != "" && !wsMatch {
			continue
		}
		if why := swapBlocker(st, m, old); why != "" {
			if blocked == nil {
				blocked = m
			}
			continue
		}
		if wsMatch {
			return m
		}
		if pkgMatch && (r.NewWorkshopID == "" || m.WorkshopID() == "") && fallback == nil {
			fallback = m
		}
	}
	if fallback != nil {
		return fallback
	}
	return blocked
}

func (a *App) ReplacementSuggestions(st *LoadoutState) []ReplacementSuggestion {
	var out []ReplacementSuggestion
	seen := map[string]bool{}
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		r := a.replacementFor(st, m)
		if r == nil {
			continue
		}
		target := SortMod{Name: r.NewName, PackageID: dependencyPackage(r.NewPackageID), WorkshopID: numericWorkshopID(r.NewWorkshopID)}
		installed, enabled, canReplace := false, false, false
		blockedReason := ""
		if c := replacementCandidate(st, m, r); c != nil {
			target = sortMod(st, c)
			if target.WorkshopID == "" {
				target.WorkshopID = numericWorkshopID(r.NewWorkshopID)
			}
			installed, enabled = true, st.List.Index(c.ID) >= 0
			blockedReason = swapBlocker(st, c, m)
			canReplace = blockedReason == ""
		}
		if target.Name == "" {
			target.Name = target.PackageID
		}
		out = append(out, ReplacementSuggestion{Original: sortMod(st, m), Replacement: target, Installed: installed, Enabled: enabled, CanReplace: canReplace, BlockedReason: blockedReason,
			Reason: fmt.Sprintf("Use This Instead recommends %s for %s", target.Name, st.GameMM()), Source: "Use This Instead"})
	}
	return out
}

// ReplaceRecommended revalidates recommendations against the open plan. All requested
// originals are replaced in one edit, retaining positions and avoiding duplicate targets.
func (a *App) ReplaceRecommended(refs []string) (*EditResult, error) {
	return a.edit("replace recommended mods", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		targets := map[string]*store.Mod{}
		for _, ref := range refs {
			old, err := st.Ix.ResolveOne(ref)
			if err != nil {
				return nil, nil, err
			}
			if l.Index(old.ID) < 0 {
				return nil, nil, fmt.Errorf("%s is not enabled", ref)
			}
			r := a.replacementFor(st, old)
			if r == nil {
				return nil, nil, fmt.Errorf("%s has no replacement recommendation for %s", ref, st.GameMM())
			}
			target := replacementCandidate(st, old, r)
			if target == nil {
				return nil, nil, fmt.Errorf("the recommended replacement for %s is not installed or cannot be loaded", ref)
			}
			if why := swapBlocker(st, target, old); why != "" {
				return nil, nil, fmt.Errorf("cannot replace %s: %s", ref, why)
			}
			targets[old.ID] = target
		}
		for old, target := range targets {
			if targets[target.ID] != nil {
				return nil, nil, fmt.Errorf("replacement chain includes %s; replace it in separate steps", old)
			}
		}
		targetIDs := map[string]bool{}
		for _, target := range targets {
			targetIDs[target.ID] = true
		}
		replaced := make(loadout.List, 0, len(l))
		seen := map[string]bool{}
		for _, e := range l {
			if target := targets[e.Instance]; target != nil {
				e = loadout.Entry{Instance: target.ID}
			}
			if targetIDs[e.Instance] {
				if seen[e.Instance] {
					continue
				}
				seen[e.Instance] = true
			}
			replaced = append(replaced, e)
		}
		state := *st
		state.List = replaced
		for _, target := range targets {
			for _, other := range state.regWith(target.ID).SamePackage(target.ID) {
				if other.ID != target.ID && replaced.Index(other.ID) >= 0 {
					return nil, nil, fmt.Errorf("cannot enable %s: another copy of pkg:%s is enabled", target.DisplayName(), target.PackageID)
				}
			}
		}
		return replaced, nil, nil
	})
}

// SortConstraint is an edge set aside while resolving a cycle, with names for display.
type SortConstraint struct {
	Before   SortMod `json:"before"`
	After    SortMod `json:"after"`
	Kind     string  `json:"kind"`
	Strength string  `json:"strength"`
	Reason   string  `json:"reason"`
}

type SortCycle struct {
	Members    []SortMod        `json:"members"`
	Broken     []SortConstraint `json:"broken"`
	Resolution string           `json:"resolution"`
}

func sortCycles(st *LoadoutState, in order.Input, r order.Result) []SortCycle {
	mod := func(i int) SortMod {
		n := in.Nodes[i]
		if m := st.Ix.ByID[n.Instance]; m != nil {
			return sortMod(st, m)
		}
		return SortMod{Name: n.PackageID, PackageID: n.PackageID}
	}
	edge := func(e order.Edge, reason string) SortConstraint {
		return SortConstraint{Before: mod(e.From), After: mod(e.To), Kind: e.C.Kind, Strength: order.Assess(in, e).Strength.String(), Reason: reason}
	}
	var out []SortCycle
	for _, cy := range r.Cycles {
		c := SortCycle{Resolution: "keep the strongest constraints; preserve the current order when priorities tie"}
		for _, i := range cy.Members {
			c.Members = append(c.Members, mod(i))
		}
		for _, i := range cy.Broken {
			c.Broken = append(c.Broken, edge(cy.Edges[i], c.Resolution))
		}
		out = append(out, c)
	}
	for _, d := range r.Dropped {
		if d.Reason != order.ReasonOfficialFirst {
			continue
		}
		c := SortCycle{Broken: []SortConstraint{edge(d.Edge, d.Reason)}, Resolution: d.Reason}
		for _, i := range d.Members {
			c.Members = append(c.Members, mod(i))
		}
		out = append(out, c)
	}
	return out
}

// SortNotices is the shared after-sort payload for GUI and CLI, including dry runs.
type SortNotices struct {
	MissingDependencies []MissingDependency     `json:"missingDependencies,omitempty"`
	MissingMods         []MissingMod            `json:"missingMods,omitempty"`
	Replacements        []ReplacementSuggestion `json:"replacements,omitempty"`
	Cycles              []SortCycle             `json:"cycles,omitempty"`
}
