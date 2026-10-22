package mcp_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// I9: launch_game refuses while a game runs and names the environment; save_loadout still
// writes and says the game reads the list on its next start.
func TestLaunchGameRefusesWhileRunning(t *testing.T) {
	e := manageEnv(t)
	cs := connect(t, e)
	running := map[string]*app.GameRun{}
	e.App.GameProbe = func(root string) *app.GameRun { return running[env.PathKey(root)] }
	running[env.PathKey(e.Game)] = &app.GameRun{PID: 7, Exe: filepath.Join(e.Game, "RimWorldWin64.exe"), Started: e.Now().Add(-5 * time.Minute), ArgsKnown: true}

	before := e.ReadModsConfig()
	for _, args := range []map[string]any{{"dry_run": true}, nil} {
		out, isErr := call(t, cs, "launch_game", args)
		if !isErr || !strings.Contains(out, "a game is already running") || !strings.Contains(out, "environment main is running its list (pid 7") {
			t.Fatalf("launch_game %v while running: isError=%v\n%s", args, isErr, out)
		}
	}
	if e.ReadModsConfig() != before || len(e.Launched) != 0 {
		t.Fatal("a refused launch wrote or launched")
	}

	call(t, cs, "edit_loadout", map[string]any{"action": "disable", "refs": []string{ref(t, e, "Harmony")}})
	out, isErr := call(t, cs, "save_loadout", nil)
	if isErr || !strings.Contains(out, "it reads the new list on its next start") || strings.Contains(e.ReadModsConfig(), "brrainz.harmony") {
		t.Fatalf("save_loadout while running: isError=%v\n%s", isErr, out)
	}

	delete(running, env.PathKey(e.Game))
	if out, isErr := call(t, cs, "launch_game", nil); isErr || len(e.Launched) != 1 {
		t.Fatalf("launch_game with no game running: isError=%v, launched %v\n%s", isErr, e.Launched, out)
	}
}

// I10: plan use writes no game file; the last plan cannot be removed. (CLI and MCP give
// the same text for every plan action: TestGoldenManageTools.)
func TestManagePlans(t *testing.T) {
	e := manageEnv(t)
	cs := connect(t, e)
	if out, isErr := call(t, cs, "manage_plans", map[string]any{"action": "remove", "name": "default"}); !isErr {
		t.Fatalf("removed the last plan:\n%s", out)
	}
	before := e.ReadModsConfig()
	if out, isErr := call(t, cs, "manage_plans", map[string]any{"action": "new", "name": "empty"}); isErr {
		t.Fatal(out)
	}
	if out, isErr := call(t, cs, "manage_plans", map[string]any{"action": "use", "name": "empty"}); isErr || !strings.Contains(out, "current plan: plan:empty") {
		t.Fatalf("use:\n%s", out)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("plan use wrote ModsConfig.xml")
	}
	if out, isErr := call(t, cs, "manage_plans", map[string]any{"action": "remove", "name": "default"}); isErr || strings.Contains(e.MustRun("plan", "list"), "plan:default") {
		t.Fatalf("remove:\n%s", out)
	}
	if out, isErr := call(t, cs, "manage_plans", map[string]any{"action": "remove", "name": "empty"}); !isErr {
		t.Fatalf("removed the last plan:\n%s", out)
	}
}
