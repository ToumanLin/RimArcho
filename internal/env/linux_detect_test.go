package env_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestLinuxDetection(t *testing.T) {
	for _, tc := range []struct {
		name, root                                     string
		proton, prefix, manual, external, mixed, alias bool
	}{
		{name: "steam", root: ".steam/steam"},
		{name: "root", root: ".steam/root"},
		{name: "local", root: ".local/share/Steam"},
		{name: "flatpak local", root: ".var/app/com.valvesoftware.Steam/.local/share/Steam"},
		{name: "flatpak data external", root: ".var/app/com.valvesoftware.Steam/data/Steam", external: true},
		{name: "snap", root: "snap/steam/common/.local/share/Steam"},
		{name: "proton external", root: ".steam/steam", proton: true, prefix: true, external: true},
		{name: "proton flatpak", root: ".var/app/com.valvesoftware.Steam/data/Steam", proton: true, prefix: true},
		{name: "windows without prefix", root: ".steam/steam", proton: true},
		{name: "native wins", root: ".steam/steam", prefix: true},
		{name: "manual games", root: "Games/RimWorld", manual: true},
		{name: "manual gog", root: "GOG Games/RimWorld", manual: true},
		{name: "mixed libraries", root: ".steam/steam", mixed: true},
		{name: "symlink roots", root: ".local/share/Steam", alias: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			home := filepath.Join(e.Root, "home")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
			root := filepath.Join(home, filepath.FromSlash(tc.root))
			lib := root
			if tc.external {
				lib = filepath.Join(home, "external-library")
				e.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), fmt.Sprintf(`"libraryfolders" { "1" { "path" %q } }`, lib))
			}
			game := filepath.Join(lib, "steamapps", "common", "RimWorld")
			if tc.manual {
				game = root
			}
			exe := "RimWorldLinux"
			if tc.proton {
				exe = "RimWorldWin64.exe"
			}
			e.WriteFile(filepath.Join(game, exe), "synthetic game")
			e.WriteFile(filepath.Join(game, "Version.txt"), "1.6.4633 rev1261")
			pfx := filepath.Join(lib, "steamapps", "compatdata", env.WorkshopAppID, "pfx")
			if tc.prefix {
				e.MkdirAll(pfx)
				if !tc.proton {
					e.WriteFile(filepath.Join(game, "RimWorldWin64.exe"), "")
				}
			}
			ud := filepath.Join(home, ".config", "unity3d", "Ludeon Studios", "RimWorld by Ludeon Studios")
			flatUD := filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".config", "unity3d", "Ludeon Studios", "RimWorld by Ludeon Studios")
			e.MkdirAll(ud) // Both exist: selection must follow this game, not search order.
			e.MkdirAll(flatUD)
			if tc.proton && tc.prefix {
				ud = filepath.Join(pfx, "drive_c", "users", "steamuser", "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
			} else if strings.HasPrefix(tc.root, ".var/") {
				ud = flatUD
			}
			e.WriteFile(filepath.Join(ud, "Player.log"), "synthetic log")
			ws := filepath.Join(lib, "steamapps", "workshop", "content", env.WorkshopAppID)
			if !tc.manual {
				e.MkdirAll(ws)
			}
			if tc.alias {
				if runtime.GOOS == "windows" {
					t.Skip("canonical Steam roots are a non-Windows behavior")
				}
				e.MkdirAll(filepath.Join(home, ".steam"))
				for _, name := range []string{"steam", "root"} {
					if err := os.Symlink(root, filepath.Join(home, ".steam", name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			opts := env.DetectOptions{GOOS: "linux", SkipRegistry: true}
			d := env.Detect(opts)
			launch := "steam"
			if tc.manual {
				launch = "direct"
			}
			if d.GameRoot != game || d.UserDataRoot != ud || d.Launch != launch || d.GameVersion != "1.6.4633 rev1261" {
				t.Fatalf("detected %+v; want game %s, data %s, launch %s", d, game, ud, launch)
			}
			if !tc.manual && (d.WorkshopRoot != ws || d.SteamLibrary != lib) {
				t.Fatalf("library/workshop: %+v", d)
			}
			if env.IsProtonGame(game) != (tc.proton && tc.prefix) || env.DefaultUserDataForGame(opts, game) != ud {
				t.Fatal("game default/kind disagrees")
			}
			if got := env.PlayerLog(d.UserDataRoot, d.GameVersion); got != filepath.Join(ud, "Player.log") {
				t.Fatalf("log: %s", got)
			}
			if tc.mixed {
				second := filepath.Join(home, "proton-library")
				secondGame := filepath.Join(second, "steamapps", "common", "RimWorld")
				secondUD := filepath.Join(second, "steamapps", "compatdata", env.WorkshopAppID, "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
				e.WriteFile(filepath.Join(secondGame, "RimWorldWin64.exe"), "")
				e.MkdirAll(secondUD)
				flatRoot := filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam")
				flatGame := filepath.Join(flatRoot, "steamapps", "common", "RimWorld")
				e.WriteFile(filepath.Join(flatGame, "RimWorldLinux"), "")
				e.WriteFile(filepath.Join(root, "config", "libraryfolders.vdf"), fmt.Sprintf(`"libraryfolders" { "1" %q }`, second))
				all := env.DetectAll(opts)
				if len(all) != 3 || all[0].UserDataRoot != ud || all[1].GameRoot != secondGame || all[1].UserDataRoot != secondUD || all[2].GameRoot != flatGame || all[2].UserDataRoot != flatUD {
					t.Fatalf("mixed: %+v", all)
				}
			} else if all := env.DetectAll(opts); len(all) != 1 || all[0].UserDataRoot != ud {
				t.Fatalf("all: %+v", all)
			}
			if tc.alias {
				libs := env.SteamLibraries([]string{filepath.Join(home, ".steam", "steam"), filepath.Join(home, ".steam", "root"), root})
				if !slices.Equal(libs, []string{root}) {
					t.Fatalf("aliases: %v", libs)
				}
			}
		})
	}
}

func TestSteamCommand(t *testing.T) {
	for _, mode := range []string{"path", "script", "flatpak", "missing"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			bin := filepath.Join(e.Root, "bin")
			e.MkdirAll(bin)
			t.Setenv("PATH", bin)
			t.Setenv("PATHEXT", ".EXE")
			root := filepath.Join(e.Root, ".steam", "steam")
			want := ""
			var args []string
			if mode == "path" || mode == "flatpak" {
				name := "steam"
				if mode == "flatpak" {
					name = "flatpak"
				}
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				want = filepath.Join(bin, name)
				if err := os.WriteFile(want, []byte("synthetic command, never executed"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "script" {
				want = filepath.Join(root, "steam.sh")
				e.WriteFile(want, "")
			}
			if mode == "flatpak" {
				root = filepath.Join(e.Root, ".var", "app", "com.valvesoftware.Steam", "data", "Steam")
				e.WriteFile(filepath.Join(root, "steam.sh"), "must not run outside sandbox")
				args = []string{"run", "com.valvesoftware.Steam"}
			}
			exe, gotArgs := env.SteamCommand(env.DetectOptions{GOOS: "linux", Home: e.Root, SteamRoots: []string{root}})
			if exe != want || !slices.Equal(gotArgs, args) {
				t.Fatalf("%s %v; want %s %v", exe, gotArgs, want, args)
			}
		})
	}
}

func TestLinuxManagedDir(t *testing.T) {
	e := testkit.New(t)
	root := filepath.Join(e.Game, "RimWorldLinux_Data")
	e.WriteFile(filepath.Join(root, "Managed", "Assembly-CSharp.dll"), "synthetic")
	if env.UnityDataDir(e.Game) != root || env.ManagedDir(e.Game) != filepath.Join(root, "Managed") {
		t.Fatal("native layout not found")
	}
}
