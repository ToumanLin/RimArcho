package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Known noise does not fail a test run, and an error cluster with no mod identified whose kind
// is in more than 5% of public logs carries the "common kind, no source found" note (MCP transcript
// review 2026-11-04, P2: all 22 test_run results failed on the solid-colour material alone).
func TestTestRunKnownNoise(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	noise := "Material 'SolidColorMat-Custom/Solid Color-RGBA(0.000, 0.000, 0.000, 0.000)' with Shader 'Custom/Solid Color' doesn't have a texture property '_MainTex'\n" +
		"Translation data for language English has 19 errors. Generate translation report for more info.\n"
	e.App.TestGameStart = fakeTestGame(t, "menu", noise)
	out, code := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms", "--json")
	var r app.TestRunReport
	if code != 0 || json.Unmarshal([]byte(out), &r) != nil {
		t.Fatalf("noise only: exit %d: %s", code, out)
	}
	if !r.Success() || r.ClustersTotal != 0 || r.Counts[logs.ClassNoise] != 2 {
		t.Fatalf("noise only: %+v", r)
	}

	e.App.TestGameStart = fakeTestGame(t, "menu", noise+"Material 'Tester_Icon' with Shader 'Custom/Cutout' doesn't have a texture property '_MainTex'\n")
	out, code = e.Run("test", "run", "--timeout", "5s", "--settle", "1ms", "--json")
	r = app.TestRunReport{}
	if code != 1 || json.Unmarshal([]byte(out), &r) != nil {
		t.Fatalf("def material: exit %d: %s", code, out)
	}
	if r.ClustersTotal != 1 || r.Clusters[0].Class != logs.ClassMaterial || len(r.Clusters[0].Mods) != 0 ||
		!strings.HasPrefix(r.Clusters[0].Note, "common kind, no source found: ") || !strings.Contains(r.Clusters[0].Note, "pattern:g:asset.maintex") {
		t.Fatalf("def material: %+v", r.Clusters)
	}
	out, _ = e.Run("test", "run", "--timeout", "5s", "--settle", "1ms")
	if !strings.Contains(out, "note: common kind, no source found: this kind is in 6.7% of public 1.6 logs") {
		t.Fatalf("text report:\n%s", out)
	}
}
