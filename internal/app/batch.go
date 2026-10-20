package app

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
)

// Batches and block moves for the TUI's pick sessions (06 §12.2): the edits of one session
// apply at once, as previews, and become one undo step when the session ends; a cancelled
// session puts the list and the staging list back as they were.

// Batch is an open pick session.
type Batch struct {
	a       *App
	envID   string
	entries string // the workspace list when it began
	staged  string // the staging list when it began
	lastID  int64  // the newest undo step then
}

// BeginBatch starts a session on the current environment.
func (a *App) BeginBatch() (*Batch, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	return &Batch{a: a, envID: e.ID, entries: st.List.Encode(), staged: a.Store.Meta(stageKey(e.ID)), lastID: a.Store.LastStepID(e.ID, "undo")}, nil
}

// Commit ends the session: its undo steps become one, which restores the list as it was
// when the session began.
func (b *Batch) Commit(op string) error {
	if b.a.Store.LastStepID(b.envID, "undo") == b.lastID {
		return nil // nothing was edited
	}
	if err := b.a.Store.DeleteStepsAfter(b.envID, "undo", b.lastID); err != nil {
		return err
	}
	st, err := b.a.Loadout()
	if err != nil {
		return err
	}
	if st.List.Encode() == b.entries {
		return nil // the edits cancelled out
	}
	return b.a.Store.PushStep(b.envID, "undo", b.entries, op)
}

// Revert ends the session and undoes everything it did, staging included.
func (b *Batch) Revert() error {
	st, err := b.a.Loadout()
	if err != nil {
		return err
	}
	if err := b.a.Store.DeleteStepsAfter(b.envID, "undo", b.lastID); err != nil {
		return err
	}
	if st.List.Encode() != b.entries {
		st.WS.Entries, st.WS.Updated = b.entries, b.a.Now().UTC()
		if err := b.a.Store.SetWorkspace(st.WS); err != nil {
			return err
		}
	}
	return b.a.Store.SetMeta(stageKey(b.envID), b.staged)
}

// MoveBlock moves enabled instances to a 1-based position as one block, keeping their
// order in the list; at counts in the list without them.
func (a *App) MoveBlock(ids []string, at int) (*EditResult, error) {
	return a.edit("move", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		in := map[string]bool{}
		for _, id := range ids {
			if l.Index(id) < 0 {
				return nil, nil, fmt.Errorf("%s is not enabled", st.Ix.Ref(id))
			}
			in[id] = true
		}
		var block, rest loadout.List
		for _, e := range l {
			if in[e.Instance] {
				block = append(block, e)
			} else {
				rest = append(rest, e)
			}
		}
		i := max(0, min(at-1, len(rest)))
		out := append(append(append(loadout.List{}, rest[:i]...), block...), rest[i:]...)
		return out, nil, nil
	})
}

// SuggestAt is the 1-based position where instances that are not enabled best go in the
// list: right after the last enabled mod they must load after, unless that breaks a "load
// before"; at the end when nothing constrains them (06 §12.2).
func (a *App) SuggestAt(ids []string) (int, error) {
	st, err := a.Loadout()
	if err != nil {
		return 0, err
	}
	// Enabled ones are taken out first (06 §13.7: m suggests a place for them too); the
	// position counts in the list without them, as MoveBlock and Enable expect.
	moving := map[string]bool{}
	for _, id := range ids {
		moving[id] = true
	}
	var l loadout.List
	for _, e := range st.List {
		if !moving[e.Instance] {
			l = append(l, e)
		}
	}
	n := len(l)
	for _, id := range ids {
		if l.Index(id) < 0 {
			l = append(l, loadout.Entry{Instance: id})
		}
	}
	if len(l) == n {
		return n + 1, nil
	}
	in, err := a.SortInput(st, l)
	if err != nil {
		return 0, err
	}
	r := order.Sort(in)
	lo, hi, constrained := 0, n, false
	for _, e := range r.Edges {
		if e.C.Kind == order.KindOfficial && e.C.Field == "official-first" {
			continue // every mod comes after the official content anyway
		}
		switch {
		case e.From < n && e.To >= n: // a new one loads after an enabled one
			lo, constrained = max(lo, e.From+1), true
		case e.From >= n && e.To < n: // a new one loads before an enabled one
			hi, constrained = min(hi, e.To), true
		}
	}
	if !constrained {
		return n + 1, nil
	}
	if lo == 0 && hi < n {
		return hi + 1, nil // only "before" constraints: just before the first of them
	}
	if hi < lo {
		return lo + 1, nil // cannot satisfy both: after what they need
	}
	return lo + 1, nil
}
