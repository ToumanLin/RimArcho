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

func packEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := loadoutEnv(t)
	v := testkit.Sample("tester.versions", "Versions")
	v.Files = map[string]string{
		"1.5/Defs/Old.xml":      "<Defs><ThingDef><defName>Old</defName></ThingDef></Defs>",
		"1.6/Defs/New.xml":      "<Defs><ThingDef><defName>New</defName></ThingDef></Defs>",
		"Common/Textures/x.txt": strings.Repeat("t", 100),
	}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Versions"), v)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	return e
}

// K2: a pack is not built while something needs fixing (here: a list entry not installed).
func TestPackBuildBlocked(t *testing.T) {
	e := packEnv(t)
	out, code := e.Run("pack", "build", "first")
	if code == 0 || !strings.Contains(out, "not installed: pkg:gone.mod") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.Home, "packs", "first")); err == nil {
		t.Fatal("a blocked build wrote a pack")
	}
}

// K1, K3, K4, K6: build without other versions' folders; a dry-run launch writes nothing; a
// launch links the copies, writes the pack's own list and leaves the environment's alone; a
// scan ignores the pack's links; saving the normal list removes them.
func TestPackBuildAndLaunch(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("save")
	envCfg := e.ReadModsConfig()

	if out := e.MustRun("pack", "build", "first", "--dry-run"); !strings.Contains(out, "Versions: 1.5") {
		t.Fatalf("dry run:\n%s", out)
	}
	e.MustRun("pack", "build", "first")
	dir := filepath.Join(e.Home, "packs", "first")
	var p app.Pack
	b, err := os.ReadFile(filepath.Join(dir, "pack.json"))
	if err != nil || json.Unmarshal(b, &p) != nil {
		t.Fatalf("pack.json: %v", err)
	}
	var ver *app.PackEntry
	for i := range p.Entries {
		if p.Entries[i].PackageID == "tester.versions" {
			ver = &p.Entries[i]
		}
	}
	if ver == nil || len(ver.TrimmedDir) != 1 || ver.TrimmedDir[0] != "1.5" || p.Trimmed == 0 {
		t.Fatalf("K1 trimmed: %+v", ver)
	}
	mod := filepath.Join(dir, "mods", ver.Folder)
	for path, want := range map[string]bool{"1.6/Defs/New.xml": true, "1.5": false, "Common/Textures/x.txt": true, "About/About.xml": true, ".rimarcho/origin.json": true} {
		if _, err := os.Stat(filepath.Join(mod, filepath.FromSlash(path))); (err == nil) != want {
			t.Errorf("K1: %s present=%v", path, err == nil)
		}
	}
	if !p.Entries[0].Official || p.Entries[0].PackageID != "ludeon.rimworld" {
		t.Fatalf("Core first and official: %+v", p.Entries[0])
	}

	mods := filepath.Join(e.Game, "Mods")
	out := e.MustRun("pack", "launch", "first", "--dry-run")
	if !strings.Contains(out, "+ !ArchoPack_first_") {
		t.Fatalf("K3:\n%s", out)
	}
	if links, _ := filepath.Glob(filepath.Join(mods, "!ArchoPack_*")); len(links) != 0 || len(e.Launched) != 0 {
		t.Fatal("K3: the dry run changed something")
	}

	e.MustRun("pack", "launch", "first")
	links, _ := filepath.Glob(filepath.Join(mods, "!ArchoPack_first_*"))
	if len(links) != len(p.Entries)-1 {
		t.Fatalf("K4: %d links for %d carried mods", len(links), len(p.Entries)-1)
	}
	for _, l := range links {
		if !source.IsLink(l) {
			t.Fatalf("%s is not a link", l)
		}
	}
	if e.ReadModsConfig() != envCfg {
		t.Fatal("K4: the environment's ModsConfig.xml changed")
	}
	packCfg, err := os.ReadFile(filepath.Join(dir, "userdata", "Config", "ModsConfig.xml"))
	if err != nil || !strings.Contains(string(packCfg), "<li>tester.versions</li>") || !strings.Contains(string(packCfg), "<li>tester.a</li>") {
		t.Fatalf("pack list: %v\n%s", err, packCfg)
	}
	if len(e.Launched) != 1 || !strings.Contains(e.Launched[0], "-savedatafolder="+filepath.Join(dir, "userdata")) {
		t.Fatalf("launched: %v", e.Launched)
	}
	if out := e.MustRun("pack", "list"); !strings.Contains(out, "first") || strings.Contains(out, "never played") {
		t.Fatalf("pack list:\n%s", out)
	}

	before := len(allMods(t, e))
	scanNow(t, e)
	if after := len(allMods(t, e)); after != before {
		t.Fatalf("K6: the scan picked up the pack's copies (%d → %d instances)", before, after)
	}
	e.MustRun("enable", "pkg:tester.b")
	if out := e.MustRun("save"); !strings.Contains(out, "- !ArchoPack_first_") {
		t.Fatalf("save does not remove the pack's links:\n%s", out)
	}
	if links, _ := filepath.Glob(filepath.Join(mods, "!ArchoPack_*")); len(links) != 0 {
		t.Fatalf("links left: %v", links)
	}
}

// K5 and export/import: a light pack compared with the installed mods; a full pack exported,
// removed and unpacked again.
func TestPackExportImport(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "first")
	light := filepath.Join(e.Root, "first.archopack.json")
	e.MustRun("pack", "export", "first", light, "--light")
	out := e.MustRun("pack", "import", light)
	if !strings.Contains(out, "same version installed: 2") || !strings.Contains(out, "not installed: 0") {
		t.Fatalf("K5:\n%s", out)
	}
	e.WriteFile(filepath.Join(e.Workshop, "1001", "Defs", "Changed.xml"), "<Defs><ThingDef><defName>C</defName></ThingDef></Defs>")
	scanNow(t, e)
	if out := e.MustRun("pack", "import", light, "--as-plan"); !strings.Contains(out, "another version installed: 1") || !strings.Contains(out, "now holds the pack's list") {
		t.Fatalf("K5 after an update:\n%s", out)
	}

	full := filepath.Join(e.Root, "first.archopack")
	e.MustRun("pack", "export", "first", full)
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, p); return os.RemoveAll(p) }
	e.MustRun("pack", "remove", "first")
	if len(trashed) != 1 {
		t.Fatalf("remove: %v", trashed)
	}
	e.MustRun("pack", "import", full)
	if _, err := os.Stat(filepath.Join(e.Home, "packs", "first", "mods")); err != nil {
		t.Fatal("unpacked pack has no mods")
	}
	if _, code := e.Run("pack", "import", full); code == 0 {
		t.Fatal("importing over an existing pack must be refused")
	}
}
