package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The Mod 库存 actions that are not already somewhere else (06 §2.9): taking mods out of
// every plan, and moving a spare copy out of Mods/ into the lock library.

// PlanRemoval is one plan a removal changed: one undo step in that plan.
type PlanRemoval struct {
	Plan    string // "<env>/<plan>" for another environment of the same game
	Current bool
	Removed int
}

// RemoveFromPlans takes the instances out of every plan that has them, in this environment
// and every other one of the same game (the plans Mod 库存 lists under "used in"). Each
// plan gets one undo step; nothing is written to the game.
func (a *App) RemoveFromPlans(refs []string) ([]PlanRemoval, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	drop := map[string]bool{}
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		drop[m.ID] = true
	}
	envs, err := a.libEnvs(st)
	if err != nil {
		return nil, err
	}
	keep := func(here func(string) string) func(loadout.List) (loadout.List, int) {
		return func(l loadout.List) (loadout.List, int) {
			out := loadout.List{}
			for _, e := range l {
				if e.Instance != "" && drop[here(e.Instance)] {
					continue
				}
				out = append(out, e)
			}
			return out, len(l) - len(out)
		}
	}
	var res []PlanRemoval
	for _, le := range envs {
		ps, err := a.Store.Plans(le.e.ID)
		if err != nil {
			return res, err
		}
		f := keep(le.here)
		for _, p := range ps {
			r := PlanRemoval{Plan: p.Name}
			if !le.cur {
				r.Plan = le.e.Name + "/" + p.Name
			}
			if le.cur && st.Plan != nil && p.ID == st.Plan.ID {
				r.Current = true
				_, err = a.edit("disable", func(_ *LoadoutState, l loadout.List) (loadout.List, []string, error) {
					out, n := f(l)
					r.Removed = n
					return out, nil, nil
				})
			} else {
				r.Removed, err = a.editOther(p, "disable", f)
			}
			if err != nil {
				return res, err
			}
			if r.Removed > 0 {
				res = append(res, r)
			}
		}
	}
	return res, nil
}

// editOther changes a plan that is not the current one, with an undo step in its own
// history (the step is undone once the plan is current, 04 §3).
func (a *App) editOther(p *store.Plan, op string, f func(loadout.List) (loadout.List, int)) (int, error) {
	ws, err := a.planWorkspace(p)
	if err != nil {
		return 0, err
	}
	before := loadout.Decode(ws.Entries)
	after, n := f(before.Clone())
	if n == 0 {
		return 0, nil
	}
	return n, a.editPlan(p, ws, before, after, op)
}

// MoveToLocked moves a spare copy of a lineage out of the game's Mods/ into the lock
// library (06 §2.9, 01 §4): a copy made by hand or left by another manager, beside another
// copy of the same mod. Same volume: renamed; another volume: copied, every file checked,
// then the original goes to the recycle bin. Its instance stays the same (ID, lineage,
// history), so every plan keeps it; the game gets it through an _Archo_ link on the next
// save or launch. .rimarcho/origin.json is written only when it has none.
func (a *App) MoveToLocked(ref string, dryRun bool) (*MoveResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	switch {
	case m.Status != identity.Live:
		return nil, fmt.Errorf("%s is no longer on disk", st.Ix.Ref(m.ID))
	case m.Kind != source.Local:
		return nil, fmt.Errorf("%s is in src:%s; only copies in the game's Mods/ move to the lock library", st.Ix.Ref(m.ID), m.SourceLabel)
	}
	other := false
	for _, o := range st.Ix.ByLineage[m.LineageID] {
		if o.ID != m.ID && o.Status == identity.Live {
			other = true
		}
	}
	if !other && m.Origin == "" {
		return nil, fmt.Errorf("%s is the only copy of its mod; RimArcho moves only spare copies out of Mods/", st.Ix.Ref(m.ID))
	}
	return a.moveToLibrary(st, m, dryRun)
}

// upstreamOf is the lineage's Workshop (else SteamCMD) instance a spare copy came from.
func upstreamOf(ix *Index, m *store.Mod) *store.Mod {
	var best *store.Mod
	for _, o := range ix.ByLineage[m.LineageID] {
		if o.ID == m.ID || o.Status != identity.Live || !o.Kind.WorkshopLike() {
			continue
		}
		if best == nil || o.Kind == source.Workshop && best.Kind != source.Workshop {
			best = o
		}
	}
	return best
}

// writeOrigin gives a moved spare copy the lock library's marker: its upstream (the
// lineage's Workshop copy, else itself) and its own content as the snapshot, so "上游有更新"
// means the upstream's content differs from the copy's (04 §7.3).
func (a *App) writeOrigin(ix *Index, m *store.Mod, dir string) error {
	o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.DeclaredWorkshop, SnapshotHash: hex.EncodeToString(m.SnapHash),
		CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
	if up := upstreamOf(ix, m); up != nil {
		o.InstanceID, o.WorkshopID = up.ID, up.WorkshopID()
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	if err := os.MkdirAll(filepath.Join(dir, ".rimarcho"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644)
}

// linksOutside lists the links inside dir that point outside it (relative paths).
func linksOutside(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir || d.Type().IsRegular() {
			return nil
		}
		if !source.IsLink(p) {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if real, err := source.RealPath(p); err != nil || !source.Within(real, dir) {
			out = append(out, filepath.ToSlash(rel))
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return out
}

func hasOrigin(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(scan.OriginFile)))
	return err == nil
}

// usedBy names the plans that use an instance (every environment of this game).
func (a *App) usedBy(st *LoadoutState, id string) ([]string, error) {
	uses, err := a.libUses(st)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, u := range uses[id] {
		if u.Kind == "plan" {
			out = append(out, "plan:"+u.Name)
		}
	}
	return out, nil
}

func joinPlans(ps []string) string { return strings.Join(ps, ", ") }
