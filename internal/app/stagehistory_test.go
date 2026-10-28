package app_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type stagePayload struct{ Entries, Stage string }

func stageState(t *testing.T, e *testkit.Env) stagePayload {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ws, err := e.App.Store.Workspace(st.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.WS.Entries != ws.Entries {
		t.Fatalf("cached workspace diverged from DB: %s / %s", st.WS.Entries, ws.Entries)
	}
	return stagePayload{ws.Entries, e.App.Store.Meta(store.StageKey(st.Plan.ID))}
}
func expectStageState(t *testing.T, e *testkit.Env, want stagePayload) {
	t.Helper()
	if got := stageState(t, e); got != want {
		t.Fatalf("payload got %+v, want %+v", got, want)
	}
}

func TestStageHistoryRoundTrip(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	original := stageState(t, e)
	states := []stagePayload{original}
	commands := [][]string{
		{"stage", "add", "pkg:tester.m10", "pkg:tester.m11"},
		{"stage", "add", "pkg:tester.m12"},
		{"stage", "add", "pkg:tester.m31"}, // staging-only add must create a step
		{"stage", "drop", "pkg:tester.m11"},
		{"stage", "restore", "pkg:tester.m12"},
		{"stage", "restore", "--all"},
	}
	st, _ := e.App.Loadout()
	depth := e.App.Store.StepCount(st.Plan.ID, "undo")
	for i, cmd := range commands {
		e.MustRun(cmd...)
		states = append(states, stageState(t, e))
		if got := e.App.Store.StepCount(st.Plan.ID, "undo"); got != depth+i+1 {
			t.Fatalf("%v created %d steps", cmd, got-depth-i)
		}
		if i == 1 {
			items, st, err := e.App.Staged()
			if err != nil {
				t.Fatal(err)
			}
			expected := []app.StageItem{
				{Instance: ids(t, e, "pkg:tester.m10")[0], Pos: 11, After: ids(t, e, "pkg:tester.m09")[0]},
				{Instance: ids(t, e, "pkg:tester.m11")[0], Pos: 12, After: ids(t, e, "pkg:tester.m10")[0]},
				{Instance: ids(t, e, "pkg:tester.m12")[0], Pos: 11, After: ids(t, e, "pkg:tester.m11")[0]},
			}
			if !reflect.DeepEqual(items, expected) {
				t.Fatalf("anchors/positions %+v want %+v (plan %d)", items, expected, st.Plan.ID)
			}
		}
	}
	for i := len(states) - 2; i >= 0; i-- {
		e.MustRun("undo")
		expectStageState(t, e, states[i])
	}
	for i := 1; i < len(states); i++ {
		e.MustRun("redo")
		expectStageState(t, e, states[i])
	}
}

func TestStageUndoThenDisableHasNoGhost(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	e.MustRun("stage", "add", "pkg:tester.m10")
	e.MustRun("undo")
	e.MustRun("disable", "pkg:tester.m10")
	if got := stagedNames(t, e); got != "" {
		t.Fatalf("ghost staging: %s", got)
	}
	if raw := stageState(t, e).Stage; raw != "" {
		t.Fatalf("ghost persisted: %s", raw)
	}
}

func TestStageReenablePersistsAndRestores(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"enable", "checkout", "import"} {
		t.Run(action, func(t *testing.T) {
			e := stageEnv(t)
			e.MustRun("stage", "add", "pkg:tester.m10", "pkg:tester.m11")
			staged := stageState(t, e)
			switch action {
			case "enable":
				e.MustRun("enable", "pkg:tester.m10")
			case "checkout":
				e.MustRun("loadout", "checkout", "v1")
			case "import":
				if _, err := e.App.ApplyImport(&app.ImportedList{Tokens: []string{"ludeon.rimworld", "tester.m10"}}, "test", false); err != nil {
					t.Fatal(err)
				}
			}
			after := stageState(t, e)
			var items []app.StageItem
			if err := json.Unmarshal([]byte(after.Stage), &items); err != nil {
				t.Fatal(err)
			}
			for _, it := range items {
				if it.Instance == ids(t, e, "pkg:tester.m10")[0] {
					t.Fatal("enabled mod persisted in staging")
				}
			}
			e.MustRun("undo")
			expectStageState(t, e, staged)
			e.MustRun("redo")
			expectStageState(t, e, after)
			e.MustRun("disable", "pkg:tester.m10")
			want := "M11"
			if action == "checkout" {
				want = ""
			}
			if got := stagedNames(t, e); got != want {
				t.Fatalf("stale stage membership: %q, want %q", got, want)
			}
		})
	}
}

func TestStageLegacyHistory(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	original := stageState(t, e)
	e.MustRun("stage", "add", "pkg:tester.m10", "pkg:tester.m31")
	legacyAfter := stageState(t, e)
	st, _ := e.App.Loadout()
	// Simulate the pre-migration entries-only step, with no staging payload.
	if _, err := e.App.Store.DB.Exec(`UPDATE workspace_undo SET staging=NULL WHERE plan_id=?`, st.Plan.ID); err != nil {
		t.Fatal(err)
	}
	e.MustRun("undo")
	if stageState(t, e).Entries != original.Entries || stagedNames(t, e) != "M31" {
		t.Fatal("legacy undo lost unrelated staging or entries")
	}
	e.MustRun("redo")
	expectStageState(t, e, legacyAfter)
	e.MustRun("undo")
	e.MustRun("disable", "pkg:tester.m10")
	if stagedNames(t, e) != "M31" {
		t.Fatal("legacy undo retained a hidden ghost")
	}
	e.MustRun("undo")
	e.MustRun("redo")
	if stagedNames(t, e) != "M31" {
		t.Fatal("legacy round trip lost preparation item")
	}
}

func TestStageHistoryAtomicFailure(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	before := stageState(t, e)
	st, _ := e.App.Loadout()
	depth := e.App.Store.StepCount(st.Plan.ID, "undo")
	// Fail the final metadata write after entries/history have been written.
	if _, err := e.App.Store.DB.Exec(`CREATE TRIGGER reject_stage BEFORE INSERT ON meta WHEN NEW.key LIKE 'stage:%' BEGIN SELECT RAISE(ABORT,'stage write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Stage([]string{"pkg:tester.m10"}); err == nil {
		t.Fatal("stage unexpectedly succeeded")
	}
	expectStageState(t, e, before)
	if e.App.Store.StepCount(st.Plan.ID, "undo") != depth {
		t.Fatal("failed stage changed history")
	}
	if _, err := e.App.Store.DB.Exec(`DROP TRIGGER reject_stage`); err != nil {
		t.Fatal(err)
	}
	e.MustRun("stage", "add", "pkg:tester.m10")
	after := stageState(t, e)
	if _, err := e.App.Store.DB.Exec(fmt.Sprintf(`CREATE TRIGGER reject_stage BEFORE DELETE ON meta WHEN OLD.key='stage:%d' BEGIN SELECT RAISE(ABORT,'stage restore failure'); END`, st.Plan.ID)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.App.UndoRedo(false); err == nil {
		t.Fatal("undo unexpectedly succeeded")
	}
	expectStageState(t, e, after)
	if e.App.Store.StepCount(st.Plan.ID, "undo") != depth+1 || e.App.Store.StepCount(st.Plan.ID, "redo") != 0 {
		t.Fatal("failed undo moved history")
	}
}

func TestStageBatchHistory(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	before := stageState(t, e)
	b, err := e.App.BeginBatch()
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("stage", "add", "pkg:tester.m31")
	e.MustRun("stage", "add", "pkg:tester.m32")
	if err := b.Commit("stage"); err != nil {
		t.Fatal(err)
	}
	after := stageState(t, e)
	e.MustRun("undo")
	expectStageState(t, e, before)
	e.MustRun("redo")
	expectStageState(t, e, after)
	b, err = e.App.BeginBatch()
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("stage", "drop", "pkg:tester.m31")
	if err := b.Revert(); err != nil {
		t.Fatal(err)
	}
	expectStageState(t, e, after)
}

func TestStageEmptyRepresentationsAndNoops(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	st, _ := e.App.Loadout()
	for _, raw := range []string{"", "null", "[]"} {
		if raw != "" {
			if err := e.App.Store.SetMeta(store.StageKey(st.Plan.ID), raw); err != nil {
				t.Fatal(err)
			}
		}
		before := stageState(t, e)
		depth := e.App.Store.StepCount(st.Plan.ID, "undo")
		e.MustRun("stage", "drop", "pkg:tester.m31")
		e.MustRun("stage", "restore", "--all")
		if e.App.Store.StepCount(st.Plan.ID, "undo") != depth {
			t.Fatal("no-op recorded an undo step")
		}
		expectStageState(t, e, before)
		e.MustRun("stage", "add", "pkg:tester.m31")
		e.MustRun("stage", "add", "pkg:tester.m31")
		if e.App.Store.StepCount(st.Plan.ID, "undo") != depth+1 {
			t.Fatal("duplicate stage added a step")
		}
		e.MustRun("undo")
		expectStageState(t, e, before)
	}
}
