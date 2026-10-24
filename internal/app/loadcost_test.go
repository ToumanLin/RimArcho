package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestPlanLoadCostCacheAndSelectedPlan(t *testing.T) {
	e := setup(t, false)
	a := e.AddWorkshop("3101", testkit.Sample("tester.cost.a", "Cost A"))
	b := e.AddWorkshop("3102", testkit.Sample("tester.cost.b", "Cost B"))
	texPNG(t, filepath.Join(a, "Textures", "a.png"), 64, 64)
	texPNG(t, filepath.Join(b, "Textures", "b.png"), 6, 6)
	e.WriteFile(filepath.Join(a, "LoadFolders.xml"), "<loadFolders><v1.6><li>/</li><li IfModActive=\"tester.cost.b\">Extra</li></v1.6></loadFolders>")
	e.WriteFile(filepath.Join(a, "Extra", "Defs", "conditional.xml"), "<Defs/>")
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.cost.b", "tester.cost.a")
	scanNow(t, e)
	p, err := e.App.CurrentPlan()
	if err != nil {
		t.Fatal(err)
	}
	var progress [][2]int
	first, err := e.App.PlanLoadCost(context.Background(), p.ID, func(d, n int) { progress = append(progress, [2]int{d, n}) })
	if err != nil {
		t.Fatal(err)
	}
	if first.Mods != 2 || first.Measured != 2 || first.Cached != 0 || len(first.Top) != 2 || first.Game != loadcost.Game16 {
		t.Fatalf("first measurement: %+v", first)
	}
	if !reflect.DeepEqual(progress, [][2]int{{0, 2}, {1, 2}, {2, 2}}) {
		t.Fatalf("progress: %v", progress)
	}
	if first.Parts[0].Count != 2 || first.Parts[3].Count != 5 {
		t.Fatalf("effective assets missing: %+v", first.Parts)
	}
	if len(first.PerMod) != 2 || first.PerMod[0].PackageID != "tester.cost.b" || first.PerMod[1].PackageID != "tester.cost.a" ||
		first.PerMod[0].Name != "Cost B" || first.PerMod[0].Measure.PNGPixels != 36 || first.PerMod[1].Measure.PNGPixels != 4096 {
		t.Fatalf("per-mod measurements must retain load order: %+v", first.PerMod)
	}
	raw := make([]loadcost.Mod, 0, len(first.PerMod))
	for _, m := range first.PerMod {
		if m.Ref == "" || !reflect.DeepEqual(m.Est, loadcost.Estimate(first.Game, []loadcost.Mod{m.Measure})) {
			t.Fatalf("per-mod identity or estimate: %+v", m)
		}
		raw = append(raw, m.Measure)
	}
	if !reflect.DeepEqual(first.Report, loadcost.Estimate(first.Game, raw)) {
		t.Fatal("per-mod measurements do not reproduce totals")
	}
	second, err := e.App.PlanLoadCost(context.Background(), p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Measured != 0 || second.Cached != 2 || !reflect.DeepEqual(first.Report, second.Report) || !reflect.DeepEqual(first.PerMod, second.PerMod) {
		t.Fatalf("cache miss: %+v", second)
	}
	// Irrelevant files and inactive folders must not invalidate either mod.
	e.WriteFile(filepath.Join(a, "About", "notes.txt"), "changed")
	e.WriteFile(filepath.Join(a, "1.5", "Defs", "inactive.xml"), "<Defs/>")
	third, err := e.App.PlanLoadCost(context.Background(), p.ID, nil)
	if err != nil || third.Measured != 0 {
		t.Fatalf("irrelevant change invalidates cache: %+v, %v", third, err)
	}
	// A metadata change in a selected folder invalidates only that mod.
	texPNG(t, filepath.Join(b, "Textures", "b.png"), 32, 32)
	changed, err := e.App.PlanLoadCost(context.Background(), p.ID, nil)
	if err != nil || changed.Measured != 1 || changed.Cached != 1 || changed.VRAMHigh == second.VRAMHigh {
		t.Fatalf("asset change: %+v, %v", changed, err)
	}
	// The stored plan remains two mods when the current workspace has only one.
	e.MustRun("plan", "new", "cost-copy", "--from", "current")
	copy, err := e.App.FindPlan("cost-copy")
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("disable", "pkg:tester.cost.b")
	other, err := e.App.PlanLoadCost(context.Background(), copy.ID, nil)
	if err != nil || other.Mods != 2 || other.Measured != 0 || other.Cached != 2 {
		t.Fatalf("non-current plan: %+v, %v", other, err)
	}
	current, err := e.App.PlanLoadCost(context.Background(), p.ID, nil)
	if err != nil || current.Mods != 1 || current.Measured != 1 || current.Parts[3].Count != 2 {
		t.Fatalf("plan-dependent folders: %+v, %v", current, err)
	}
	for _, args := range [][]string{{"plan", "cost"}, {"plan", "cost", "cost-copy"}} {
		out := e.MustRun(args...)
		for _, want := range []string{"load cost (estimate)", "baseline + mods", "mods only", "parts", "Cost A"} {
			if !strings.Contains(out, want) {
				t.Fatalf("CLI lacks %q:\n%s", want, out)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.App.PlanLoadCost(ctx, p.ID, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	e.App.GameVersion = "1.7"
	newGame, err := e.App.PlanLoadCost(context.Background(), p.ID, nil)
	if err != nil || newGame.Game != loadcost.Game17 {
		t.Fatalf("1.7 game: %+v, %v", newGame, err)
	}
	ram, vram := loadcost.Baseline(loadcost.Game17)
	if newGame.Baseline.RAM != ram || newGame.Baseline.VRAM != vram {
		t.Fatalf("1.7 baseline: %+v", newGame.Baseline)
	}
}

func TestPlanLoadCostMissingLoadFolder(t *testing.T) {
	e := setup(t, false)
	dir := e.AddWorkshop("3103", testkit.Sample("tester.cost.missing", "Missing folder"))
	e.WriteFile(filepath.Join(dir, "LoadFolders.xml"), "<loadFolders><v1.6><li>/</li><li>NotShipped</li></v1.6></loadFolders>")
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.cost.missing")
	scanNow(t, e)
	first, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Mods != 1 || first.Measured != 1 || first.Parts[3].Count != 2 {
		t.Fatalf("missing load folder: %+v", first)
	}
	cached, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil || cached.Measured != 0 || cached.Cached != 1 {
		t.Fatalf("missing folder cache: %+v, %v", cached, err)
	}
	// Creating an empty load folder must replace the missing-folder fingerprint.
	e.MkdirAll(filepath.Join(dir, "NotShipped"))
	created, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil || created.Measured != 1 || !reflect.DeepEqual(created.Report, first.Report) {
		t.Fatalf("newly created load folder: %+v, %v", created, err)
	}
	e.WriteFile(filepath.Join(dir, "NotShipped", "Defs", "new.xml"), "<Defs/>")
	populated, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil || populated.Measured != 1 || populated.Parts[3].Count != 3 {
		t.Fatalf("newly populated load folder: %+v, %v", populated, err)
	}
}
