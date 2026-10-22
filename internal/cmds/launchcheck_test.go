package cmds_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 04 §8: before a launch, the problems of app.LaunchChecks are listed, one row per code; several
// problems of a code are summed up, with their shared fix. Nothing of it blocks the launch.
func TestLaunchChecksGrouped(t *testing.T) {
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), testkit.Sample("tester.lib", "Lib"))
	for _, n := range []string{"Alpha", "Beta"} {
		e.WriteFile(filepath.Join(e.Game, "Mods", n, "About", "About.xml"), fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<ModMetaData>
  <packageId>tester.%s</packageId>
  <name>%s</name>
  <supportedVersions><li>1.6</li></supportedVersions>
  <modDependencies><li><packageId>tester.lib</packageId><displayName>Lib</displayName></li></modDependencies>
</ModMetaData>
`, strings.ToLower(n), n))
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	e.AddEnv(false)
	e.MustRun("scan")
	out, code := e.Run("launch", "--dry-run")
	if code != 0 {
		t.Fatalf("launch blocked: %s", out)
	}
	lib := ref(e, "Lib")
	for _, want := range []string{
		"before you play: 2 problems that often cause errors",
		"error dep.inactive  2 dependencies of 2 mods are installed but not enabled: " + ref(e, "Alpha") + " Alpha, " + ref(e, "Beta") + " Beta",
		"why: a mod without its dependency fails in parts",
		"fix: archo enable " + lib,
		"checks: declarations only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Every row of the table names a distinct code and says why.
	seen := map[string]bool{}
	for _, c := range app.LaunchChecks {
		if seen[c.Code] || c.Why == "" || c.Many == nil {
			t.Errorf("bad LaunchChecks row %q", c.Code)
		}
		seen[c.Code] = true
	}
	// Fixed: nothing listed.
	e.MustRun("enable", lib)
	e.MustRun("save")
	out = e.MustRun("launch", "--dry-run")
	if strings.Contains(out, "before you play") {
		t.Fatalf("still listed after the fix:\n%s", out)
	}
}
