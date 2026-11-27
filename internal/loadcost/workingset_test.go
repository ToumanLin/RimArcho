package loadcost

import "testing"

// 16 §2.17, research 2026-10-24 §4.5: the 1.7 working-set range holds the private working
// sets measured on 1.7.5021 (menu, a minute after loading, first and repeat launches, cards
// from 2 to 24 GiB). The parts are the estimator's totals for those lists (no Mod data).
func TestWorkingSet17Calibration(t *testing.T) {
	const gib = 1 << 30
	report := func(p ...Part) Report { return Report{Parts: p} }
	for _, tc := range []struct {
		name     string
		r        Report
		mods     int
		min, max float64 // measured, GiB
	}{
		{"vanilla", Report{}, 0, 1.15, 1.15},
		{"250 (light)", report(Part{Key: "textures", RAMLow: 362420952, RAMHigh: 892261608, VRAMLow: 217452571, VRAMHigh: 662822909},
			Part{Key: "dds", RAMLow: 212467912, RAMHigh: 495758463, VRAMLow: 127480747, VRAMHigh: 368277715},
			Part{Key: "sounds", RAMLow: 17468363, RAMHigh: 26202545}, Part{Key: "defs", RAMLow: 3788010, RAMHigh: 10101360},
			Part{Key: "assemblies", RAMLow: 52066448, RAMHigh: 130166120}), 235, 1.66, 2.38},
		{"724", report(Part{Key: "textures", RAMLow: 2984969920, RAMHigh: 7032603424, VRAMLow: 1790981952, VRAMHigh: 5224219687},
			Part{Key: "dds", RAMLow: 1331991484, RAMHigh: 3107980131, VRAMLow: 799194890, VRAMHigh: 2308785240},
			Part{Key: "sounds", RAMLow: 42703952, RAMHigh: 64055928}, Part{Key: "defs", RAMLow: 88386201, RAMHigh: 235696536},
			Part{Key: "assemblies", RAMLow: 131985440, RAMHigh: 329963600}), 665, 4.47, 5.12},
		{"#31 subset", report(Part{Key: "textures", RAMLow: 2521382288, RAMHigh: 5953919552, VRAMLow: 1512829372, VRAMHigh: 4422911668},
			Part{Key: "dds", RAMLow: 1597710852, RAMHigh: 3727991988, VRAMLow: 958626511, VRAMHigh: 2769365477},
			Part{Key: "sounds", RAMLow: 14541631, RAMHigh: 21812447}, Part{Key: "defs", RAMLow: 45916071, RAMHigh: 122442856},
			Part{Key: "assemblies", RAMLow: 83009040, RAMHigh: 207522600}), 200, 2.67, 5.29},
	} {
		low, high := WorkingSet(Game17, tc.r, tc.mods)
		t.Logf("%s: estimate %.2f–%.2f GiB, measured %.2f–%.2f", tc.name, float64(low)/gib, float64(high)/gib, tc.min, tc.max)
		if float64(low) > tc.min*gib*1.02 || float64(high) < tc.max*gib*0.98 {
			t.Errorf("%s: %.2f–%.2f GiB does not hold the measured %.2f–%.2f", tc.name, float64(low)/gib, float64(high)/gib, tc.min, tc.max)
		}
	}
}
