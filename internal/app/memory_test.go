package app

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/machine"
)

func TestMemoryCapacityPolicy(t *testing.T) {
	const gib = int64(1 << 30)
	cost := &LoadCost{LoadingRAMLow: 5 * gib, LoadingRAMHigh: 8 * gib, Baseline: LoadCostBaseline{RAM: 2 * gib, VRAM: gib}}
	cost.VRAMLow = 2 * gib
	cost.VRAMHigh = 4 * gib
	ws := AssessMemory(machine.Memory{TotalRAM: 16 * gib, AvailableRAM: 12 * gib, SampledAt: time.Now()}, cost).WorkingSetHigh
	if ws <= 0 || ws >= 7*gib {
		t.Fatalf("working set %d", ws)
	}
	for _, tc := range []struct {
		name                            string
		total, available, commit, video int64
		ram, vram                       string
	}{
		{"ample", 16 * gib, 12 * gib, 0, 8 * gib, "ok", "ok"},
		{"RAM severe", 16 * gib, 6 * gib, 0, 8 * gib, "severe", "ok"},
		{"commit severe", 16 * gib, 12 * gib, 6 * gib, 8 * gib, "severe", "ok"},
		// below the loading commit but above the working set: the commit has room, no warning
		{"commit room", 16 * gib, ws, 20 * gib, 8 * gib, "ok", "ok"},
		{"RAM warning", 16 * gib, ws - 1, 20 * gib, 8 * gib, "warning", "ok"},
		{"VRAM warning", 16 * gib, 12 * gib, 0, 3 * gib, "ok", "warning"},
		{"both", 16 * gib, 0, 0, 3 * gib, "severe", "warning"},
		{"equal boundary", 16 * gib, 10 * gib, 0, 5 * gib, "ok", "ok"},
		{"unknown hardware", 0, 0, 0, 0, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := machine.Memory{TotalRAM: tc.total, AvailableRAM: tc.available, CommitAvailable: tc.commit, SampledAt: time.Now()}
			if tc.video > 0 {
				m.Adapters = []machine.Adapter{{Name: "synthetic GPU", Dedicated: tc.video}}
			}
			c := AssessMemory(m, cost)
			if c.RAMSeverity != tc.ram || c.VRAMSeverity != tc.vram || c.RAMBudget != tc.available {
				t.Fatalf("assessment %+v", c)
			}
			if c.RAMHigh != 10*gib || c.VRAMHigh != 5*gib {
				t.Fatalf("baseline excluded: %+v", c)
			}
		})
	}
	c := AssessMemory(machine.Memory{TotalRAM: 16 * gib}, nil)
	if c.Estimated || c.RAMSeverity != "unknown" || c.VRAMSeverity != "unknown" {
		t.Fatalf("missing estimate %+v", c)
	}
}

// Unity 1.6 holds VRAM in RAM only when the card cannot: all of it without a
// readable dedicated adapter (the VM's case), none of it on a card with room.
func TestMemoryCapacityHoldsUnity16VRAMInRAM(t *testing.T) {
	const gib = int64(1 << 30)
	cost := &LoadCost{Game: loadcost.Game16, LoadingRAMLow: 5 * gib, LoadingRAMHigh: 8 * gib, Baseline: LoadCostBaseline{RAM: gib, VRAM: gib}}
	cost.VRAMLow, cost.VRAMHigh = 3*gib, 5*gib
	m := machine.Memory{TotalRAM: 32 * gib, AvailableRAM: 14 * gib, SampledAt: time.Now()}
	c := AssessMemory(m, cost)
	// the held VRAM is in the working set too; 14 GiB available holds it (no warning)
	if c.VRAMInRAMLow != 4*gib || c.VRAMInRAMHigh != 6*gib || c.RAMLow != 10*gib || c.RAMHigh != 15*gib || c.RAMSeverity != "ok" || c.WorkingSetHigh < 6*gib || c.WorkingSetHigh > 14*gib {
		t.Fatalf("no adapter: all VRAM is RAM: %+v", c)
	}
	m.Adapters = []machine.Adapter{{Name: "synthetic GPU", Dedicated: 24 * gib}}
	c = AssessMemory(m, cost)
	if c.VRAMInRAMHigh != 0 || c.RAMLow != 6*gib || c.RAMHigh != 9*gib || c.RAMSeverity != "ok" {
		t.Fatalf("24 GiB card holds the textures: %+v", c)
	}
}

func TestMemoryAvailableUnknownIsNotZero(t *testing.T) {
	m := machine.Memory{TotalRAM: 8 << 30, RAMAvailableUnknown: true, SampledAt: time.Unix(1, 0)}
	c := AssessMemory(m, &LoadCost{LoadingRAMLow: 2 << 30, LoadingRAMHigh: 4 << 30})
	if c.RAMSeverity != "unknown" || c.VRAMSeverity != "unknown" {
		t.Fatalf("unknown memory warned: %+v", c)
	}
}

// A token dedicated size (VMware SVGA's 4 MB, Intel integrated's ~128 MB) counts as an
// unknown card (machine.MinDedicatedVRAM): 1.7 stays uncapped, the VRAM capacity is
// unknown. A real card keeps its watermark cap.
func TestMemoryTokenDedicatedVRAMIsUnknown(t *testing.T) {
	const gib, mib = int64(1 << 30), int64(1 << 20)
	r := loadcost.Report{RAMLow: 3 * gib, RAMHigh: 6 * gib, VRAMLow: 2 * gib, VRAMHigh: 5 * gib, Parts: []loadcost.Part{
		{Key: "textures", RAMLow: 2 * gib, RAMHigh: 5 * gib, VRAMLow: 2 * gib, VRAMHigh: 5 * gib},
		{Key: "defs", RAMLow: gib, RAMHigh: gib},
	}}
	cost := &LoadCost{Game: loadcost.Game17, Report: r, LoadingRAMLow: 4 * gib, LoadingRAMHigh: 8 * gib}
	cost.Baseline.RAM, cost.Baseline.VRAM = loadcost.Baseline(loadcost.Game17)
	assess := func(dedicated int64) MemoryAssessment {
		m := machine.Memory{TotalRAM: 16 * gib, AvailableRAM: 8 * gib, SampledAt: time.Now()}
		if dedicated > 0 {
			m.Adapters = []machine.Adapter{{Name: "synthetic GPU", Dedicated: dedicated}}
		}
		return AssessMemory(m, cost)
	}
	unknown := assess(0)
	if unknown.TexturesOnDemand != 0 || unknown.VRAMSeverity != "unknown" {
		t.Fatalf("unknown card capped: %+v", unknown)
	}
	for _, d := range []int64{4 * mib, 128 * mib, machine.MinDedicatedVRAM - 1} {
		c := assess(d)
		if c.RAMLow != unknown.RAMLow || c.RAMHigh != unknown.RAMHigh || c.VRAMLow != unknown.VRAMLow || c.VRAMHigh != unknown.VRAMHigh ||
			c.TexturesOnDemand != 0 || c.VRAMSeverity != "unknown" || c.Hardware.VRAM() != 0 {
			t.Fatalf("%d MiB dedicated differs from unknown: %+v vs %+v", d/mib, c, unknown)
		}
	}
	c := assess(2 * gib)
	_, ramHigh, _, vramHigh := loadcost.Cap17(loadcost.Game17, r, 2*gib)
	if ramHigh == 0 || c.RAMHigh != unknown.RAMHigh-ramHigh || c.VRAMHigh != unknown.VRAMHigh-vramHigh || c.TexturesOnDemand == 0 || c.VRAMSeverity != "ok" {
		t.Fatalf("2 GiB card not capped as before: %+v", c)
	}
}
