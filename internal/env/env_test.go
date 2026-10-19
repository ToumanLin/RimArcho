package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	root := t.TempDir()
	steam := filepath.Join(root, "Steam")
	lib := filepath.Join(root, "Lib2")
	game := filepath.Join(lib, "steamapps", "common", "RimWorld")
	write(t, filepath.Join(game, "RimWorld.exe"), "") // 1.7 executable name
	write(t, filepath.Join(game, "Version.txt"), "\xef\xbb\xbf1.7.4982 rev1280\r\n")
	os.MkdirAll(filepath.Join(lib, "steamapps", "workshop", "content", "294100"), 0o755)
	write(t, filepath.Join(steam, "steamapps", "libraryfolders.vdf"), `"libraryfolders" { "0" { "path" "`+
		strings.ReplaceAll(steam, `\`, `\\`)+`" } "1" { "path" "`+strings.ReplaceAll(lib, `\`, `\\`)+`" } }`)
	home := filepath.Join(root, "home")
	ud := filepath.Join(home, "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
	os.MkdirAll(ud, 0o755)

	d := Detect(DetectOptions{SteamRoots: []string{steam}, ManualRoots: []string{}, Home: home, GOOS: "windows", SkipRegistry: true})
	if d.GameRoot != game || d.Launch != "steam" || d.GameVersion != "1.7.4982 rev1280" || d.UserDataRoot != ud {
		t.Fatalf("detected %+v", d)
	}
	if d.WorkshopRoot != filepath.Join(lib, "steamapps", "workshop", "content", "294100") {
		t.Fatalf("workshop %q", d.WorkshopRoot)
	}
	// Manual install fallback.
	manual := filepath.Join(root, "Games", "RimWorld")
	write(t, filepath.Join(manual, "RimWorldWin64.exe"), "")
	d = Detect(DetectOptions{SteamRoots: []string{}, ManualRoots: []string{manual}, Home: home, GOOS: "windows", SkipRegistry: true})
	if d.GameRoot != manual || d.Launch != "direct" {
		t.Fatalf("manual %+v", d)
	}
}

func TestMajorMinorAndPlayerLog(t *testing.T) {
	if MajorMinor("1.6.4633 rev1261") != "1.6" || MajorMinor("") != "" {
		t.Fatal("MajorMinor")
	}
	if !VersionLess("1.9", "1.10") || VersionLess("1.7", "1.6") {
		t.Fatal("VersionLess")
	}
	ud := t.TempDir()
	write(t, filepath.Join(ud, "Player.log"), "old")
	write(t, filepath.Join(ud, "Logs", "Player.log"), "new")
	if got := PlayerLog(ud, "1.6.1"); got != filepath.Join(ud, "Player.log") {
		t.Fatalf("1.6: %s", got)
	}
	if got := PlayerLog(ud, "1.7.1"); got != filepath.Join(ud, "Logs", "Player.log") {
		t.Fatalf("1.7: %s", got)
	}
	future := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(ud, "Player.log"), future, future)
	if got := PlayerLog(ud, ""); got != filepath.Join(ud, "Player.log") {
		t.Fatalf("unknown version should pick newest: %s", got)
	}
}

func TestReadModsConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ModsConfig.xml")
	write(t, p, "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?><ModsConfigData><version>1.6.4871 rev591</version><activeMods><li>Ludeon.RimWorld</li><li>brrainz.harmony</li></activeMods></ModsConfigData>")
	mc, err := ReadModsConfig(p)
	if err != nil || mc.Version != "1.6.4871 rev591" || len(mc.ActiveMods) != 2 || mc.ActiveMods[0] != "ludeon.rimworld" {
		t.Fatalf("%+v %v", mc, err)
	}
}
