package cmds_test

import (
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
	if calls != 2 || !strings.Contains(out, "resubscribed 1 Workshop item") {
		t.Fatalf("resubscribe: %s calls=%d", out, calls)
	}
}
