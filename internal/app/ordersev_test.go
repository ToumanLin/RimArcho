package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Load-order findings by constraint strength (04 §4.4), and the DLC-after-mods move.
func TestOrderSeverityAndDLCMove(t *testing.T) {
	e := setup(t, false)
	e.WriteFile(filepath.Join(e.Game, "Data", "Odyssey", "About", "About.xml"),
		"<ModMetaData><packageId>Ludeon.RimWorld.Odyssey</packageId><name>Odyssey</name></ModMetaData>")
	lib := testkit.Sample("tester.lib", "Lib")
	user := testkit.Sample("tester.user", "User")
	user.Deps = []string{"tester.lib"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteMod(filepath.Join(e.Game, "Mods", "User"), user)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Late", "About", "About.xml"), `<ModMetaData><packageId>tester.late</packageId><name>Late</name>
<supportedVersions><li>1.6</li></supportedVersions><loadAfter><li>tester.lib</li></loadAfter></ModMetaData>`)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Sorter", "About", "About.xml"), `<ModMetaData><packageId>Superniquito.ModOptionsSort</packageId><name>Mod Options Sort</name>
<supportedVersions><li>1.6</li></supportedVersions><loadBefore><li>Ludeon.RimWorld</li></loadBefore></ModMetaData>`)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Needy", "About", "About.xml"), `<ModMetaData><packageId>tester.needy</packageId><name>Needy</name>
<supportedVersions><li>1.6</li></supportedVersions><loadAfter><li>Ludeon.RimWorld.Odyssey</li></loadAfter></ModMetaData>`)
	// Odyssey enabled later: appended after the mods.
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "superniquito.modoptionssort", "tester.late", "tester.user", "tester.lib", "tester.needy", "ludeon.rimworld.odyssey")
	scanNow(t, e)
	ps := problems(t, e)
	sev := map[string]string{}
	for _, p := range byCode(ps.List, "order.violated") {
		sev[p.Key] = p.Severity
	}
	want := map[string]string{
		"tester.lib>tester.user":                      finding.Warning, // dependency
		"tester.lib>tester.late":                      finding.Info,    // loadAfter
		"superniquito.modoptionssort>ludeon.rimworld": finding.Info,    // observed harmless
	}
	for k, w := range want {
		if sev[k] != w {
			t.Errorf("%s: %q, want %q (all %v)", k, sev[k], w, sev)
		}
	}
	if _, ok := sev["ludeon.rimworld.odyssey>tester.needy"]; ok {
		t.Errorf("needs-a-DLC violation should be part of official.order: %v", sev)
	}
	off := byCode(ps.List, "official.order")
	if len(off) != 1 {
		t.Fatalf("official.order = %+v", off)
	}
	odyssey, core := finding.Inst(instOf(t, e, "pkg:ludeon.rimworld.odyssey")), finding.Inst(instOf(t, e, "pkg:ludeon.rimworld"))
	if !slices.Contains(off[0].Suggestions, "archo move "+odyssey+" --after "+core) || off[0].Suggestions[0] != "archo move "+odyssey+" --after "+core {
		t.Fatalf("official.order suggestions %v", off[0].Suggestions)
	}
	needs := false
	for _, ev := range off[0].Evidence {
		needs = needs || strings.Contains(ev.Text, "needs it loaded first")
	}
	if !needs || !strings.Contains(off[0].Claim, "move it back") {
		t.Fatalf("official.order = %+v", off[0])
	}
	// The harmless one says why, and offers the ignore rule first.
	for _, p := range byCode(ps.List, "order.violated") {
		if p.Key == "superniquito.modoptionssort>ludeon.rimworld" {
			if !strings.Contains(p.Claim, "not known to cause errors") || !strings.HasPrefix(p.Suggestions[0], "archo rule add ignore ") {
				t.Fatalf("harmless: %+v", p)
			}
		}
	}
	// The suggested move fixes official.order.
	e.MustRun("move", "pkg:ludeon.rimworld.odyssey", "--after", "pkg:ludeon.rimworld")
	if off := byCode(problems(t, e).List, "official.order"); len(off) != 0 {
		t.Fatalf("after the move: %+v", off)
	}
}

// A loadBefore Core that other constraints make impossible is left out with the reason,
// not reported as a cycle.
func TestSortDropsUnsatisfiable(t *testing.T) {
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Early", "About", "About.xml"), `<ModMetaData><packageId>tester.early</packageId><name>Early</name>
<supportedVersions><li>1.6</li></supportedVersions><loadBefore><li>Ludeon.RimWorld</li></loadBefore>
<modDependencies><li><packageId>tester.lib</packageId><displayName>Lib</displayName></li></modDependencies></ModMetaData>`)
	e.WriteModsConfig("1.6.4633 rev1261", "tester.early", "ludeon.rimworld", "tester.lib")
	scanNow(t, e)
	out := e.MustRun("sort", "--dry-run")
	if strings.Contains(out, "order.cycle") || !strings.Contains(out, "not used:") || !strings.Contains(out, "loadBefore pkg:ludeon.rimworld  (would put the mod before official content") {
		t.Fatalf("sort:\n%s", out)
	}
	if cy := byCode(problems(t, e).List, "order.cycle"); len(cy) != 0 {
		t.Fatalf("cycle: %+v", cy)
	}
}
