package app_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// V2 (10 §6): three edits; going back two steps from the history, then forward one.
func TestHistorySteps(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	start := joined(active(t, e))
	e.MustRun("enable", "pkg:tester.b")
	e.MustRun("disable", "pkg:tester.a")
	e.MustRun("move", "pkg:tester.b", "--to", "1")
	h, err := e.App.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Undo) != 3 || h.Undo[0].Op != "move" || h.Undo[2].Op != "enable" || h.Undo[2].Steps != 3 {
		t.Fatalf("undo steps: %+v", h.Undo)
	}
	if len(h.Undo[1].Changes) != 1 || h.Undo[1].Changes[0].Kind != loadout.Removed {
		t.Fatalf("the disable step: %+v", h.Undo[1].Changes)
	}
	afterEnable := ""
	if _, err := e.App.StepTo(2); err != nil {
		t.Fatal(err)
	}
	afterEnable = joined(active(t, e))
	if !strings.Contains(afterEnable, "B") || !strings.Contains(afterEnable, "A") {
		t.Fatalf("two steps back (after enable B): %s", afterEnable)
	}
	h, _ = e.App.History()
	if len(h.Undo) != 1 || len(h.Redo) != 2 || h.Redo[0].Op != "disable" || h.Redo[0].Steps != -1 {
		t.Fatalf("after going back: undo %d redo %+v", len(h.Undo), h.Redo)
	}
	e.MustRun("redo", "2")
	if got := joined(active(t, e)); strings.Contains(got, ", A") || !strings.HasPrefix(got, "B") {
		t.Fatalf("redo 2: %s", got)
	}
	e.MustRun("undo", "3")
	if got := joined(active(t, e)); got != start {
		t.Fatalf("undo 3: %s, want %s", got, start)
	}
	out := e.MustRun("history")
	if !strings.Contains(out, "redo") || !strings.Contains(out, "versions") {
		t.Fatalf("history:\n%s", out)
	}
}
