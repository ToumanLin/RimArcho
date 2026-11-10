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
	for _, tc := range []struct {
		name                    string
		total, available, video int64
		ram, vram               string
	}{
		{"ample", 16 * gib, 12 * gib, 8 * gib, "ok", "ok"},
		{"RAM severe", 16 * gib, 6 * gib, 8 * gib, "severe", "ok"},
		{"RAM warning", 16 * gib, 9 * gib, 8 * gib, "warning", "ok"},
		{"lower boundary", 16 * gib, 7 * gib, 8 * gib, "warning", "ok"},
		{"VRAM warning", 16 * gib, 12 * gib, 3 * gib, "ok", "warning"},
		{"both", 16 * gib, 0, 3 * gib, "severe", "warning"},
		{"equal boundary", 16 * gib, 10 * gib, 5 * gib, "ok", "ok"},
		{"unknown hardware", 0, 0, 0, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := machine.Memory{TotalRAM: tc.total, AvailableRAM: tc.available, SampledAt: time.Now()}
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
	if c.VRAMInRAMLow != 4*gib || c.VRAMInRAMHigh != 6*gib || c.RAMLow != 10*gib || c.RAMHigh != 15*gib || c.RAMSeverity != "warning" {
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
