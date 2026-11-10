package cmds_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

func TestDepsConfiguredToddsSource(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	path := filepath.Join(e.Root, "encoder", textures.ToddsExecutable)
	e.WriteFile(path, "synthetic encoder; never executed")
	if err := e.App.Store.SetMeta("cfg:textures.todds", path); err != nil {
		t.Fatal(err)
	}
	e.App.Deps().Version = func(string, string) (string, error) { return textures.ToddsVersion, nil }
	for _, want := range []string{"status: installed", "path: " + path, "source: config"} {
		if out := e.MustRun("deps"); !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestGoldenDeps(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), deps.SteamCMD, deps.SteamCMDExecutable()), "fake")
	runGolden(t, "deps", e, []step{
		cmd("deps"),
		cmd("deps", "install", "unknown"),
		cmd("deps", "remove", "steamcmd"),
		cmd("deps", "install"),
	})
}
