package cmds_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestEnvDetectListsUnmanagedProtonLibrary(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.Detect.GOOS = "linux"
	e.WriteFile(filepath.Join(e.Game, "RimWorldLinux"), "synthetic native game")
	lib := filepath.Join(e.Root, "Extra Steam Library")
	game := filepath.Join(lib, "steamapps", "common", "RimWorld")
	e.WriteFile(filepath.Join(game, "RimWorldWin64.exe"), "synthetic Proton game")
	ud := filepath.Join(lib, "steamapps", "compatdata", "294100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
	e.MkdirAll(ud)
	e.WriteFile(filepath.Join(e.Steam, "steamapps", "libraryfolders.vdf"), fmt.Sprintf(`"libraryfolders" { "0" { "path" %q } "1" { "path" %q } }`, e.Steam, lib))
	if _, err := os.Stat(filepath.Join(lib, "steamapps", "appmanifest_294100.acf")); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has a manifest: %v", err)
	}
	all := env.DetectAll(e.App.Detect)
	if len(all) != 2 || all[1].GameRoot != game || all[1].UserDataRoot != ud {
		t.Fatalf("unmanaged library not detected: %+v", all)
	}
	out := e.MustRun("env", "detect")
	for _, want := range []string{e.Game, "another detected environment", game, "user-data: " + ud, "--game"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if first := e.App.Detected(); first.GameRoot != e.Game {
		t.Fatalf("default --detected choice changed: %+v", first)
	}
	registered, err := e.App.Store.Envs()
	if err != nil || len(registered) != 0 {
		t.Fatalf("detect registered an environment: %+v %v", registered, err)
	}
}
