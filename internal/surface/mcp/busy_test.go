package mcp_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestReadToolsWhileLocked: while another process holds the launch lock (a test run, which
// holds it for minutes), read-only tools still answer from the stored lists, and a tool that
// needs the lock says it is busy (14 §1 "Busy").
func TestReadToolsWhileLocked(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	cs := connect(t, e)
	// The analysis exists, as in a session that ran get_harmony before the test run started;
	// a new analysis syncs the list and, without a test run's busy.json, needs the lock (14 §1
	// "Busy"; TestFirstAnalysisDuringTestRun covers the test run).
	for _, tool := range []string{"status", "get_harmony"} {
		if out, isErr := call(t, cs, tool, nil); isErr {
			t.Fatalf("%s: %s", tool, out)
		}
	}
	lock, err := app.LockWorkspace(e.App.Home, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	for _, c := range []struct {
		tool string
		args map[string]any
	}{{"status", nil}, {"find_mods", map[string]any{"query": "a"}}, {"get_harmony", nil}, {"list_problems", nil}, {"plan_history", nil}} {
		if out, isErr := call(t, cs, c.tool, c.args); isErr || strings.Contains(out, "busy:") {
			t.Errorf("%s while the lock is held: %s", c.tool, out)
		}
	}
	out, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "undo"})
	if !isErr || !strings.Contains(out, "busy: another launch") {
		t.Fatalf("an edit while the lock is held: %s", out)
	}
}
