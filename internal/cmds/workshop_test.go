package cmds_test

import (
	"context"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkshopResubscribeCommand(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.repair")
	e.AddEnv(false)
	e.MustRun("scan")
	calls := 0
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		calls++
		return "ITEM 8100 ok 1\nDONE " + args[0] + " 1\n", nil
	}
	out := e.MustRun("workshop", "resubscribe", "ws:8100", "--dry-run")
	for _, want := range []string{"would resubscribe", "recycle bin", "Steam re-downloads", "local edits", "8100"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if calls != 0 || strings.Contains(out, "warning: used by") {
		t.Fatalf("dry run: %s calls=%d", out, calls)
	}
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake")
	e.App.TrashFunc = os.RemoveAll
	out = e.MustRun("workshop", "resubscribe", "ws:8100")
	if calls != 3 || !strings.Contains(out, "resubscribed 1 Workshop item") {
		t.Fatalf("resubscribe: %s calls=%d", out, calls)
	}
}

func TestWorkshopCLIProgress(t *testing.T) {
	// Progress is a command-wide sink; keep this test out of the parallel group.
	e := testkit.New(t)
	e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.repair")
	e.AddEnv(false)
	e.MustRun("scan")
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake")
	e.App.TrashFunc = os.RemoveAll
	e.App.SteamWorker = func(_ string, args []string) (string, error) { return "ITEM 8100 ok 1\nDONE " + args[0] + " 1", nil }
	var lines []string
	old := cmds.Progress
	cmds.Progress = func(line string) { lines = append(lines, line) }
	defer func() { cmds.Progress = old }()
	e.App.SteamWorkerStream = func(_ context.Context, _ string, _ []string, line func(string)) error {
		line("PROGRESS 8100 0 0 5")
		line("PROGRESS 8100 45 100 21")
		line("PROGRESS 8100 46 100 21")
		if len(lines) != 2 || !strings.Contains(lines[0], "waiting for Steam") || !strings.Contains(lines[1], "40%") {
			t.Fatalf("live progress %v", lines)
		}
		line("PROGRESS 8100 100 100 5")
		line("ITEM 8100 ok 5")
		line("DONE watch 1")
		return nil
	}
	out := e.MustRun("workshop", "resubscribe", "ws:8100")
	if len(lines) != 4 || !strings.HasSuffix(lines[3], "downloaded") || !strings.Contains(out, "downloads have finished") {
		t.Fatalf("%v %s", lines, out)
	}
}
