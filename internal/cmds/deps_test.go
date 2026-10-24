package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestGoldenDeps(t *testing.T) {
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), deps.SteamCMD, "steamcmd.exe"), "fake")
	wezterm := filepath.Join(e.Root, "external", "wezterm-gui.exe")
	e.WriteFile(wezterm, "fake")
	e.MustRun("config", "set", "wezterm.path", wezterm)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	program := filepath.Dir(exe)
	runGolden(t, "deps", e, []step{
		cmd("deps"),
		cmd("deps", "install", "unknown"),
		cmd("deps", "install", "font"),
		cmd("deps", "remove", "wezterm"),
		cmd("deps", "remove", "steamcmd"),
		cmd("deps", "install"),
	}, func(s string) string { return strings.ReplaceAll(s, filepath.ToSlash(program), "<program>") })
}

func TestWezTermPathSetting(t *testing.T) {
	e := testkit.New(t)
	for _, p := range []string{filepath.Join(e.Root, "missing.exe"), e.Root} {
		if out, code := e.Run("config", "set", "wezterm.path", p); code != cmds.ExitUsage || !strings.Contains(out, "existing program file") {
			t.Fatalf("accepted %q: %s (%d)", p, out, code)
		}
	}
	p := filepath.Join(e.Root, "external", "wezterm-gui.exe")
	e.WriteFile(p, "fake")
	e.MustRun("config", "set", "wezterm.path", p)
	if i := e.App.Deps().Status(deps.WezTerm); i.Path != p || i.Source != "config" || i.Own {
		t.Fatalf("configured %+v", i)
	}
	e.MustRun("config", "set", "wezterm.path", "")
}
