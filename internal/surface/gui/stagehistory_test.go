package gui

import (
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
	"testing"
)

// The real GUI's API compositions must create exactly one complete undo step.
func TestStageAPIHistory(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	board := api.Board()
	var alpha, beta string
	for _, r := range board.Cols[view.Active] {
		if r.Name == "Alpha" {
			alpha = r.ID
		}
	}
	for _, r := range board.Cols[view.Inactive] {
		if r.Name == "Beta" {
			beta = r.ID
		}
	}
	if alpha == "" || beta == "" {
		t.Fatal("fixture rows missing")
	}
	st, _ := e.App.Loadout()
	state := func() [2]string {
		t.Helper()
		ws, err := e.App.Store.Workspace(st.Plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		return [2]string{ws.Entries, e.App.Store.Meta(store.StageKey(st.Plan.ID))}
	}
	round := func(action func() error) {
		t.Helper()
		before := state()
		depth := e.App.Store.StepCount(st.Plan.ID, "undo")
		if err := action(); err != nil {
			t.Fatal(err)
		}
		after := state()
		if after == before || e.App.Store.StepCount(st.Plan.ID, "undo") != depth+1 {
			t.Fatal("API action must be one changed step")
		}
		if _, err := api.Undo(); err != nil {
			t.Fatal(err)
		}
		if state() != before {
			t.Fatalf("API undo: %v want %v", state(), before)
		}
		if _, err := api.Redo(); err != nil {
			t.Fatal(err)
		}
		if state() != after {
			t.Fatalf("API redo: %v want %v", state(), after)
		}
	}
	round(func() error { _, err := api.Stage(int(view.Active), []string{alpha}); return err })
	round(func() error { _, err := api.Stage(int(view.Inactive), []string{beta}); return err })
	round(func() error { _, err := api.Unstage([]string{beta}); return err })
	round(func() error { _, err := api.Place(int(view.Staging), []string{alpha}, 2); return err })
	// The same row can be disabled again without recreating its old staging entry.
	if _, err := api.Toggle(int(view.Active), []string{alpha}, nil); err != nil {
		t.Fatal(err)
	}
	staged, _, err := e.App.Staged()
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 0 {
		t.Fatalf("API enabled-then-disabled ghost: %+v", staged)
	}
}
