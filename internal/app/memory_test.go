package app

import (
	"testing"
	"time"

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
