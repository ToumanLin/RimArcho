package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPlanLoadCostDisplay(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	for _, game := range []loadcost.Game{loadcost.Game16, loadcost.Game17} {
		r := &app.LoadCost{Game: game, Mods: 2, Report: loadcost.Estimate(game, []loadcost.Mod{{PNG: loadcost.Files{Count: 1, Bytes: 1024}, PNGPixelsMip: 4096}})}
		r.Baseline.RAM, r.Baseline.VRAM = loadcost.Baseline(game)
		r.Top = []app.TopMod{{Name: "Example", Ref: "#example", Page: "example", RAMHigh: 1024, VRAMHigh: 4096}}
		p := view.PlanLoadCost(r)
		if !strings.HasPrefix(p.Summary, "内存约 ") || !strings.Contains(p.Summary, "显存约 ") || !strings.Contains(p.Detail, "原版和 DLC") || p.Mods != "2 个 Mod" {
			t.Fatalf("translated totals: %+v", p)
		}
		if len(p.Parts) != len(r.Parts) || p.Parts[0].Files != "1 个文件" || p.Parts[0].Category != "贴图" || len(p.Top) != 1 {
			t.Fatalf("display rows: %+v", p)
		}
		notes := strings.Join(p.Notes, "\n")
		if len(p.Notes) < 5 || !strings.Contains(notes, "不算存档") {
			t.Fatalf("notes: %s", notes)
		}
		if game == loadcost.Game17 && (!strings.Contains(notes, "BC7") || !strings.Contains(notes, "2 MiB")) {
			t.Fatalf("1.7 notes: %s", notes)
		}
		if game == loadcost.Game16 && (!strings.Contains(notes, "静态图集") || !strings.Contains(notes, "300 KiB")) {
			t.Fatalf("1.6 notes: %s", notes)
		}
	}
}
