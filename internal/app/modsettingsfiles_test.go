package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// fakeRecycle records what goes to the recycle bin and removes it, never touching the host's.
func fakeRecycle(e *testkit.Env) *[]string {
	var got []string
	e.App.TrashFunc = func(p string) error { got = append(got, filepath.Base(p)); return os.Remove(p) }
	return &got
}

func configEnv(t *testing.T) (*testkit.Env, string) {
	e := loadoutEnv(t)
	scanNow(t, e)
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock><ModSettings><on>True</on></ModSettings></SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Mod_B_BMod.xml"), "<SettingsBlock/>")
	e.WriteFile(filepath.Join(cfg, "Mod_Gone_Old_GMod.xml"), "<SettingsBlock/>")
	e.WriteFile(filepath.Join(cfg, "Prefs.xml"), "<PrefsData/>")
	return e, cfg
}

// 16 §2.9 S1: every file of Config/ and where it stands.
func TestModSettingsFiles(t *testing.T) {
	t.Parallel()
	e, _ := configEnv(t)
	cf, err := e.App.ModSettingsFiles()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range cf.Files {
		got[f.File] = f.Status + "/" + f.Class
	}
	want := map[string]string{"Mod_1001_AMod.xml": "listed/AMod", "Mod_B_BMod.xml": "unlisted/BMod",
		"Mod_Gone_Old_GMod.xml": "orphan/", "Prefs.xml": "game/"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	out := e.MustRun("mod-settings", "files", "--filter", "orphans")
	if !strings.Contains(out, "Mod_Gone_Old_GMod.xml") || strings.Contains(out, "Prefs.xml") || !strings.Contains(out, "1 not installed") {
		t.Fatalf("files:\n%s", out)
	}
}

// 16 §2.9 C1: orphans go to the recycle bin; the game's own files and other folders never.
func TestRecycleModSettings(t *testing.T) {
	t.Parallel()
	e, cfg := configEnv(t)
	got := fakeRecycle(e)
	if out := e.MustRun("mod-settings", "recycle", "--orphans", "--dry-run"); !strings.Contains(out, "would move 1 settings file") || len(*got) != 0 {
		t.Fatalf("dry run %v:\n%s", *got, out)
	}
	if out, code := e.Run("mod-settings", "recycle", "Prefs.xml"); code == 0 || !strings.Contains(out, "not a Mod settings file") {
		t.Fatalf("Prefs.xml: %d\n%s", code, out)
	}
	if err := e.App.ToTrash(filepath.Join(e.Root, "Mod_x_Y.xml")); err == nil {
		e.WriteFile(filepath.Join(e.Root, "Mod_x_Y.xml"), "")
		t.Fatal("a Mod_*.xml outside Config/ was allowed")
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 9} }
	if out, code := e.Run("mod-settings", "recycle", "--orphans"); code == 0 || !strings.Contains(out, "the game is running") {
		t.Fatalf("running: %d\n%s", code, out)
	}
	e.App.GameProbe = nil
	out := e.MustRun("mod-settings", "recycle", "--orphans")
	if !strings.Contains(out, "moved 1 settings file") || strings.Join(*got, ",") != "Mod_Gone_Old_GMod.xml" {
		t.Fatalf("recycle %v:\n%s", *got, out)
	}
	if _, err := os.Stat(filepath.Join(cfg, "Prefs.xml")); err != nil {
		t.Fatal("Prefs.xml went")
	}
	// several at once
	out = e.MustRun("mod-settings", "recycle", "Mod_B_BMod.xml", "Mod_1001_AMod.xml")
	if !strings.Contains(out, "moved 2 settings files") {
		t.Fatalf("two:\n%s", out)
	}
}

// 16 §2.9 C2: reset recycles the Mod's own files.
func TestResetModSettings(t *testing.T) {
	t.Parallel()
	e, cfg := configEnv(t)
	got := fakeRecycle(e)
	out := e.MustRun("mod-settings", "reset", "pkg:tester.a")
	if !strings.Contains(out, "moved 1 settings file") || strings.Join(*got, ",") != "Mod_1001_AMod.xml" {
		t.Fatalf("reset %v:\n%s", *got, out)
	}
	if _, err := os.Stat(filepath.Join(cfg, "Mod_B_BMod.xml")); err != nil {
		t.Fatal("another Mod's file went")
	}
	if out, code := e.Run("mod-settings", "reset", "pkg:tester.a"); code == 0 || !strings.Contains(out, "already uses its defaults") {
		t.Fatalf("again: %d\n%s", code, out)
	}
}

// 16 §2.9 P2: copy from another Game Config, matched by package across folder names; a file
// here is recycled before it is replaced.
func TestCopyModSettingsFrom(t *testing.T) {
	t.Parallel()
	e, cfg := configEnv(t)
	got := fakeRecycle(e)
	other := filepath.Join(e.Root, "other-data")
	e.MustRun("env", "add", "other", "--game", e.OtherGame("other-game", "1.6.4633 rev1261"), "--user-data", other)
	e.MustRun("env", "use", "main")
	// the other Game Config saw A under a SteamCMD copy's link name
	e.WriteMod(filepath.Join(e.SteamCMD, "1001"), testkit.Sample("tester.a", "A"))
	scanNow(t, e)
	e.WriteFile(filepath.Join(other, "Config", "Mod__Archo_steamcmd_1001_AMod.xml"), "<SettingsBlock>other</SettingsBlock>")
	e.WriteFile(filepath.Join(other, "Config", "Mod_1001_Extra.xml"), "<SettingsBlock>extra</SettingsBlock>")

	out := e.MustRun("mod-settings", "copy-from", "other", "--dry-run")
	if !strings.Contains(out, "would copy 2 files, 1 replacing") || !strings.Contains(out, "Mod__Archo_steamcmd_1001_AMod.xml → Mod_1001_AMod.xml  (replaces the file here)") ||
		!strings.Contains(out, "Mod_1001_Extra.xml → Mod_1001_Extra.xml  (new)") || len(*got) != 0 {
		t.Fatalf("dry run:\n%s", out)
	}
	out = e.MustRun("mod-settings", "copy-from", "other", refOf(t, e, "A", source.Workshop))
	if !strings.Contains(out, "copied 2 files") || strings.Join(*got, ",") != "Mod_1001_AMod.xml" {
		t.Fatalf("copy %v:\n%s", *got, out)
	}
	if s := readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")); s != "<SettingsBlock>other</SettingsBlock>" {
		t.Fatalf("copied %q", s)
	}
	if out := e.MustRun("mod-settings", "copy-from", "other"); !strings.Contains(out, "copied 0 files") || !strings.Contains(out, "already the same") {
		t.Fatalf("again:\n%s", out)
	}
	if out, code := e.Run("mod-settings", "copy-from", "nope"); code == 0 || !strings.Contains(out, "no Game Config named") {
		t.Fatalf("unknown: %d\n%s", code, out)
	}
}
