package app_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestIdenticalCheckoutIsNoOp(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"import", "external", "save", "checkout", "edited"} {
		t.Run(kind, func(t *testing.T) {
			e := loadoutEnv(t)
			scanNow(t, e)
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.App.Store.AddVersion(st.Plan.ID, &store.Version{Created: e.Now(), Source: "user", Entries: st.WS.Entries}); err != nil {
				t.Fatal(err)
			}
			ws := *st.WS
			ws.OriginVersion, ws.OriginKind, ws.Conflict = 2, kind, 7 // a conflict about another version
			if err := e.App.Store.SetWorkspace(&ws); err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.PushStep(st.Plan.ID, "redo", ws.Entries, "test redo"); err != nil {
				t.Fatal(err)
			}
			before, err := e.App.Store.Workspace(st.Plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			redo, err := e.App.Store.Steps(st.Plan.ID, "redo")
			if err != nil {
				t.Fatal(err)
			}
			stage := e.App.Store.Meta(store.StageKey(st.Plan.ID))
			game := e.ReadModsConfig()
			e.Advance(time.Hour)
			r, n, err := e.App.Checkout("v1")
			if err != nil {
				t.Fatal(err)
			}
			after, err := e.App.Store.Workspace(st.Plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(before, r.State.WS) {
				t.Errorf("identical checkout changed workspace: before=%+v after=%+v result=%+v", before, after, r.State.WS)
			}
			if n != 1 || len(r.Changes) != 0 {
				t.Errorf("checkout result: v%d %+v", n, r.Changes)
			}
			if e.App.Store.StepCount(st.Plan.ID, "undo") != 0 {
				t.Error("no-op created an undo step")
			}
			gotRedo, err := e.App.Store.Steps(st.Plan.ID, "redo")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(redo, gotRedo) {
				t.Error("no-op changed redo history")
			}
			if out := e.MustRun("loadout", "checkout", "v1"); !strings.Contains(out, "already matches v1; no changes") {
				t.Errorf("no-op checkout feedback: %s", out)
			}
			if stage != e.App.Store.Meta(store.StageKey(st.Plan.ID)) || game != e.ReadModsConfig() {
				t.Error("no-op changed staging or game config")
			}
		})
	}
}

// The list already is the external version it was kept from, so checking that version
// out resolves the conflict; nothing else changes and there is still nothing to undo.
func TestIdenticalCheckoutClearsItsConflict(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Store.AddVersion(st.Plan.ID, &store.Version{Created: e.Now(), Source: "external", Entries: st.WS.Entries}); err != nil {
		t.Fatal(err)
	}
	ws := *st.WS
	ws.OriginVersion, ws.OriginKind, ws.Conflict = 2, "edited", 1
	if err := e.App.Store.SetWorkspace(&ws); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.App.Checkout("v1"); err != nil {
		t.Fatal(err)
	}
	after, err := e.App.Store.Workspace(st.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Conflict != 0 || after.OriginVersion != 2 || after.OriginKind != "edited" || after.Entries != ws.Entries {
		t.Errorf("want only the conflict cleared: %+v", after)
	}
	if e.App.Store.StepCount(st.Plan.ID, "undo") != 0 {
		t.Error("no-op created an undo step")
	}
}

func TestCrossGameSortUndoPreservesBindings(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.a", "brrainz.harmony")
	scanNow(t, e)
	planID := mustPlan(t, e)
	original, err := e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	originalCore := loadout.Decode(original.Entries)[0].Instance
	g2 := secondGame(e, "Game15")
	e.MustRun("env", "add", "old", "--game", g2, "--saves", "own")
	e.MustRun("env", "use", "old")
	scanNow(t, e)
	e.MustRun("plan", "use", "default")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.WS.Entries != original.Entries || st.List[0].Instance == originalCore {
		t.Fatal("fixture must retain stored main Core and display old Core")
	}
	localCore := st.List[0].Instance
	beforeView := st.List.Clone()
	game := e.ReadModsConfig()
	sorted, err := e.App.Sort(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted.Changes) == 0 {
		t.Fatal("fixture sort must change order")
	}
	history, err := e.App.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Undo) != 1 || !reflect.DeepEqual(history.Undo[0].Changes, sorted.Changes) {
		t.Errorf("history shows binding changes as list edits: %+v", history.Undo)
	}
	sortedWS, err := e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	step, err := e.App.Store.PeekStep(planID, "undo")
	if err != nil {
		t.Fatal(err)
	}
	if step == nil || step.Entries != original.Entries {
		t.Errorf("undo snapshot lost stored bindings: %+v, want %s", step, original.Entries)
	}
	undone, op, err := e.App.UndoRedo(false)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Entries != original.Entries {
		t.Errorf("undo rebound list: %s, want %s", ws.Entries, original.Entries)
	}
	if op != "sort" || !undone.State.List.Equal(beforeView) || undone.State.List[0].Instance != localCore {
		t.Errorf("undo result must display current game's Core: %+v", undone.State.List)
	}
	if e.ReadModsConfig() != game {
		t.Error("sort/undo wrote game config")
	}
	// Redo from the other game must preserve the sorted payload, not rebind it again.
	e.MustRun("env", "use", "main")
	e.MustRun("plan", "use", "default")
	redone, _, err := e.App.UndoRedo(true)
	if err != nil {
		t.Fatal(err)
	}
	ws, err = e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Entries != sortedWS.Entries {
		t.Errorf("redo rebound list: %s, want %s", ws.Entries, sortedWS.Entries)
	}
	if redone.State.List[0].Instance != originalCore {
		t.Error("redo result is not localized for main")
	}
	e.MustRun("undo")
	ws, err = e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Entries != original.Entries {
		t.Error("second undo failed exact restoration")
	}
	// Picker batches must preserve the same stored bindings as individual edits.
	e.MustRun("env", "use", "old")
	for _, revert := range []bool{false, true} {
		b, err := e.App.BeginBatch()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.App.Sort(false); err != nil {
			t.Fatal(err)
		}
		if revert {
			err = b.Revert()
		} else {
			err = b.Commit("sort batch")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !revert {
			e.MustRun("undo")
		}
		ws, err = e.App.Store.Workspace(planID)
		if err != nil {
			t.Fatal(err)
		}
		if ws.Entries != original.Entries {
			t.Errorf("batch revert=%v changed bindings: %s", revert, ws.Entries)
		}
	}
}

// Undoing a checkout leaves a list that is not the checked-out version, so saving it is
// not recorded as restoring that version.
func TestUndoCheckoutDropsCheckoutOrigin(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "tester.a")
	e.MustRun("save")
	planID := mustPlan(t, e)
	if _, _, err := e.App.Checkout("v1"); err != nil {
		t.Fatal(err)
	}
	ws, err := e.App.Store.Workspace(planID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.OriginKind != "checkout" || ws.OriginVersion != 1 {
		t.Fatalf("after checkout: %+v", ws)
	}
	if _, _, err := e.App.UndoRedo(false); err != nil {
		t.Fatal(err)
	}
	if ws, err = e.App.Store.Workspace(planID); err != nil {
		t.Fatal(err)
	}
	if ws.OriginKind == "checkout" {
		t.Fatalf("after undo the workspace still claims the checkout: %+v", ws)
	}
	e.MustRun("save")
	vs, _ := e.App.Store.Versions(planID)
	if vs[0].Source != "user" || vs[0].Note != "" {
		t.Fatalf("save after undo recorded as %s %q", vs[0].Source, vs[0].Note)
	}
}
