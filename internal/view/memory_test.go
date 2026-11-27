package view_test

import (
	"runtime"
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

func TestMemoryNeedUnknownWording(t *testing.T) {
	t.Cleanup(func() { view.SetLang("en") })
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, hardware := range []machine.Memory{
			{},
			{TotalRAM: 8 << 30, RAMAvailableUnknown: true, SampledAt: time.Unix(1, 0)},
			{TotalRAM: 8 << 30, AvailableRAM: 0, SampledAt: time.Unix(1, 0)},
		} {
			c := app.AssessMemory(hardware, nil)
			n := view.MemoryNeed(c, 1)
			if n.Counted || n.Status != view.StatusHint || c.RAMSeverity != "unknown" || c.VRAMSeverity != "unknown" {
				t.Fatalf("unknown capacity warned: %+v", c)
			}
			if runtime.GOOS == "windows" {
				if hardware.TotalRAM == 0 && !strings.Contains(n.Text, view.Tf("RAM: about %s in Task Manager (estimated); available now %s of %s", view.T("not measured"), view.T("unavailable"), view.T("unavailable"))) {
					t.Fatalf("Windows text changed: %s", n.Text)
				}
			} else {
				if !strings.Contains(n.Text, view.Tf("VRAM: estimated %s; dedicated capacity is unknown", view.T("not measured"))) {
					t.Fatal(n.Text)
				}
				if hardware.TotalRAM == 0 && !strings.Contains(n.Text, view.Tf("RAM: the game uses about %s (estimated); available memory and total capacity are unavailable", view.T("not measured"))) {
					t.Fatal(n.Text)
				}
				if hardware.RAMAvailableUnknown && !strings.Contains(n.Text, view.Tf("RAM: the game uses about %s (estimated); available memory is unavailable; total capacity %s", view.T("not measured"), view.SizeText(hardware.TotalRAM))) {
					t.Fatal(n.Text)
				}
			}
		}
	}
}

// The GUI's bars (16 §7 principle 9) carry every sentence they replace: each bar is named by
// its line, the rest of Why is shown beside them, and Text and Why still hold everything.
func TestMemoryNeedBarsKeepTheSentences(t *testing.T) {
	view.SetLang("en")
	t.Cleanup(func() { view.SetLang("en") })
	r := &app.LoadCost{LoadingRAMLow: 2 << 30, LoadingRAMHigh: 4 << 30, Baseline: app.LoadCostBaseline{RAM: 1 << 30}}
	r.VRAMLow, r.VRAMHigh = 1<<30, 3<<30
	m := machine.Memory{TotalRAM: 8 << 30, AvailableRAM: 3 << 30, SampledAt: time.Now(), Adapters: []machine.Adapter{{Name: "fixture GPU", Dedicated: 2 << 30}}}
	c := app.AssessMemory(m, r)
	n := view.MemoryNeed(c, 1)
	if len(n.Bars) != 2 {
		t.Fatalf("bars: %+v", n.Bars)
	}
	ram, vram := n.Bars[0], n.Bars[1]
	// the RAM bar is the expected working set (#31), not the loading commit
	if ram.Total != 8<<30 || ram.Used != 5<<30 || len(ram.Parts) != 1 || ram.Parts[0].Low != c.WorkingSetLow || ram.Parts[0].High != c.WorkingSetHigh || c.WorkingSetHigh == 0 {
		t.Fatalf("RAM bar: %+v", ram)
	}
	if vram.Total != 2<<30 || vram.Used != 0 || vram.Note != "fixture GPU" || vram.Parts[0].High != 3<<30 {
		t.Fatalf("VRAM bar: %+v", vram)
	}
	lines := strings.Split(n.Why, "\n")
	if !strings.HasPrefix(ram.Text, lines[0]) || !strings.HasPrefix(vram.Text, lines[1]) || !strings.Contains(vram.Text, "GPU: fixture GPU") {
		t.Fatalf("bars not named by their sentences: %q / %q; why %q", ram.Text, vram.Text, n.Why)
	}
	for _, l := range lines[2:] {
		if !strings.HasPrefix(l, "GPU:") && !strings.Contains(n.BarsWhy, l) && !strings.Contains(ram.Text, l) {
			t.Errorf("%q neither drawn nor shown: barsWhy %q", l, n.BarsWhy)
		}
	}
	if strings.Contains(n.BarsWhy, "RAM: loading") || strings.Contains(n.BarsWhy, "GPU:") {
		t.Errorf("barsWhy repeats a bar: %q", n.BarsWhy)
	}
	// unknown capacity: no scale, the estimate is printed instead
	n = view.MemoryNeed(app.AssessMemory(machine.Memory{}, r), 1)
	if b := n.Bars[1]; b.Total != 0 || b.Size != "1.0 GB–3.0 GB" {
		t.Fatalf("unknown VRAM bar: %+v", b)
	}
	if b := view.MemoryNeed(app.AssessMemory(machine.Memory{}, nil), 1).Bars[0]; len(b.Parts) != 0 || b.Size != "" {
		t.Fatalf("no estimate drew one: %+v", b)
	}
}
