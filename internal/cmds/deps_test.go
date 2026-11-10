package cmds_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestGoldenDeps(t *testing.T) {
	if runtime.GOOS == "linux" {
		// Linux plan §3 step 3: the removal transcript needs toolIdle.
		t.Skip("Linux tool-process detection is not implemented (plan step 3)")
	}
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
