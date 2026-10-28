package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func phaseLine(t *testing.T, name, save string) string {
	return monitorLine(t, "phase", "INFO", "[Archo Monitor] phase "+name, "", 1, 0, "", map[string]any{"phase": name, "save": save})
}

func TestLogPhasesFullAndLive(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	root := filepath.Join(e.Root, "userdata")
	pl, am := filepath.Join(root, "Player.log"), filepath.Join(root, "ArchoMonitor.log")
	msg := "Could not load Texture2D at 'Things/Example/Empty' for def 'a_Thing'"
	ctx := map[string]any{"relatedModIds": []string{"tester.a"}, "inferredType": "MissingTexture"}
	entry := func(n, total int) string { return monitorLine(t, "", "WARNING", msg, "", n, total, "texture", ctx) }
	// Before any boundary, neither the full report nor the live tracker adds phases.
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa")+entry(1, 1)+entry(2, 3))
	e.WriteFile(pl, unityHead+marker("aaaaaaaaaaaa", am)+msg+"\n\n")
	lv := e.App.FollowLog(pl, am)
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	for _, c := range lv.Clusters() {
		if len(c.Phases) > 0 {
			t.Fatal("unmarked live log split")
		}
	}
	r, err := e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	if r.HasPhases || len(r.Clusters) != 1 || len(r.Clusters[0].Phases) != 0 {
		t.Fatalf("unmarked report: %+v", r)
	}
	// Same dedup id continues after the marker. total remains session-wide.
	appendFile(t, am, phaseLine(t, "menu", "")+phaseLine(t, "play", "")+entry(4, 7)+phaseLine(t, "save", "Colony1.rws")+entry(2, 9)+phaseLine(t, "play", ""))
	appendFile(t, pl, "[Archo Monitor] phase menu\n[Archo Monitor] phase play\n"+msg+"\n\n[Archo Monitor] phase save\n"+msg+"\n\n[Archo Monitor] phase play\n")
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	r, err = e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasPhases || len(r.Clusters) != 1 {
		t.Fatalf("report: %+v", r)
	}
	c := r.Clusters[0]
	want := " [loading x3, save x2, play x4]"
	if c.Count != 9 || logs.PhaseSummary(c.Phases) != want || c.Phase.Name != "loading" {
		t.Fatalf("full count %d %s first %+v", c.Count, logs.PhaseSummary(c.Phases), c.Phase)
	}
	if !strings.Contains(c.Finding.Claim, want) {
		t.Fatal(c.Finding.Claim)
	}
	for _, live := range lv.Clusters() {
		if live.Class == logs.ClassTexture && (live.Count != 9 || logs.PhaseSummary(live.Phases) != want || live.Report.Phase.Name != "loading") {
			t.Fatalf("live %+v %s", live, logs.PhaseSummary(live.Phases))
		}
	}
	if c.Phases[1].Phase.Save != "Colony1.rws" && c.Phases[2].Phase.Save != "Colony1.rws" {
		t.Fatal("save metadata lost")
	}
	out := e.MustRun("log", "--file", pl)
	for _, s := range []string{want, "save Colony1.rws", "phases:"} {
		if !strings.Contains(out, s) {
			t.Fatalf("CLI missing %q: %s", s, out)
		}
	}
	// A phase with only companion counts must remain visible when Player.log stops.
	appendFile(t, am, phaseLine(t, "new", "")+entry(5, 14))
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	for _, live := range lv.Clusters() {
		if live.Class == logs.ClassTexture && logs.PhaseSummary(live.Phases) != " [loading x3, save x2, new x5, play x4]" {
			t.Fatal(logs.PhaseSummary(live.Phases))
		}
	}
	// Same bytes after a cache reload retain structured phases, not just the claim text.
	r, err = e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasPhases || r.Clusters[0].Count != 14 {
		t.Fatal("companion phase cache not refreshed")
	}
	// Player.log can lag behind the companion. Its later copy of the same five
	// occurrences must not be added to the already merged live total.
	appendFile(t, pl, "[Archo Monitor] phase new\n"+strings.Repeat(msg+"\n\n", 5))
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	for _, live := range lv.Clusters() {
		if live.Class == logs.ClassTexture && (live.Count != 14 || logs.PhaseSummary(live.Phases) != " [loading x3, save x2, new x5, play x4]") {
			t.Fatalf("late Player.log double counted: %d %s", live.Count, logs.PhaseSummary(live.Phases))
		}
	}
}
