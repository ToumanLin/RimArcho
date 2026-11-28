package env

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// GameFoldersIn finds RimWorld* folders that look like a game, in name order, and skips
// other folders, files and RimWorld* folders without a game (03 §3.2).
func TestGameFoldersInFindsRimWorldStar(t *testing.T) {
	root := t.TempDir()
	games, gog := filepath.Join(root, "Games"), filepath.Join(root, "GOG Games")
	write(t, filepath.Join(games, "RimWorld 1.7", "RimWorldWin64.exe"), "")
	write(t, filepath.Join(games, "RimWorld 1.7", "Version.txt"), "1.7.5014 rev1300\n")
	write(t, filepath.Join(games, "rimworld-old", "Data", "Core", "About", "About.xml"), "<x/>")
	write(t, filepath.Join(games, "RimWorld backup", "readme.txt"), "not a game")
	write(t, filepath.Join(games, "Factorio", "factorio.exe"), "")
	write(t, filepath.Join(games, "RimWorld.txt"), "a file")
	write(t, filepath.Join(gog, "RimWorld", "RimWorldWin64.exe"), "")
	got := GameFoldersIn([]string{games, gog, filepath.Join(root, "missing")})
	want := []string{filepath.Join(games, "RimWorld 1.7"), filepath.Join(games, "rimworld-old"), filepath.Join(gog, "RimWorld")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// the version comes from Version.txt, not the folder's name
	if v := ReadGameVersion(want[0]); v != "1.7.5014 rev1300" {
		t.Fatalf("version %q", v)
	}
}

// Off Windows the games folders are ~/Games and ~/GOG Games, when they exist; a folder
// found there is a direct-launch game for DetectAll.
func TestGameParentsHomeAndDetectAll(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "Games", "RimWorld 1.7", "RimWorldLinux"), "")
	if got := GameParents("linux", home); !reflect.DeepEqual(got, []string{filepath.Join(home, "Games")}) {
		t.Fatalf("parents %q", got)
	}
	if GameParents("darwin", home) != nil {
		t.Fatal("macOS has no games folders")
	}
	all := DetectAll(DetectOptions{SteamRoots: []string{}, Home: home, GOOS: "linux"})
	if len(all) != 1 || all[0].GameRoot != filepath.Join(home, "Games", "RimWorld 1.7") || all[0].Launch != "direct" {
		t.Fatalf("detected %+v", all)
	}
	libs, folders := Searched(DetectOptions{SteamRoots: []string{}, Home: home, GOOS: "linux"})
	if libs != 0 || !reflect.DeepEqual(folders, []string{filepath.Join(home, "Games")}) {
		t.Fatalf("searched %d %q", libs, folders)
	}
}

// Searched with explicit roots names their parents that exist, once.
func TestSearchedExplicitRoots(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Games"), 0o755)
	libs, folders := Searched(DetectOptions{SteamRoots: []string{}, ManualRoots: []string{filepath.Join(root, "Games", "RimWorld"), filepath.Join(root, "Games", "RimWorld 2"), filepath.Join(root, "nowhere", "RimWorld")}, Home: root, GOOS: "windows", SkipRegistry: true})
	if libs != 0 || !reflect.DeepEqual(folders, []string{filepath.Join(root, "Games")}) {
		t.Fatalf("searched %d %q", libs, folders)
	}
}

func TestCountModDirs(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"A", "B", ".git", "_Link_C"} {
		os.MkdirAll(filepath.Join(dir, n), 0o755)
	}
	write(t, filepath.Join(dir, "notes.txt"), "")
	if n := CountModDirs(dir); n != 2 {
		t.Fatalf("count %d", n)
	}
	if n := CountModDirs(filepath.Join(dir, "missing")); n != -1 {
		t.Fatalf("missing %d", n)
	}
}
