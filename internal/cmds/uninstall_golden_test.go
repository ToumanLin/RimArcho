package cmds_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

var sizeRe = regexp.MustCompile(`\b\d+(\.\d+)? (B|KB|MB|GB)\b`)

// archo uninstall (02 §1, 13 §4.2): the dry run with the defaults and with every other
// answer, the refusals, the run, and every command refused after it.
func TestGoldenUninstall(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony", Files: map[string]string{"Assemblies/0Harmony.dll": "MZ"}})
	e.AddWorkshop("1001", testkit.Sample("tester.a", "A"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a")
	e.AddEnv(true)
	e.WriteMod(filepath.Join(e.SteamCMD, "8888"), testkit.Sample("tester.cmd", "CmdOnly"))
	e.MustRun("scan")
	e.MustRun("enable", ref(e, "CmdOnly"))
	e.MustRun("pin", ref(e, "A"))
	e.MustRun("save")
	e.WriteFile(filepath.Join(e.Game, "Mods", "_Archo_mine", "About", "About.xml"), "<ModMetaData><packageId>me.mine</packageId></ModMetaData>")
	old := e.OtherGame("old", "1.5.4409 rev1118")
	e.WriteFile(filepath.Join(old, "steamapps", "appmanifest_294100.acf"), `"AppState" {}`)
	e.MustRun("env", "add", "old", "--game", old, "--saves", "own", "--launch", "direct")
	e.WriteFile(filepath.Join(e.Home, "userdata", "old", "Saves", "colony.rws"), "a save")
	e.WriteFile(filepath.Join(e.Home, "packs", "P1", "pack.json"), `{"format":1,"name":"P1","entries":[]}`)
	local := filepath.Join(e.Root, "LocalAppData", "RimArcho")
	e.WriteFile(filepath.Join(local, "monitor", "ArchoMonitor-0.9.0", "About", "About.xml"), "<ModMetaData/>")
	e.WriteFile(filepath.Join(local, "steamcmd", deps.SteamCMDExecutable()), "MZ")
	e.WriteFile(filepath.Join(local, "todds", "todds.exe"), "MZ")
	for _, n := range []string{"archo.exe", "RimArcho.exe", "README.md", "fonts/legacy.ttf"} {
		e.WriteFile(filepath.Join(e.Root, "Program", n), "x")
	}
	e.App.UninstallExe = filepath.Join(e.Root, "Program", "archo.exe")
	e.MustRun("env", "use", "main")
	e.App.TrashFunc = func(p string) error { return os.RemoveAll(p) }
	runGolden(t, "uninstall", e, []step{
		cmd("uninstall"),
		cmd("uninstall", "--keep-locked", "--remove-packs", "--remove-saves", "--remove-games", "--restore-modsconfig-backup"),
		cmd("uninstall", "--keep-locked", "--locked-to", "old", "--remove-games"),
		cmd("uninstall", "--yes", "--dry-run"),
		cmd("uninstall", "--yes"),
		cmd("status"),
	}, func(s string) string { return sizeRe.ReplaceAllString(s, "<size>") })
}
