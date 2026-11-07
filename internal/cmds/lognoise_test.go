package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The solid-colour material made in code and the translation error count are known noise:
// hidden informational clusters, named in the hidden line, with an honest note and no
// uiIconColor route. A def's texture-less material with no mod identified is labelled
// "common kind, no source found" (MCP transcript review 2026-11-04, P2; 06 §1, §8).
func TestLogKnownNoiseAndCommon(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	mods := filepath.Join(e.Game, "Mods")
	log := testkit.FixtureLog(filepath.Join(mods, "Compat", "Patches", "Compat.xml")) +
		"Material 'SolidColorMat-Custom/Solid Color-RGBA(0.000, 0.000, 0.000, 0.000)' with Shader 'Custom/Solid Color' doesn't have a texture property '_MainTex'\n" +
		"Material 'SolidColorMat-Custom/Solid Color-RGBA(0.000, 0.000, 0.000, 0.000)' with Shader 'Custom/Solid Color' doesn't have a texture property '_MainTex'\n" +
		"Translation data for language English has 19 errors. Generate translation report for more info.\n" +
		"Material 'Tester_Icon' with Shader 'Custom/Cutout' doesn't have a texture property '_MainTex'\n"
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, log)
	os.Chtimes(p, e.Now(), e.Now())

	out := e.MustRun("log")
	for _, want := range []string{
		"log:E7 material x1  Material 'Tester_Icon'",
		"(common kind, no source found)  [pattern:g:asset.maintex]",
		"hidden: 2 informational clusters (archo log --all); known noise, not mod problems: log:E5 material made in code, log:E6 translation error count",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log: missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "SolidColorMat") || strings.Contains(out, "Translation data") {
		t.Errorf("log lists known noise by default:\n%s", out)
	}
	if out := e.MustRun("log", "--all"); !strings.Contains(out, "log:E5 noise x2  Material 'SolidColorMat-") || !strings.Contains(out, "log:E6 noise x1  Translation data") {
		t.Errorf("log --all:\n%s", out)
	}

	solid := e.MustRun("log", "--cluster", "E5")
	for _, want := range []string{"detail: known noise: a material made in code", "no def or file causes it"} {
		if !strings.Contains(solid, want) {
			t.Errorf("E5: missing %q in\n%s", want, solid)
		}
	}
	for _, bad := range []string{"uiIconColor but no icon path", "search_files uiIconColor", "pattern:g:asset.maintex", "common kind, no source found"} {
		if strings.Contains(solid, bad) {
			t.Errorf("E5: %q in\n%s", bad, solid)
		}
	}
	trans := e.MustRun("log", "--cluster", "E6")
	if !strings.Contains(trans, "detail: informational: the game's count of problems in the active language's translation files") || strings.Contains(trans, "pattern:") {
		t.Errorf("E6:\n%s", trans)
	}

	def := e.MustRun("log", "--cluster", "E7")
	for _, want := range []string{
		"note: common kind, no source found: this kind is in 6.7% of public 1.6 logs (pattern:g:asset.maintex)",
		// Nearby lines tag known noise.
		"Translation data for language English has 19 errors. Generate translation report for more info.  (known noise: translation error count)",
	} {
		if !strings.Contains(def, want) {
			t.Errorf("E7: missing %q in\n%s", want, def)
		}
	}
	// Attributed clusters are never labelled.
	if strings.Count(out, "common kind, no source found") != 1 {
		t.Errorf("only E7 should be labelled:\n%s", out)
	}
}
