package app

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/machine"
)

// issue31Cost is a 1.7 list shaped like GitHub #31's (615 Mods, about 18.7 GB of texture
// payload as the estimator measured it: 9.7 GB VRAM kept on a 16 GB card, 8.3 GB on demand).
func issue31Cost() *LoadCost {
	const gib = int64(1 << 30)
	payload := 18.7 * float64(gib)
	tex := loadcost.Part{Key: "textures", RAMLow: int64(payload * 1.5), RAMHigh: int64(payload * 3.5), VRAMLow: int64(payload * 0.9), VRAMHigh: int64(payload * 2.6)}
	defs := loadcost.Part{Key: "defs", RAMLow: gib / 2, RAMHigh: gib * 13 / 10}
	r := loadcost.Report{Parts: []loadcost.Part{tex, defs}}
	for _, p := range r.Parts {
		r.RAMLow += p.RAMLow
		r.RAMHigh += p.RAMHigh
		r.VRAMLow += p.VRAMLow
		r.VRAMHigh += p.VRAMHigh
	}
	c := &LoadCost{Report: r, Game: loadcost.Game17, Mods: 615}
	c.LoadingRAMLow, c.LoadingRAMHigh = loadcost.LoadingBudget(loadcost.Game17, r, nil)
	c.Baseline.RAM, c.Baseline.VRAM = loadcost.Baseline(loadcost.Game17)
	return c
}

// GitHub #31: the RAM figure and the warning use the expected working set; private bytes
// (commit) only decide the severe level, against what can still be committed.
func TestMemoryIssue31Shape(t *testing.T) {
	const gib = int64(1 << 30)
	cost := issue31Cost()
	hw := func(available, commit int64) machine.Memory {
		return machine.Memory{TotalRAM: gib * 319 / 10, AvailableRAM: available, CommitAvailable: commit, SampledAt: time.Now(),
			Adapters: []machine.Adapter{{Name: "synthetic 16 GB card", Dedicated: gib * 156 / 10}}}
	}
	c := AssessMemory(hw(8*gib, 40*gib), cost)
	t.Logf("working set %.1f–%.1f GiB; loading commit %.1f–%.1f GiB; VRAM %.1f–%.1f GiB, on demand %.1f GiB",
		float64(c.WorkingSetLow)/float64(gib), float64(c.WorkingSetHigh)/float64(gib), float64(c.RAMLow)/float64(gib), float64(c.RAMHigh)/float64(gib),
		float64(c.VRAMLow)/float64(gib), float64(c.VRAMHigh)/float64(gib), float64(c.TexturesOnDemand)/float64(gib))
	// the headline: below what the game reserves, never above it
	if c.WorkingSetLow <= 0 || c.WorkingSetHigh > c.RAMHigh || c.WorkingSetLow > c.RAMLow || c.WorkingSetHigh >= c.RAMHigh && c.WorkingSetLow >= c.RAMLow {
		t.Fatalf("working set %d–%d against commit %d–%d", c.WorkingSetLow, c.WorkingSetHigh, c.RAMLow, c.RAMHigh)
	}
	// research §4.6: the texture term saturates, so the fitted working set is about 1.9–7.2 GiB
	// and 8 GB free is enough. While loading it commits 2.2 GiB (repeat launch) to 29.7 GiB
	// (first launch, textures encoded into the cache), on any card
	near := func(v int64, want float64) bool {
		return float64(v) > (want-0.1)*float64(gib) && float64(v) < (want+0.1)*float64(gib)
	}
	if !near(c.WorkingSetLow, 1.9) || !near(c.WorkingSetHigh, 7.2) {
		t.Fatalf("working set %.2f–%.2f GiB, want about 1.9–7.2", float64(c.WorkingSetLow)/float64(gib), float64(c.WorkingSetHigh)/float64(gib))
	}
	if c.RAMSeverity != "ok" {
		t.Fatalf("8 GB free: %s with working set up to %d", c.RAMSeverity, c.WorkingSetHigh)
	}
	// 7 GB free is below the fitted upper bound: the warning returns
	if w := AssessMemory(hw(7*gib, 40*gib), cost); w.RAMSeverity != "warning" {
		t.Fatalf("7 GB free: %s", w.RAMSeverity)
	}
	// free RAM between the working set and the commit: the old rule warned, this one does not
	c = AssessMemory(hw(c.WorkingSetHigh, 40*gib), cost)
	if c.RAMSeverity != "ok" || c.RAMBudget >= c.RAMHigh {
		t.Fatalf("room for the working set but not the commit: %s", c.RAMSeverity)
	}
	// severe is unchanged: the loading commit's lower bound against what can still be committed
	if c = AssessMemory(hw(30*gib, c.RAMLow-1), cost); c.RAMSeverity != "severe" {
		t.Fatalf("commit below the loading budget: %s", c.RAMSeverity)
	}
	if c = AssessMemory(hw(30*gib, c.RAMLow), cost); c.RAMSeverity != "ok" {
		t.Fatalf("commit at the loading budget: %s", c.RAMSeverity)
	}
	if !strings.Contains(c.Hardware.Adapters[0].Name, "16 GB") {
		t.Fatal("fixture")
	}
}

// A list whose working set fits: 8 GB free, no warning, though the commit is above it.
func TestMemoryWarningFollowsWorkingSet(t *testing.T) {
	const gib = int64(1 << 30)
	r := loadcost.Report{Parts: []loadcost.Part{{Key: "textures", RAMLow: 4 * gib, RAMHigh: 9 * gib, VRAMLow: 2 * gib, VRAMHigh: 6 * gib}}}
	r.RAMLow, r.RAMHigh, r.VRAMLow, r.VRAMHigh = 4*gib, 9*gib, 2*gib, 6*gib
	cost := &LoadCost{Report: r, Game: loadcost.Game17, Mods: 600, LoadingRAMLow: 5 * gib, LoadingRAMHigh: 11 * gib}
	cost.Baseline.RAM, cost.Baseline.VRAM = loadcost.Baseline(loadcost.Game17)
	m := machine.Memory{TotalRAM: 32 * gib, AvailableRAM: 8 * gib, CommitAvailable: 30 * gib, SampledAt: time.Now(),
		Adapters: []machine.Adapter{{Name: "synthetic", Dedicated: 16 * gib}}}
	c := AssessMemory(m, cost)
	if c.WorkingSetHigh >= 8*gib || c.RAMHigh <= 8*gib || c.RAMSeverity != "ok" {
		t.Fatalf("working set %d, commit %d, %s", c.WorkingSetHigh, c.RAMHigh, c.RAMSeverity)
	}
}
