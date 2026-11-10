package cmds_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestGoldenDeps(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), deps.SteamCMD, "steamcmd.exe"), "fake")
	runGolden(t, "deps", e, []step{
		cmd("deps"),
		cmd("deps", "install", "unknown"),
		cmd("deps", "remove", "steamcmd"),
		cmd("deps", "install"),
	})
}
