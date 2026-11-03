package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 08 §3: a Mod played from its Workshop folder and then from a SteamCMD copy (an _Archo_
// link) keeps its settings; going back never replaces the Workshop file.
func TestSettingsFollowDeployedCopy(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "1001"), testkit.Sample("tester.a", "A"))
	scanNow(t, e)
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>workshop</SettingsBlock>")
	carried := filepath.Join(cfg, "Mod__Archo_steamcmd_1001_AMod.xml")

	e.MustRun("swap", refOf(t, e, "A", source.Workshop), refOf(t, e, "A", source.SteamCMD))
	out := e.MustRun("save", "--dry-run")
	if !strings.Contains(out, "would copy 1 settings file") || !strings.Contains(out, "A: Mod_1001_AMod.xml → Mod__Archo_steamcmd_1001_AMod.xml") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(carried); !os.IsNotExist(err) {
		t.Fatal("the dry run copied")
	}
	if out := e.MustRun("save"); !strings.Contains(out, "copied 1 settings file") {
		t.Fatalf("save:\n%s", out)
	}
	if got := readFile(t, carried); got != "<SettingsBlock>workshop</SettingsBlock>" {
		t.Fatalf("carried %q", got)
	}

	// Played as the copy, its settings change; back on the Workshop folder, its own file stays.
	e.WriteFile(carried, "<SettingsBlock>copy</SettingsBlock>")
	e.MustRun("swap", refOf(t, e, "A", source.SteamCMD), refOf(t, e, "A", source.Workshop))
	if out := e.MustRun("save"); strings.Contains(out, "Mod settings") {
		t.Fatalf("copied over an existing file:\n%s", out)
	}
	if got := readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")); got != "<SettingsBlock>workshop</SettingsBlock>" {
		t.Fatalf("the Workshop file changed: %q", got)
	}
	e.MustRun("env", "set", "main", "--launch", "direct")
	if out := e.MustRun("launch", "--dry-run"); strings.Contains(out, "Mod settings") {
		t.Fatalf("launch preview:\n%s", out)
	}
}

// 08 §5.3 step 4: a pack's links read their own names; the first play copies each Mod's
// settings to them, and HugsLib's; later plays keep the pack's own.
func TestPackSettingsFollowTheMod(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("save")
	e.MustRun("pack", "build", "first")
	dir := filepath.Join(e.Home, "packs", "first")
	var p app.Pack
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "pack.json"))), &p); err != nil {
		t.Fatal(err)
	}
	folder := ""
	for _, pe := range p.Entries {
		if pe.PackageID == "tester.a" {
			folder = pe.Folder
		}
	}
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>env</SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Mod_Versions_VMod.xml"), "<SettingsBlock>versions</SettingsBlock>")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml"), "<settings/>")
	to := app.SettingsFile("!ArchoPack_first_"+folder, "AMod")
	packCfg := filepath.Join(dir, "userdata", "Config")

	out := e.MustRun("pack", "launch", "first", "--dry-run")
	if !strings.Contains(out, "would copy 1 settings file") || !strings.Contains(out, "Mod_1001_AMod.xml → "+to) {
		t.Fatalf("dry run (%s):\n%s", to, out)
	}
	out = e.MustRun("pack", "launch", "first")
	if !strings.Contains(out, "copied 1 settings file") || !strings.Contains(out, "HugsLib/ModSettings.xml") {
		t.Fatalf("launch:\n%s", out)
	}
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>env</SettingsBlock>" {
		t.Fatalf("pack copy %q", got)
	}
	// Versions is used from Mods/ (the same content): its file keeps its name.
	if got := readFile(t, filepath.Join(packCfg, "Mod_Versions_VMod.xml")); got != "<SettingsBlock>versions</SettingsBlock>" {
		t.Fatalf("reused copy's settings %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "userdata", "HugsLib", "ModSettings.xml")); got != "<settings/>" {
		t.Fatalf("HugsLib %q", got)
	}

	e.WriteFile(filepath.Join(packCfg, to), "<SettingsBlock>pack</SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>env later</SettingsBlock>")
	if out := e.MustRun("pack", "launch", "first"); strings.Contains(out, "Mod settings") || strings.Contains(out, "HugsLib") {
		t.Fatalf("second launch copied again:\n%s", out)
	}
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>pack</SettingsBlock>" {
		t.Fatalf("the pack's settings were replaced: %q", got)
	}
}
