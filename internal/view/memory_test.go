package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestMemoryNeedSeverityAndUnknown(t *testing.T) {
	view.SetLang("en")
	t.Cleanup(func() { view.SetLang("en") })
	r := &app.LoadCost{LoadingRAMLow: 2 << 30, LoadingRAMHigh: 4 << 30}
	r.VRAMHigh = 3 << 30
	m := machine.Memory{TotalRAM: 8 << 30, AvailableRAM: 1 << 30, SampledAt: time.Now(), Adapters: []machine.Adapter{{Name: "fixture", Dedicated: 2 << 30}}}
	n := view.MemoryNeed(app.AssessMemory(m, r), 1)
	if n.Status != view.StatusErr || !n.Counted || n.Section != view.SectionBreak {
		t.Fatalf("RAM not severe: %+v", n)
	}
	if !strings.Contains(n.Text, "close other apps") || !strings.Contains(n.Text, "optimize textures") {
		t.Fatalf("missing actions: %s", n.Text)
	}
	// Past available RAM the game pages but runs: a warning is worth knowing, not counted.
	m.AvailableRAM = 3 << 30
	n = view.MemoryNeed(app.AssessMemory(m, r), 1)
	if n.Status != view.StatusWarn || n.Counted || n.Section != view.SectionHint {
		t.Fatalf("RAM warning not shared: %+v", n)
	}
	m.AvailableRAM = 8 << 30
	n = view.MemoryNeed(app.AssessMemory(m, r), 1)
	if n.Status != view.StatusWarn || n.Counted || n.Section != view.SectionHint {
		t.Fatalf("VRAM not ordinary warning: %+v", n)
	}
	// Severe follows what can still be committed, not available physical RAM.
	m.AvailableRAM, m.CommitAvailable = 1<<30, 16<<30
	if n = view.MemoryNeed(app.AssessMemory(m, r), 1); n.Status != view.StatusWarn || n.Counted {
		t.Fatalf("page file headroom still severe: %+v", n)
	}
	m.CommitAvailable = 0
	n = view.MemoryNeed(app.AssessMemory(machine.Memory{}, nil), 1)
	if n.Counted || !strings.Contains(n.Text, "not measured") || !strings.Contains(n.Text, "unavailable") || len(n.Actions) != 1 {
		t.Fatalf("unknown treated as zero: %+v", n)
	}
	view.SetLang("zh")
	n = view.MemoryNeed(app.AssessMemory(m, r), 1)
	if !strings.Contains(n.Text, "显卡专用显存") || !strings.Contains(n.Text, "显存可能不足") {
		t.Fatalf("Chinese missing: %s", n.Text)
	}
}

// 1.7 keeps at most its upload watermark in VRAM: a texture-heavy list on an 8 GiB
// card is no VRAM shortage, and the item says the rest loads on demand.
func TestMemoryNeedTextures17OnDemand(t *testing.T) {
	view.SetLang("en")
	t.Cleanup(func() { view.SetLang("en") })
	const gib = int64(1 << 30)
	r := &app.LoadCost{Game: loadcost.Game17, Baseline: app.LoadCostBaseline{RAM: 2 * gib, VRAM: gib}}
	r.Parts = []loadcost.Part{{Key: "textures", RAMLow: 15 * gib, RAMHigh: 35 * gib, VRAMLow: 9 * gib, VRAMHigh: 26 * gib}}
	r.RAMLow, r.RAMHigh, r.VRAMLow, r.VRAMHigh = 16*gib, 37*gib, 9*gib, 26*gib
	r.LoadingRAMLow, r.LoadingRAMHigh = 17*gib, 40*gib
	m := machine.Memory{TotalRAM: 32 * gib, AvailableRAM: 20 * gib, CommitAvailable: 30 * gib, SampledAt: time.Now(), Adapters: []machine.Adapter{{Name: "fixture", Dedicated: 8 * gib}}}
	c := app.AssessMemory(m, r)
	if c.VRAMSeverity != "ok" || c.VRAMHigh > 8*gib || c.TexturesOnDemand == 0 || c.RAMSeverity != "ok" {
		t.Fatalf("1.7 textures not capped by the card: %+v", c)
	}
	n := view.MemoryNeed(c, 1)
	if n.Counted || n.Status != view.StatusHint || !strings.Contains(n.Text, "load from the disk cache when drawn") || strings.Contains(n.Text, "VRAM may be insufficient") {
		t.Fatalf("on-demand textures reported as a shortage: %s", n.Text)
	}
}

func TestRefreshMemoryPreservesSectionOrder(t *testing.T) {
	t.Parallel()
	e, b, _ := shellEnv(t)
	e.App.MemoryProbe = func() machine.Memory { return machine.Memory{} }
	old := &view.Needs{Items: []view.Need{
		{Section: view.SectionRun, Group: view.GroupLastRun, Text: "last run", Counted: true, Status: view.StatusErr},
		view.MemoryNeed(app.AssessMemory(machine.Memory{}, nil), b.St.Plan.ID),
	}}
	fresh := view.RefreshMemory(e.App, b.St, old)
	if len(fresh.Items) != 2 || fresh.Items[0].Kind != "memory.capacity" || fresh.Items[1].Section != view.SectionRun || fresh.Count != 1 {
		t.Fatalf("refresh changed section order/counts: %+v", fresh)
	}
	if old.Items[0].Section != view.SectionRun {
		t.Fatal("refresh mutated cached items")
	}
}
