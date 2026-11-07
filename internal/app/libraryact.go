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

// The Mod library actions that are not already somewhere else (16 §2.9): taking mods out of
// every plan, and moving a spare copy out of Mods/ into the lock library.

// PlanRemoval is one plan a removal changed: one undo step in that plan.
type PlanRemoval struct {
	Plan    string
	Current bool
	Removed int
}

// RemoveFromPlans takes the instances out of every plan that has them (the plans Mod library
// lists under "used in"; lists are shared by every game). Each plan gets one undo step;
// nothing is written to the game.
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
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	f := keep(libHere(envs))
	for _, p := range ps {
		r := PlanRemoval{Plan: p.Name}
		if st.Plan != nil && p.ID == st.Plan.ID {
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
	return res, nil
}

// editOther changes a plan that is not the current one, with an undo step in its own
// history (the step is undone once the plan is current, 07 §3).
func (a *App) editOther(p *store.Plan, op string, f func(loadout.List) (loadout.List, int)) (int, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return 0, err
	}
	defer unlock()

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
// library (16 §2.9, 02 §1): a copy made by hand or left by another manager, beside another
// copy of the same mod. Same volume: renamed; another volume: copied, every file checked,
// then the original goes to the recycle bin. Its instance stays the same (ID, lineage,
// history), so every plan keeps it. Incoming installations and an entry needed by
// the active plan are reattached after relocation. .rimarcho/origin.json is written
// only when it has none.
func (a *App) MoveToLocked(ref string, dryRun bool) (*MoveResult, error) {
	return a.MoveToLockedFor(ref, dryRun, "")
}

// MoveToLockedFor is MoveToLocked with the reason, kept in the copy's note (16 §2.9).
func (a *App) MoveToLockedFor(ref string, dryRun bool, reason string) (*MoveResult, error) {
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
	case !m.Kind.GameMods():
		return nil, fmt.Errorf("%s is in src:%s; only copies in a registered game's Mods/ move to the lock library", st.Ix.Ref(m.ID), m.SourceLabel)
	}
	other := false
	for _, o := range st.Ix.ByLineage[m.LineageID] {
		if o.ID != m.ID && o.Status == identity.Live {
			other = true
		}
	}
	// The only copy of its mod moves too when it shares a packageId with another entry of
	// Mods/ (09 §3): there the game would report an error, and in the lock library plans
	// still choose it.
	if !other && m.Origin == "" && !a.inModsDuplicate(st, m.ID) {
		return nil, fmt.Errorf("%s is the only copy of its mod; RimArcho moves only spare copies out of Mods/", st.Ix.Ref(m.ID))
	}
	// Another tool's link to it would break (RimCrow's _Link_…).
	if by := linkedBy(st, m.RealPath, a.ModsCopies(st.Ix)); by != "" {
		return nil, fmt.Errorf("%s: %s", st.Ix.Ref(m.ID), by)
	}
	r, err := a.moveToLibrary(st, m, dryRun)
	if err != nil || dryRun {
		return r, err
	}
	// The copy's note starts from this, unless RimArcho recorded how it made it before.
	notes, err := a.Store.CopyNotes(st.Ix.Env.ID)
	if err != nil {
		return r, err
	}
	if n := notes[m.ID]; n == nil || n.Made == nil {
		var plans []string
		for _, p := range r.Plans {
			plans = append(plans, strings.TrimPrefix(p, "plan:"))
		}
		err = a.Store.SetMade(m.ID, &store.Made{Kind: "move", At: a.Now().UTC(), Plans: plans, Reason: reason})
	}
	return r, err
}

// SetCopyNote sets one copy's own note (16 §2.9); "" clears it. The lineage's note (note,
// has:note) is about the mod and all its copies; this one is about this copy only.
func (a *App) SetCopyNote(ref, text string) (*store.Mod, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	return m, a.Store.SetCopyNote(m.ID, strings.TrimSpace(text), a.Now().UTC())
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
// lineage's Workshop copy, else itself) and its own content as the snapshot, so "a newer version is out"
// means the upstream's content differs from the copy's (07 §5.3).
func (a *App) writeOrigin(ix *Index, m *store.Mod, dir string) error {
	o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.DeclaredWorkshop, SnapshotHash: hex.EncodeToString(m.SnapHash),
		CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
	if up := upstreamOf(ix, m); up != nil {
		o.InstanceID, o.WorkshopID = up.ID, up.WorkshopID()
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	// RimArcho's marker is not an update of the mod: the folder keeps its time (update date).
	fi, _ := os.Stat(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".rimarcho"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
		return err
	}
	if fi != nil {
		os.Chtimes(dir, fi.ModTime(), fi.ModTime())
	}
	return nil
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
	uses, _, err := a.libUses(st)
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

// PlansUsing is usedBy for the pages: deleting or unsubscribing such a copy warns, and goes.
func (a *App) PlansUsing(st *LoadoutState, id string) ([]string, error) { return a.usedBy(st, id) }
