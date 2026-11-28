package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestImportLinksOtherCopies: a list's packageId that the game sees no copy of picks a copy in
// another game's Mods/ or a SteamCMD folder when that copy lists the game's version or is
// the only one, and the import says where it is linked from (07 §7.1 "Copies the game does
// not see"). Nothing is copied or moved.
func TestImportLinksOtherCopies(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(true)
	other := e.OtherGame("old-game", "1.5.4409 rev1118")
	e.WriteMod(filepath.Join(other, "Mods", "Far"), testkit.Mod{PackageID: "tester.far", Name: "Far Lamp", Supported: []string{"1.5", "1.6"}})
	e.WriteMod(filepath.Join(other, "Mods", "Old"), testkit.Mod{PackageID: "tester.old", Name: "Old Lamp", Supported: []string{"1.5"}})
	e.WriteMod(filepath.Join(other, "Mods", "Two"), testkit.Mod{PackageID: "tester.two", Name: "Two Lamp", Supported: []string{"1.5"}})
	e.WriteMod(filepath.Join(e.SteamCMD, "3001"), testkit.Mod{PackageID: "tester.two", Name: "Two Lamp", Supported: []string{"1.4"}})
	e.WriteMod(filepath.Join(other, "Mods", "Both"), testkit.Mod{PackageID: "tester.both", Name: "Both Lamp", Supported: []string{"1.5"}})
	e.WriteMod(filepath.Join(e.SteamCMD, "3002"), testkit.Mod{PackageID: "tester.both", Name: "Both Lamp", Supported: []string{"1.6"}})
	e.MustRun("env", "add", "old", "--game", other, "--user-data", e.UserData)
	e.App.EnvName = "old"
	e.MustRun("scan")
	e.App.EnvName = ""
	e.MustRun("scan")
	list := filepath.Join(e.Root, "list.txt")
	e.WriteFile(list, "ludeon.rimworld\ntester.far\ntester.old\ntester.two\ntester.both\n")

	out := e.MustRun("plan", "new", "imp", "--from", list)
	for _, want := range []string{
		"linked from other folders: 3 (a save or launch links them into Mods/)",
		"Far Lamp (tester.far) linked from old's Mods folder",
		"Old Lamp (tester.old) linked from old's Mods folder",     // the only copy, though it lists 1.5 only
		"Both Lamp (tester.both) linked from the SteamCMD folder", // the copy that lists 1.6
		"not installed: 1: tester.two",                            // two copies, neither lists 1.6
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan new lacks %q:\n%s", want, out)
		}
	}
	e.MustRun("plan", "use", "imp")
	if out := e.MustRun("problems", "--all"); !strings.Contains(out, "does not list 1.6") {
		t.Fatalf("the version finding flags the 1.5-only copy:\n%s", out)
	}
	out = e.MustRun("loadout", "import", list)
	if !strings.Contains(out, "Far Lamp (tester.far) linked from old's Mods folder") {
		t.Fatalf("loadout import names the linked copies:\n%s", out)
	}
	// Nothing is copied or linked before a save or launch.
	if ents, err := os.ReadDir(filepath.Join(e.Game, "Mods")); err != nil || len(ents) != 0 {
		t.Fatalf("the game's Mods/ after the import: %v %v", ents, err)
	}
}
