package app

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The history area (08 §14, 10 §6): the unsaved steps of the workspace (the undo and redo
// stacks) and the saved versions, so the player sees what u takes back and can jump.

// HistoryStep is one workspace edit.
type HistoryStep struct {
	Op      string
	Changes []loadout.Change // what the step did
	// Steps reaches the state this row stands for: n > 0 undoes n edits (the state before
	// this step), n < 0 redoes -n edits (the state after it).
	Steps int
}

// History is the workspace's past and future edits and the saved versions.
type History struct {
	Redo     []HistoryStep    // the next redo first
	Undo     []HistoryStep    // the latest edit first
	Versions []*store.Version // newest first
}

// History reads the undo and redo stacks and the versions.
func (a *App) History() (*History, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	h := &History{}
	undo, err := a.Store.Steps(e.ID, "undo")
	if err != nil {
		return nil, err
	}
	after := st.List
	for i, s := range undo {
		before := loadout.Decode(s.Entries)
		h.Undo = append(h.Undo, HistoryStep{Op: s.Op, Changes: loadout.Diff(before, after, st.LineageOf), Steps: i + 1})
		after = before
	}
	redo, err := a.Store.Steps(e.ID, "redo")
	if err != nil {
		return nil, err
	}
	before := st.List
	for i, s := range redo {
		next := loadout.Decode(s.Entries)
		h.Redo = append(h.Redo, HistoryStep{Op: s.Op, Changes: loadout.Diff(before, next, st.LineageOf), Steps: -(i + 1)})
		before = next
	}
	vs, err := a.Store.Versions(e.ID)
	if err != nil {
		return nil, err
	}
	h.Versions = vs
	return h, nil
}

// StepTo undoes (n > 0) or redoes (n < 0) |n| edits as one move through the history.
func (a *App) StepTo(n int) (*EditResult, error) {
	if n == 0 {
		return nil, fmt.Errorf("already there")
	}
	redo := n < 0
	if redo {
		n = -n
	}
	var first *EditResult
	var last *EditResult
	for i := 0; i < n; i++ {
		r, _, err := a.UndoRedo(redo)
		if err != nil {
			if last != nil {
				break // fewer steps than asked: stop at the end of the stack
			}
			return nil, err
		}
		if first == nil {
			first = r
		}
		last = r
	}
	last.Before = first.Before
	last.Changes = loadout.Diff(first.Before, last.State.List, last.State.LineageOf)
	return last, nil
}
