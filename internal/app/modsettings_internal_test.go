package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The names follow Verse.LoadedModManager.GetSettingsFilename and GenText.SanitizeFilename
// (1.6.4871, 1.7.4995): the whole name is split on invalid characters, empty parts dropped.
func TestSettingsFileMatchesGame(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ folder, class, want string }{
		{"1234567890", "ExampleMod", "Mod_1234567890_ExampleMod.xml"},
		{"_1234567890_", "ExampleMod", "Mod__1234567890__ExampleMod.xml"},
		{"_Archo_steamcmd_1234567890", "ExampleMod", "Mod__Archo_steamcmd_1234567890_ExampleMod.xml"},
		{"!ArchoPack_Colony_1234567890", "ExampleMod", "Mod__ArchoPack_Colony_1234567890_ExampleMod.xml"},
		{"My Mod {v2}!", "M", "Mod_My Mod _v2__M.xml"},
		{"a@#b", "Settings_UI", "Mod_a_b_Settings_UI.xml"},
	} {
		if got := SettingsFile(c.folder, c.class); got != c.want {
			t.Errorf("SettingsFile(%q, %q) = %q, want %q", c.folder, c.class, got, c.want)
		}
		if got, want := settingsStem(c.folder)+c.class+".xml", c.want; got != want {
			t.Errorf("stem of %q: %q, want %q", c.folder, got, want)
		}
	}
}

func writeSettings(t *testing.T, dir, name, body string, mod time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A Mod seen under a new folder gets each Mod class's newest file from its other folders;
// a file of a longer known folder is not taken, an existing file is never replaced and the
// originals stay.
func TestCarryModSettings(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	old, newer := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	writeSettings(t, cfg, "Mod_1234_ExampleMod.xml", "workshop", old)
	writeSettings(t, cfg, "Mod__1234__ExampleMod.xml", "local copy", newer)
	writeSettings(t, cfg, "Mod_1234_Other_Thing.xml", "second class", old)
	writeSettings(t, cfg, "Mod_1234_5_Foo.xml", "another mod's", old)
	writeSettings(t, cfg, "Mod_1234_Kept.xml", "old kept", old)
	writeSettings(t, cfg, "Mod__Archo_steamcmd_1234_Kept.xml", "current kept", old)
	// 1234_5 is another Mod of the library, not in the list.
	job := settingsJob{read: []string{cfg}, have: cfg, write: cfg, known: []string{"1234_5"}, mods: []settingsMod{
		{name: "Example", folder: "_Archo_steamcmd_1234", old: []string{"1234", "_1234_", "_Archo_steamcmd_1234"}},
	}}
	want := []SettingsCarry{
		{Mod: "Example", From: "Mod__1234__ExampleMod.xml", To: "Mod__Archo_steamcmd_1234_ExampleMod.xml", src: cfg},
		{Mod: "Example", From: "Mod_1234_Other_Thing.xml", To: "Mod__Archo_steamcmd_1234_Other_Thing.xml", src: cfg},
	}
	if got := planModSettings(job); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan %+v\nwant %+v", got, want)
	}
	if got := carryModSettings(job, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("carried %+v", got)
	}
	for name, body := range map[string]string{
		"Mod__Archo_steamcmd_1234_ExampleMod.xml":  "local copy",
		"Mod__Archo_steamcmd_1234_Other_Thing.xml": "second class",
		"Mod__Archo_steamcmd_1234_Kept.xml":        "current kept",
		"Mod_1234_ExampleMod.xml":                  "workshop",
		"Mod__1234__ExampleMod.xml":                "local copy",
	} {
		if got := readText(t, filepath.Join(cfg, name)); got != body {
			t.Errorf("%s: %q, want %q", name, got, body)
		}
	}
	if fi, err := os.Stat(filepath.Join(cfg, "Mod__Archo_steamcmd_1234_ExampleMod.xml")); err != nil || !fi.ModTime().Equal(newer) {
		t.Errorf("copy keeps its source's time: %v, %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "Mod__Archo_steamcmd_1234_5_Foo.xml")); !os.IsNotExist(err) {
		t.Error("took the file of folder 1234_5")
	}
	if ents, _ := os.ReadDir(cfg); len(ents) != 8 {
		t.Errorf("%d files, want 8 (no temporary files left)", len(ents))
	}
	if got := carryModSettings(job, nil); len(got) != 0 {
		t.Fatalf("second run copied again: %+v", got)
	}
}

// A pack reads its own Config first, then the environment's. A test run checks the
// environment's Config and writes its own, refreshing what it copied before.
func TestCarryModSettingsDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pack, envCfg, test := filepath.Join(root, "pack"), filepath.Join(root, "env"), filepath.Join(root, "test")
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	writeSettings(t, pack, "Mod_1001_A.xml", "pack's", at)
	writeSettings(t, envCfg, "Mod_1001_A.xml", "env's newer", at.Add(time.Hour))
	writeSettings(t, envCfg, "Mod_1001_B.xml", "env only", at)
	mods := []settingsMod{{name: "A", folder: "!ArchoPack_p_1001", old: []string{"1001"}}}
	got := carryModSettings(settingsJob{read: []string{pack, envCfg}, have: pack, write: pack, mods: mods}, nil)
	if len(got) != 2 || got[0].Err != "" || got[1].Err != "" {
		t.Fatalf("pack carried %+v", got)
	}
	if a, b := readText(t, filepath.Join(pack, "Mod__ArchoPack_p_1001_A.xml")), readText(t, filepath.Join(pack, "Mod__ArchoPack_p_1001_B.xml")); a != "pack's" || b != "env only" {
		t.Fatalf("pack files %q, %q", a, b)
	}

	writeSettings(t, test, "Mod__ArchoPack_p_1001_A.xml", "stale", at)
	got = carryModSettings(settingsJob{read: []string{envCfg}, have: envCfg, write: test, mods: mods}, nil)
	if len(got) != 2 {
		t.Fatalf("test carried %+v", got)
	}
	if a := readText(t, filepath.Join(test, "Mod__ArchoPack_p_1001_A.xml")); a != "env's newer" {
		t.Fatalf("test copy %q", a)
	}
	if _, err := os.Stat(filepath.Join(envCfg, "Mod__ArchoPack_p_1001_A.xml")); !os.IsNotExist(err) {
		t.Fatal("a test run wrote the environment's Config")
	}
}

// HugsLib's settings reach a pack only while it has none.
func TestCopyHugsLib(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from, to := filepath.Join(root, "env"), filepath.Join(root, "pack")
	writeSettings(t, filepath.Join(from, "HugsLib"), "ModSettings.xml", "<settings/>", time.Now())
	got, err := copyHugsLib(from, to)
	if err != nil || !reflect.DeepEqual(got, []string{"HugsLib/ModSettings.xml"}) {
		t.Fatalf("copied %v, %v", got, err)
	}
	writeSettings(t, filepath.Join(to, "HugsLib"), "ModSettings.xml", "<settings>pack</settings>", time.Now())
	if got, err := copyHugsLib(from, to); err != nil || len(got) != 0 {
		t.Fatalf("second copy %v, %v", got, err)
	}
	if s := readText(t, filepath.Join(to, "HugsLib", "ModSettings.xml")); s != "<settings>pack</settings>" {
		t.Fatalf("replaced the pack's: %q", s)
	}
}
