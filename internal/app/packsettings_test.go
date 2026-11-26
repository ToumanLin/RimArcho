package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

const hugsAuthor = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<settings>\n  <tester.a>\n    <x>author</x>\n  </tester.a>\n  <other.mod>\n    <y>2</y>\n  </other.mod>\n</settings>\n"

// 10 §1 "settings", §3 step 4 (M1): a pack carries its author's Mod settings by default and
// seeds them, under its own folder names, on its first launch; the receiver's Mod settings
// are not taken then, the game's own files are. The player's later changes stay.
func TestPackCarriesSettings(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("save")
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>author</SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Prefs.xml"), "<PrefsData/>")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml"), hugsAuthor)

	if out := e.MustRun("pack", "build", "first"); !strings.Contains(out, "carries the settings of 1 Mod") {
		t.Fatalf("build:\n%s", out)
	}
	p, err := e.App.ReadPack("first")
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings == nil || len(p.Settings.Files) != 1 || p.Settings.Files[0].Class != "AMod" || p.Settings.Files[0].PackageID != "tester.a" ||
		len(p.Settings.HugsLib) != 1 || p.Settings.HugsLib[0].ID != "tester.a" || strings.Contains(p.Settings.HugsLib[0].XML, "other.mod") {
		t.Fatalf("settings: %+v", p.Settings)
	}
	folder := ""
	for _, pe := range p.Entries {
		if pe.PackageID == "tester.a" {
			folder = pe.Folder
		}
	}
	// the receiver has other settings for the same Mod
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>receiver</SettingsBlock>")
	to := app.SettingsFile("!ArchoPack_first_"+folder, "AMod")
	if out := e.MustRun("pack", "launch", "first", "--dry-run"); !strings.Contains(out, "A: the pack's settings → "+to) {
		t.Fatalf("dry run:\n%s", out)
	}
	e.MustRun("pack", "launch", "first")
	packCfg := filepath.Join(e.App.PacksDir(), "first", "userdata", "Config")
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>author</SettingsBlock>" {
		t.Fatalf("seeded %q", got)
	}
	if _, err := os.Stat(filepath.Join(packCfg, "Prefs.xml")); err != nil {
		t.Fatal("the game's own settings were not copied")
	}
	if _, err := os.Stat(filepath.Join(packCfg, "Mod_1001_AMod.xml")); err == nil {
		t.Fatal("the receiver's Mod options were copied")
	}
	hugs := readFile(t, filepath.Join(e.App.PacksDir(), "first", "userdata", "HugsLib", "ModSettings.xml"))
	if !strings.Contains(hugs, "<x>author</x>") || strings.Contains(hugs, "other.mod") {
		t.Fatalf("HugsLib:\n%s", hugs)
	}
	e.WriteFile(filepath.Join(packCfg, to), "<SettingsBlock>played</SettingsBlock>")
	os.Remove(filepath.Join(packCfg, "Prefs.xml"))
	e.MustRun("pack", "launch", "first")
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>played</SettingsBlock>" {
		t.Fatalf("the player's change was replaced: %q", got)
	}

	if out := e.MustRun("pack", "build", "bare", "--no-settings"); !strings.Contains(out, "none carried") {
		t.Fatalf("--no-settings:\n%s", out)
	}
	if q, err := e.App.ReadPack("bare"); err != nil || q.Settings != nil {
		t.Fatalf("--no-settings: %+v %v", q, err)
	}
}

// 10 §2, §4 (U1): a pack keeps its id; a new revision replaces its Mods in place and keeps its
// saves; importing a newer revision is offered with a diff, applied with --update; an older
// or the same revision is refused.
func TestPackRevisionUpdate(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("pack", "build", "first")
	rev1 := filepath.Join(e.Root, "rev1.archopack")
	e.MustRun("pack", "export", "first", rev1)
	p1, _ := e.App.ReadPack("first")
	dir := filepath.Join(e.App.PacksDir(), "first")
	save := filepath.Join(dir, "userdata", "Saves", "colony.rws")
	e.WriteFile(save, "<savegame/>")

	e.MustRun("disable", "pkg:tester.versions")
	out := e.MustRun("pack", "build", "first", "--update")
	if !strings.Contains(out, "revision") || !strings.Contains(out, "removed") || !strings.Contains(out, "Versions") {
		t.Fatalf("--update:\n%s", out)
	}
	p2, err := e.App.ReadPack("first")
	if err != nil || p2.ID != p1.ID || p2.Revision != 2 || len(p2.Entries) != len(p1.Entries)-1 {
		t.Fatalf("revision 2: %+v %v", p2, err)
	}
	if readFile(t, save) != "<savegame/>" {
		t.Fatal("the update lost the pack's save")
	}
	if _, err := os.Stat(dir + ".archo-old"); err == nil {
		t.Fatal("the previous revision was left beside the pack")
	}
	rev2 := filepath.Join(e.Root, "rev2.archopack")
	e.MustRun("pack", "export", "first", rev2)

	// the receiver has revision 1 under another name
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	e.MustRun("pack", "import", rev1)
	recv := filepath.Join(e.App.PacksDir(), "first")
	if err := os.Rename(recv, filepath.Join(e.App.PacksDir(), "mine")); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(e.App.PacksDir(), "mine")
	e.WriteFile(filepath.Join(mine, "userdata", "Saves", "colony.rws"), "<savegame>mine</savegame>")
	out = e.MustRun("pack", "import", rev2)
	if !strings.Contains(out, "revision 2, you have revision 1") || !strings.Contains(out, "removed") || !strings.Contains(out, "--update") {
		t.Fatalf("offer:\n%s", out)
	}
	if q, _ := e.App.ReadPack("mine"); q.Revision != 1 {
		t.Fatal("the offer changed the pack")
	}
	if up, err := e.App.PackUpdateOf(rev2); err != nil || up == nil || up.Name != "mine" || up.To != 2 || len(up.Diff.Removed) != 1 {
		t.Fatalf("PackUpdateOf: %+v %v", up, err)
	}
	out = e.MustRun("pack", "import", rev2, "--update")
	if !strings.Contains(out, "updated pack mine to revision 2") {
		t.Fatalf("update:\n%s", out)
	}
	q, err := e.App.ReadPack("mine")
	if err != nil || q.Revision != 2 || q.Name != "mine" {
		t.Fatalf("updated: %+v %v", q, err)
	}
	if readFile(t, filepath.Join(mine, "userdata", "Saves", "colony.rws")) != "<savegame>mine</savegame>" {
		t.Fatal("the update lost the receiver's save")
	}
	if _, err := os.Stat(filepath.Join(e.App.PacksDir(), "first")); err == nil {
		t.Fatal("the update made a second pack")
	}
	if problems, err := e.App.VerifyPack("mine"); err != nil || len(problems) > 0 {
		t.Fatalf("verify after update: %+v %v", problems, err)
	}
	if out, code := e.Run("pack", "import", rev1); code == 0 || !strings.Contains(out, "already is revision 2") {
		t.Fatalf("older revision: %d\n%s", code, out)
	}
}

// 07 §7.3 (X1): a RimArcho list carries Mod settings only when asked; applying them is a
// preview first, keeps files that are here unless replace, and recycles what it replaces.
func TestListSettingsExportApply(t *testing.T) {
	t.Parallel()
	e, cfg := configEnv(t)
	trashed := fakeRecycle(e)
	hugsFile := filepath.Join(e.UserData, "HugsLib", "ModSettings.xml")
	e.WriteFile(hugsFile, hugsAuthor)
	mineA := readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml"))

	plain := filepath.Join(e.Root, "plain.archolist.json")
	e.MustRun("loadout", "export", "--format", "archo", "--out", plain)
	var lp app.Pack
	if json.Unmarshal([]byte(readFile(t, plain)), &lp) != nil || lp.Format != 2 || lp.Settings != nil {
		t.Fatalf("plain list: %+v", lp)
	}
	if out, code := e.Run("loadout", "export", "--format", "text", "--settings"); code == 0 || !strings.Contains(out, "only a RimArcho list") {
		t.Fatalf("text with settings: %d\n%s", code, out)
	}
	file := filepath.Join(e.Root, "with.archolist.json")
	if out := e.MustRun("loadout", "export", "--format", "archo", "--settings", "--out", file); !strings.Contains(out, "Mod options") {
		t.Fatal(out)
	}
	if json.Unmarshal([]byte(readFile(t, file)), &lp) != nil || lp.Format != app.PackFormat || lp.Settings.Mods() != 1 {
		t.Fatalf("list with settings: %+v", lp)
	}
	// unlisted B's file is not the list's
	for _, f := range lp.Settings.Files {
		if f.PackageID != "tester.a" {
			t.Fatalf("carried %+v", f)
		}
	}

	// the receiver: other settings for A, no HugsLib file
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>other</SettingsBlock>")
	os.Remove(hugsFile)
	if out := e.MustRun("loadout", "import", file); !strings.Contains(out, "carries settings of 1 Mod; not applied") {
		t.Fatalf("import:\n%s", out)
	}
	out := e.MustRun("mod-settings", "apply", file, "--dry-run")
	if !strings.Contains(out, "would write 1 file") || !strings.Contains(out, "Mod_1001_AMod.xml  (kept") || !strings.Contains(out, "HugsLib tester.a)  (new)") {
		t.Fatalf("preview:\n%s", out)
	}
	e.MustRun("mod-settings", "apply", file)
	if readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")) != "<SettingsBlock>other</SettingsBlock>" {
		t.Fatal("a file here was replaced without --replace")
	}
	if h := readFile(t, hugsFile); !strings.Contains(h, "<x>author</x>") || strings.Contains(h, "other.mod") {
		t.Fatalf("HugsLib:\n%s", h)
	}
	e.MustRun("mod-settings", "apply", file, "tester.a", "--replace")
	if readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")) != mineA {
		t.Fatal("--replace did not write the list's file")
	}
	if strings.Join(*trashed, ",") != "Mod_1001_AMod.xml" {
		t.Fatalf("recycled %v", *trashed)
	}
	if out := e.MustRun("mod-settings", "apply", file, "--dry-run"); !strings.Contains(out, "would write 0 files") || !strings.Contains(out, "already the same") {
		t.Fatalf("again:\n%s", out)
	}
	// an older list without settings
	if out, code := e.Run("mod-settings", "apply", plain); code == 0 || !strings.Contains(out, "carries no Mod options") {
		t.Fatalf("plain apply: %d\n%s", code, out)
	}
}

// putHugsLib keeps other packs' bytes and the file's line ends.
func TestHugsLibMerge(t *testing.T) {
	t.Parallel()
	e, _ := configEnv(t)
	hugsFile := filepath.Join(e.UserData, "HugsLib", "ModSettings.xml")
	e.WriteFile(hugsFile, "<settings>\r\n  <tester.a>\r\n    <x>mine</x>\r\n  </tester.a>\r\n  <keep.me><z>1</z></keep.me>\r\n</settings>\r\n")
	s := &app.PackSettings{HugsLib: []app.PackHugsLib{{PackageID: "tester.a", ID: "tester.a", XML: "<tester.a><x>list</x></tester.a>"}}}
	trashed := fakeRecycle(e)
	if _, err := e.App.ApplyListSettings(s, nil, true, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, hugsFile)
	if got != "<settings>\r\n  <tester.a><x>list</x></tester.a>\r\n  <keep.me><z>1</z></keep.me>\r\n</settings>\r\n" {
		t.Fatalf("merged:\n%q", got)
	}
	if len(*trashed) != 1 || (*trashed)[0] != "ModSettings.xml" {
		t.Fatalf("recycled %v", *trashed)
	}
	bad := &app.PackSettings{HugsLib: []app.PackHugsLib{{PackageID: "tester.a", ID: "tester.a", XML: "<evil/>"}}}
	if _, err := e.App.ApplyListSettings(bad, nil, true, true); err == nil {
		t.Fatal("an element that is not the pack was accepted")
	}
	badFile := &app.PackSettings{Files: []app.PackSettingsFile{{PackageID: "tester.a", Class: "../x", XML: "<a/>"}}}
	if _, err := e.App.ApplyListSettings(badFile, nil, true, true); err == nil {
		t.Fatal("a class with a path was accepted")
	}
}

// 10 §3 (VM 2026-11-18): revision 1 was played, so its links are in Mods/; revision 2 drops a
// Mod and with it the folder one link points to. Launching revision 2 (or the plan) removes
// that link; the broken link of the pack's own does not refuse the launch.
func TestPackRevisionLaunchAfterDroppedMod(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("pack", "build", "first")
	e.MustRun("pack", "launch", "first")
	mods := filepath.Join(e.Game, "Mods")
	stale := ""
	for _, de := range readDir(t, mods) {
		if de == "!ArchoPack_first_1001" { // tester.a
			stale = de
		}
	}
	if stale == "" {
		t.Fatalf("no link of tester.a in %v", readDir(t, mods))
	}
	e.MustRun("disable", "pkg:tester.a")
	e.MustRun("pack", "build", "first", "--update")
	if out, code := e.Run("pack", "launch", "first"); code != 0 {
		t.Fatalf("revision 2 launch refused (%d):\n%s", code, out)
	}
	if _, err := os.Lstat(filepath.Join(mods, stale)); err == nil {
		t.Fatalf("the dropped Mod's link is still in Mods/: %s", stale)
	}
}

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, de := range ents {
		out = append(out, de.Name())
	}
	return out
}
