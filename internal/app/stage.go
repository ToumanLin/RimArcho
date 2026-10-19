package app

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// Staging (06 §5.4): one list per environment, kept by RimArcho and never written to the
// game. A mod staged from the Loadout remembers where it was, so it can be put back:
// its 1-based position then and the instance just before it (the anchor).
//
// Staging holds only mods that are not enabled: a staged mod that is enabled again (by
// undo, enable, a checkout) leaves staging when the list is next read.

// StageItem is one staged mod.
type StageItem struct {
	Instance string `json:"i"`
	Pos      int    `json:"p,omitempty"` // 0: it was not enabled when staged
	After    string `json:"a,omitempty"` // the instance before it then; "" at the top
}

func stageKey(envID string) string { return "stage:" + envID }

// Staged returns the staging list of the current environment, in staging order.
func (a *App) Staged() ([]StageItem, *LoadoutState, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, nil, err
	}
	var items []StageItem
	if v := a.Store.Meta(stageKey(st.Ix.Env.ID)); v != "" {
		if err := json.Unmarshal([]byte(v), &items); err != nil {
			return nil, nil, fmt.Errorf("staging list unreadable: %w", err)
		}
	}
	out := items[:0]
	for _, it := range items {
		if st.Ix.ByID[it.Instance] != nil && st.List.Index(it.Instance) < 0 {
			out = append(out, it)
		}
	}
	return out, st, nil
}

func (a *App) setStaged(envID string, items []StageItem) error {
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return a.Store.SetMeta(stageKey(envID), string(b))
}

// Stage puts mods into staging; enabled ones leave the Loadout (one undo step) and
// remember their place.
func (a *App) Stage(refs []string) (*EditResult, error) {
	items, st, err := a.Staged()
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	next := map[string]string{} // anchor → the mod staged earlier right after it (latest wins)
	for _, it := range items {
		in[it.Instance] = true
		if it.Pos > 0 {
			next[it.After] = it.Instance
		}
	}
	// anchor: the list predecessor, moved forward past mods staged earlier that stood
	// between it and this mod (they come back in between).
	anchor := func(pred string) string {
		for hops := 0; hops <= len(items); hops++ {
			n, ok := next[pred]
			if !ok {
				break
			}
			pred = n
		}
		return pred
	}
	var add []StageItem
	var notes []string
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		if in[m.ID] {
			notes = append(notes, st.Ix.Ref(m.ID)+" is already staged")
			continue
		}
		in[m.ID] = true
		it := StageItem{Instance: m.ID}
		if i := st.List.Index(m.ID); i >= 0 {
			it.Pos = i + 1
			pred := "" // the top of the list, or a missing entry: falls back to Pos
			if i > 0 {
				pred = st.List[i-1].Instance
			}
			it.After = anchor(pred)
		}
		add = append(add, it)
	}
	r, err := a.edit("stage", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		for _, it := range add {
			if i := l.Index(it.Instance); i >= 0 {
				l = remove(l, i)
			}
		}
		return l, notes, nil
	})
	if err != nil {
		return nil, err
	}
	return r, a.setStaged(st.Ix.Env.ID, append(items, add...))
}

// Unstage takes mods out of staging without enabling them.
func (a *App) Unstage(refs []string) ([]string, error) {
	items, st, err := a.Staged()
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
	var keep []StageItem
	var gone []string
	for _, it := range items {
		if drop[it.Instance] {
			gone = append(gone, it.Instance)
			continue
		}
		keep = append(keep, it)
	}
	return gone, a.setStaged(st.Ix.Env.ID, keep)
}

// Restore enables staged mods again (all of them when refs is empty): each goes back after
// its anchor, following anchors through mods that are still staged, else to the position
// it had. With a placement (one mod only) it goes there instead. One undo step.
func (a *App) Restore(refs []string, p Placement) (*EditResult, error) {
	items, st, err := a.Staged()
	if err != nil {
		return nil, err
	}
	pick := map[string]bool{}
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		pick[m.ID] = true
	}
	var todo []StageItem
	var keep []StageItem
	for _, it := range items {
		if len(refs) == 0 || pick[it.Instance] {
			todo = append(todo, it)
		} else {
			keep = append(keep, it)
		}
	}
	if len(refs) > 0 && len(todo) != len(pick) {
		return nil, fmt.Errorf("not staged: %d of %d", len(pick)-len(todo), len(pick))
	}
	if p.set() && len(todo) != 1 {
		return nil, fmt.Errorf("a position needs exactly one staged mod")
	}
	// Order: a mod's anchor first when it is restored in the same call (positions of mods
	// staged at different times are not comparable); otherwise by position; mods staged from
	// outside the Loadout last, in staging order.
	sort.SliceStable(todo, func(i, j int) bool {
		pi, pj := todo[i].Pos, todo[j].Pos
		if pi == 0 || pj == 0 {
			return pj == 0 && pi != 0
		}
		return pi < pj
	})
	anchorOf := map[string]string{}
	for _, it := range items {
		anchorOf[it.Instance] = it.After
	}
	byID := map[string]StageItem{}
	for _, it := range todo {
		byID[it.Instance] = it
	}
	ordered := make([]StageItem, 0, len(todo))
	done := map[string]bool{}
	var visit func(it StageItem)
	visit = func(it StageItem) {
		if done[it.Instance] {
			return
		}
		done[it.Instance] = true // set before the anchor: a cycle cannot recurse forever
		if anc, ok := byID[it.After]; ok {
			visit(anc)
		}
		ordered = append(ordered, it)
	}
	for _, it := range todo {
		visit(it)
	}
	todo = ordered
	r, err := a.edit("restore", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		for _, it := range todo {
			m := st.Ix.ByID[it.Instance]
			if why := whyNotLoadable(st, m); why != "" {
				return nil, nil, fmt.Errorf("cannot enable %s: %s", st.Ix.Ref(m.ID), why)
			}
			at := len(l)
			switch {
			case p.set():
				var err error
				if at, err = p.target(st, l, len(l)); err != nil {
					return nil, nil, err
				}
			case it.Pos > 0:
				at = min(it.Pos-1, len(l))
				// The bound stops a cycle of anchors (possible after edits between stagings).
				for anc, hops := it.After, 0; anc != "" && hops <= len(items); anc, hops = anchorOf[anc], hops+1 {
					if i := l.Index(anc); i >= 0 {
						at = i + 1
						break
					}
				}
			}
			l = insert(l, at, loadout.Entry{Instance: it.Instance})
		}
		return l, nil, nil
	})
	if err != nil {
		return nil, err
	}
	return r, a.setStaged(st.Ix.Env.ID, keep)
}
