package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

func TestLinuxLaunch(t *testing.T) {
	for _, mode := range []string{"steam", "direct", "own", "custom", "argument", "missing steam", "native", "flatpak direct", "flatpak native", "flatpak proton"} {
		t.Run(mode, func(t *testing.T) {
			e := loadoutEnv(t)
			bin := filepath.Join(e.Root, "empty-path")
			e.MkdirAll(bin)
			t.Setenv("PATH", bin)
			e.App.Detect = env.DetectOptions{GOOS: "linux", Home: e.Root, SteamRoots: []string{e.Steam}, ManualRoots: []string{}, SkipRegistry: true}
			e.MkdirAll(filepath.Join(e.Steam, "steamapps", "compatdata", env.WorkshopAppID, "pfx"))
			if mode == "native" || mode == "flatpak direct" || mode == "flatpak native" {
				e.WriteFile(filepath.Join(e.Game, "RimWorldLinux"), "")
			}
			if strings.HasPrefix(mode, "flatpak") {
				flatRoot := filepath.Join(e.Root, ".var", "app", "com.valvesoftware.Steam", "data", "Steam")
				e.WriteFile(filepath.Join(flatRoot, "steamapps", "libraryfolders.vdf"), fmt.Sprintf(`"libraryfolders" { "1" { "path" %q } }`, e.Steam))
				e.App.Detect.SteamRoots = []string{flatRoot}
				if mode != "flatpak direct" {
					name := "flatpak"
					if runtime.GOOS == "windows" {
						name += ".exe"
					}
					t.Setenv("PATHEXT", ".EXE")
					if err := os.WriteFile(filepath.Join(bin, name), []byte("never executed"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			ud := env.DefaultUserDataForGame(e.App.Detect, e.Game)
			e.WriteFile(filepath.Join(ud, "Config", "ModsConfig.xml"), e.ReadModsConfig())
			e.MustRun("env", "set", "main", "--saves", "shared", "--launch", "steam")
			current, _ := e.App.Store.EnvByName("main")
			if current.UserDataRoot != ud || e.App.Saves(current) != app.SavesShared {
				t.Fatalf("shared default: %+v", current)
			}
			if mode == "own" {
				if _, _, err := e.App.EnvSet("main", app.EnvSpec{Saves: app.SavesOwn}); err == nil || !strings.Contains(err.Error(), "Proton") {
					t.Fatalf("own saves: %v", err)
				}
				if _, err := os.Stat(filepath.Join(e.Home, "userdata", "main")); !os.IsNotExist(err) {
					t.Fatal("own saves were created")
				}
				otherLib := filepath.Join(e.Root, "second-library")
				otherGame := filepath.Join(otherLib, "steamapps", "common", "RimWorld")
				e.WriteFile(filepath.Join(otherGame, "RimWorldWin64.exe"), "")
				e.MkdirAll(filepath.Join(otherLib, "steamapps", "compatdata", env.WorkshopAppID, "pfx"))
				if _, _, err := e.App.EnvAdd("own", app.EnvSpec{GameRoot: otherGame, Saves: app.SavesOwn}); err == nil || !strings.Contains(err.Error(), "Proton") {
					t.Fatalf("adding own saves: %v", err)
				}
				if added, _ := e.App.Store.EnvByName("own"); added != nil {
					t.Fatal("refused environment was added")
				}
				return
			}
			if mode != "missing steam" {
				e.WriteFile(filepath.Join(e.Steam, "steam.sh"), "never executed")
			}
			if mode == "direct" || mode == "native" || mode == "flatpak direct" {
				e.MustRun("env", "set", "main", "--launch", "direct")
			}
			if mode == "custom" {
				e.MustRun("env", "set", "main", "--user-data", e.UserData)
			}
			if mode == "argument" {
				e.MustRun("env", "set", "main", "--launch-args=-savedatafolder=elsewhere")
			}
			scanNow(t, e)
			config := filepath.Join(ud, "Config", "ModsConfig.xml")
			before, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			out, code := e.Run("launch", "--dry-run")
			if mode == "steam" || mode == "native" || strings.HasPrefix(mode, "flatpak") {
				want := "-applaunch 294100"
				if mode == "native" || mode == "flatpak direct" {
					want = "RimWorldLinux"
				}
				if mode == "flatpak native" || mode == "flatpak proton" {
					want = "run com.valvesoftware.Steam -applaunch 294100"
				}
				if code != 0 || !strings.Contains(out, want) || strings.Contains(out, "-savedatafolder") != (mode == "flatpak direct") {
					t.Fatalf("launch preview: %d %s", code, out)
				}
				if mode == "flatpak direct" && !strings.Contains(out, "-savedatafolder="+ud) {
					t.Fatalf("Flatpak data argument: %s", out)
				}
				e.MustRun("launch")
				if len(e.Launched) != 1 || !strings.Contains(e.Launched[0], want) {
					t.Fatalf("launch: %v", e.Launched)
				}
			} else {
				if code == 0 || !strings.Contains(out, "Proton") {
					t.Fatalf("refusal: %d %s", code, out)
				}
				if _, err := e.App.Launch(false); err == nil {
					t.Fatal("real launch was allowed")
				}
				after, _ := os.ReadFile(config)
				if string(after) != string(before) || len(e.Launched) != 0 {
					t.Fatal("refused launch wrote or started a game")
				}
			}
			if mode == "steam" {
				if _, err := e.App.TestRun(context.Background(), app.TestRunOptions{}); err == nil || !strings.Contains(err.Error(), "Proton") {
					t.Fatalf("isolated run: %v", err)
				}
				if _, err := os.Stat(filepath.Join(e.Home, "tests")); !os.IsNotExist(err) {
					t.Fatal("test run created data")
				}
				e.MustRun("disable", "pkg:gone.mod")
				e.MustRun("pack", "build", "proton-test")
				for _, dry := range []bool{true, false} {
					if _, err := e.App.LaunchPack("proton-test", dry); err == nil || !strings.Contains(err.Error(), "Proton") {
						t.Fatalf("pack launch: %v", err)
					}
				}
				if _, err := os.Stat(filepath.Join(e.Home, "packs", "proton-test", "userdata")); !os.IsNotExist(err) {
					t.Fatal("refused pack created user data")
				}
			}
		})
	}
}
