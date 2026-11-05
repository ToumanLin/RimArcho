package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

const ownArtLine = "Failed to find any textures at UI/LogRefs/Stand_Female while constructing Multi(initPath=UI/LogRefs/Stand_Female, color=RGBA(1.000, 1.000, 1.000, 1.000), colorTwo=RGBA(1.000, 1.000, 1.000, 1.000))\n"

// A texture path a mod builds in its code ("UI/LogRefs/Stand_" + defName) that no installed
// mod ships: the mod is the source by the literal prefix, and the cluster is marked as that
// mod's own missing art, rendering only — in the log, in a test run's remaining clusters and
// in its fixed list, where it went away because files were added (MCP review round 2 P22).
func TestOwnArtMissing(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "Coder"), testkit.Mod{PackageID: "tester.coder", Name: "Coder", Author: "Tester"})
	b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "LogRefs.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(mods, "Coder", "Assemblies", "LogRefs.dll"), string(b))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.coder")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	writePlayerLog(e, ownArtLine)
	out := e.MustRun("log", "--cluster", "E1")
	for _, want := range []string{`builds the path from "UI/LogRefs/Stand_" + … in its code (Assemblies/LogRefs.dll string literal)`, "own art missing: the source mod's code asks for this texture"} {
		if !strings.Contains(out, want) {
			t.Errorf("cluster misses %q:\n%s", want, out)
		}
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	r := testRunWith(t, e, "menu", ownArtLine)
	if len(r.Clusters) != 1 || r.Clusters[0].Before != "present" || !strings.HasPrefix(r.Clusters[0].Note, "own art missing") {
		t.Fatalf("remaining: %+v", r.Clusters)
	}
	r = testRunWith(t, e, "menu", "")
	if len(r.Fixed) != 1 || !strings.HasPrefix(r.Fixed[0].Note, "own art missing") {
		t.Fatalf("fixed: %+v", r.Fixed)
	}
}
