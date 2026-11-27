package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Applying or undoing several looks at once (21 §3.3, the gallery's selection): one index,
// one rebuild of the generated Mod and, for an apply, one undoable plan edit.

// LooksManyItem is the outcome for one look of the selection.
type LooksManyItem struct {
	Look string
	// Err says why this look was left out (the Mod has no texture of it, no such look, no
	// choice to undo); "" when it was applied or undone.
	Err     string
	Mixed   bool // apply: the in-game result takes slots from more than one Mod
	Removed int  // undo: the choices removed
}

// LooksManyResult is what an apply or undo of several looks did, or with DryRun would do.
// Its Look is "" and its Mixed false: Items carries them per look.
type LooksManyResult struct {
	*LooksResult
	Items []LooksManyItem
	Done  int // the looks applied or undone
}

func looksIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// LooksApplyMany chooses that every look of ids comes, whole, from the Mod modRef names. A
// look the Mod has no texture of is reported in Items and left as it is; when no look can
// be applied the call fails. The plan gains _Archo_Looks as one undoable edit, and the
// generated Mod is rebuilt once. dryRun writes nothing.
func (a *App) LooksApplyMany(ctx context.Context, ids []string, modRef, scope string, dryRun bool) (*LooksManyResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	sc, err := looksScope(st, scope)
	if err != nil {
		return nil, err
	}
	ls, err := a.LooksState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	out := &LooksManyResult{LooksResult: &LooksResult{DryRun: dryRun, Scope: scopeName(scope), Plan: st.Plan.Name}}
	var first error
	for _, id := range looksIDs(ids) {
		m, rows, err := a.looksApplyRows(ls, id, modRef, nil, sc)
		it := LooksManyItem{Look: id}
		if err != nil {
			if first == nil {
				first = err
			}
			it.Err = err.Error()
		} else {
			out.From, out.Set = m, append(out.Set, rows...)
			out.Done++
		}
		out.Items = append(out.Items, it)
	}
	if out.Done == 0 {
		if first == nil {
			first = errors.New("no look was named")
		}
		return nil, first
	}
	if fs := a.modsVolumeFS(st.Ix); noLinks(fs) {
		return nil, fmt.Errorf("Mods/ is on %s, which cannot hold the link %s needs (09 §3)", fs, LooksName)
	}
	r := out.LooksResult
	if dryRun {
		rows, _ := a.Store.LookChoices(store.LookScope(st.Plan.ID), store.LookScopeAll)
		for _, c := range out.Set {
			rows = withRows(rows, []store.LookChoice{c}, sc, c.Look, true)
		}
		if _, err := a.looksPreview(r, ls, rows); err != nil {
			return nil, err
		}
		out.mixed()
		return out, nil
	}
	if _, r.Dir, r.DirChosen, err = a.ensureLooksMod(st.Ix.Env.GameRoot); err != nil {
		return nil, err
	}
	if err := a.Store.SetLookChoices(out.Set, true); err != nil {
		return nil, err
	}
	for _, c := range out.Set {
		a.logLooksChoice(st, &LooksResult{Look: c.Look, Scope: r.Scope}, "apply from "+out.From.DisplayName())
	}
	if r.Added, err = a.looksLast(); err != nil {
		return out, err
	}
	err = a.looksAfter(ctx, r)
	out.mixed()
	return out, err
}

// mixed fills each applied item's Mixed from the state after the call.
func (r *LooksManyResult) mixed() {
	if r.State == nil {
		return
	}
	for i := range r.Items {
		if s := r.State.Looks[r.Items[i].Look]; s != nil && r.Items[i].Err == "" {
			r.Items[i].Mixed = s.Mixed
		}
	}
}

// LooksUndoMany removes the choices of every look of ids in one scope (21 §3.3). A look
// without a choice there is reported in Items; when none has one the call fails. The
// generated Mod is rebuilt once. dryRun writes nothing.
func (a *App) LooksUndoMany(ctx context.Context, ids []string, scope string, dryRun bool) (*LooksManyResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	sc, err := looksScope(st, scope)
	if err != nil {
		return nil, err
	}
	out := &LooksManyResult{LooksResult: &LooksResult{DryRun: dryRun, Scope: scopeName(scope), Plan: st.Plan.Name}}
	r := out.LooksResult
	stored, err := a.Store.LookChoices(sc)
	if err != nil {
		return nil, err
	}
	count := map[string]int{}
	for _, c := range stored {
		count[c.Look]++
	}
	for _, id := range looksIDs(ids) {
		it := LooksManyItem{Look: id, Removed: count[id]}
		if it.Removed == 0 {
			it.Err = fmt.Sprintf("no choice for the look %s in scope %s", id, r.Scope)
		} else {
			out.Done++
			r.Removed += it.Removed
		}
		out.Items = append(out.Items, it)
	}
	if out.Done == 0 {
		return nil, fmt.Errorf("no choice for these looks in scope %s (archo looks --filter chosen)", r.Scope)
	}
	if dryRun {
		ls, err := a.LooksState(ctx, st, nil)
		if err != nil {
			return nil, err
		}
		rows, _ := a.Store.LookChoices(store.LookScope(st.Plan.ID), store.LookScopeAll)
		for _, it := range out.Items {
			if it.Err == "" {
				rows = withRows(rows, nil, sc, it.Look, true)
			}
		}
		_, err = a.looksPreview(r, ls, rows)
		return out, err
	}
	for _, it := range out.Items {
		if it.Err != "" {
			continue
		}
		if _, err := a.Store.DeleteLookChoices(sc, it.Look); err != nil {
			return out, err
		}
		a.logLooksChoice(st, &LooksResult{Look: it.Look, Scope: r.Scope}, fmt.Sprintf("undo: %s removed", plural(it.Removed, "choice")))
	}
	return out, a.looksAfter(ctx, r)
}

// Would resolves one look as it would be with a hypothetical apply of the Mod at pos to
// slots (none: the entire look) in scope, on top of the stored choices: the prediction an
// interface shows before the player applies (21 §3.3). It reads and writes nothing. ok is
// false when that Mod is not in the plan or supplies none of the named slots.
func (ls *LooksState) Would(l *looks.Look, pos int, slots []string, scope string) (would *LookState, ok bool, err error) {
	st := ls.St
	sc, err := looksScope(st, scope)
	if err != nil {
		return nil, false, err
	}
	m := ls.Mods[pos]
	if m == nil {
		return nil, false, nil
	}
	var set []store.LookChoice
	if len(slots) == 0 {
		set = []store.LookChoice{{Scope: sc, Look: l.ID, Slot: looks.All, Lineage: m.LineageID}}
		for _, s := range textureSlots(l) {
			ok = ok || supplies(l, pos, s)
		}
	}
	for _, s := range slots {
		if s = strings.ToLower(strings.TrimSpace(s)); !looks.IsMask(s) && supplies(l, pos, s) {
			set = append(set, store.LookChoice{Scope: sc, Look: l.ID, Slot: s, Lineage: m.LineageID})
			ok = true
		}
	}
	if !ok {
		return nil, false, nil
	}
	var rows []store.LookChoice
	if cur := ls.Looks[l.ID]; cur != nil {
		for _, c := range cur.Choices {
			rows = append(rows, c.LookChoice)
		}
	}
	rows = withRows(rows, set, sc, l.ID, len(slots) == 0)
	return looksState(st, &looks.Index{ByID: map[string]*looks.Look{l.ID: l}}, rows).Looks[l.ID], true, nil
}
