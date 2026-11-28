package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestFindScope: find names its Game Config, folds removed copies into one line, and marks a
// copy in another game's Mods/ folder, which a list file's packageId does not pick (09 §3).
func TestFindScope(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gone"), testkit.Mod{PackageID: "tester.gone", Name: "Bed Gone"})
	e.WriteMod(filepath.Join(e.Game, "Mods", "Here"), testkit.Mod{PackageID: "tester.here", Name: "Bed Here"})
	e.AddEnv(false)
	e.MustRun("scan")
	if err := os.RemoveAll(filepath.Join(e.Game, "Mods", "Gone")); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	other := filepath.Join(e.Root, "other-game")
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.WriteMod(filepath.Join(other, "Mods", "Far"), testkit.Mod{PackageID: "tester.far", Name: "Bed Far"})
	e.MustRun("env", "add", "other", "--game", other, "--user-data", e.UserData)
	e.App.EnvName = "other"
	e.MustRun("scan")
	e.App.EnvName = ""
	e.MustRun("scan")
	out := e.MustRun("find", "bed")
	if !strings.Contains(out, `found: 2 for "bed" (env:main)`) || !strings.Contains(out, "+1 removed copies (tombstones; archo find bed --all)") ||
		strings.Contains(out, "Bed Gone") {
		t.Fatalf("find folds tombstones:\n%s", out)
	}
	if !strings.Contains(out, "Bed Far") || !strings.Contains(out, "another game's Mods/: enable this reference to link it") {
		t.Fatalf("find marks another game's copy:\n%s", out)
	}
	// env list reads Version.txt: Steam updated the game since the last scan.
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.6.4999 rev1\n")
	if out := e.MustRun("env", "list"); !strings.Contains(out, "1.6.4999 rev1") {
		t.Fatalf("env list shows the stored version:\n%s", out)
	}
	if out := e.MustRun("find", "bed", "--all"); !strings.Contains(out, "Bed Gone") || !strings.Contains(out, "tombstone since") {
		t.Fatalf("find --all lists tombstones:\n%s", out)
	}
}
