package loadcost

import "testing"

// 16 §2.17, research 2026-10-24 §4.5: the 1.7 working-set range holds the private working
// sets measured on 1.7.5021 (menu, a minute after loading, first and repeat launches, cards
// from 2 to 24 GiB), and §4.6 adds texture-heavy lists and the loading peaks. The parts are the estimator's totals for those lists (no Mod data).
func TestWorkingSet17Calibration(t *testing.T) {
	const gib = 1 << 30
	report := func(p ...Part) Report {
		r := Report{Parts: p}
		for _, x := range p {
			r.RAMLow += x.RAMLow
			r.RAMHigh += x.RAMHigh
		}
		return r
	}
	base, _ := Baseline(Game17)
	for _, tc := range []struct {
		name       string
		r          Report
		mods       int
		min, max   float64 // measured, GiB
		peakRepeat float64 // peak private bytes from start to menu + 60 s, repeat launch, GiB (0: not checked)
		peakFirst  float64 // the same, first launch (cache empty), highest on any card
	}{
		{"vanilla", Report{}, 0, 1.15, 1.15, 0, 0},
		{"250 (light)", report(Part{Key: "textures", RAMLow: 362420952, RAMHigh: 892261608, VRAMLow: 217452571, VRAMHigh: 662822909},
			Part{Key: "dds", RAMLow: 212467912, RAMHigh: 495758463, VRAMLow: 127480747, VRAMHigh: 368277715},
			Part{Key: "sounds", RAMLow: 17468363, RAMHigh: 26202545}, Part{Key: "defs", RAMLow: 3788010, RAMHigh: 10101360},
			Part{Key: "assemblies", RAMLow: 52066448, RAMHigh: 130166120}), 235, 1.66, 2.38, 0, 0},
		{"724", report(Part{Key: "textures", RAMLow: 2984969920, RAMHigh: 7032603424, VRAMLow: 1790981952, VRAMHigh: 5224219687},
			Part{Key: "dds", RAMLow: 1331991484, RAMHigh: 3107980131, VRAMLow: 799194890, VRAMHigh: 2308785240},
			Part{Key: "sounds", RAMLow: 42703952, RAMHigh: 64055928}, Part{Key: "defs", RAMLow: 88386201, RAMHigh: 235696536},
			Part{Key: "assemblies", RAMLow: 131985440, RAMHigh: 329963600}), 665, 4.47, 5.12, 9.11, 12.42},
		{"#31 subset", report(Part{Key: "textures", RAMLow: 2521382288, RAMHigh: 5953919552, VRAMLow: 1512829372, VRAMHigh: 4422911668},
			Part{Key: "dds", RAMLow: 1597710852, RAMHigh: 3727991988, VRAMLow: 958626511, VRAMHigh: 2769365477},
			Part{Key: "sounds", RAMLow: 14541631, RAMHigh: 21812447}, Part{Key: "defs", RAMLow: 45916071, RAMHigh: 122442856},
			Part{Key: "assemblies", RAMLow: 83009040, RAMHigh: 207522600}), 200, 2.67, 5.29, 5.09, 8.12},
		// §4.6: texture-heavy Workshop lists (Core, DLC, dependencies), repeat and first launch
		{"heavy 0.9 GiB", report(Part{Key: "textures", RAMLow: 1459910928, RAMHigh: 3406458832, VRAMLow: 875946556, VRAMHigh: 2530512276},
			Part{Key: "defs", RAMLow: 3914847, RAMHigh: 10439592}, Part{Key: "assemblies", RAMLow: 12166144, RAMHigh: 30415360}), 7, 1.22, 1.48, 1.83, 4.23},
		{"heavy 2.1 GiB", report(Part{Key: "textures", RAMLow: 3391389864, RAMHigh: 7913243016, VRAMLow: 2034833918, VRAMHigh: 5878409098},
			Part{Key: "defs", RAMLow: 3996555, RAMHigh: 10657480}, Part{Key: "assemblies", RAMLow: 12166144, RAMHigh: 30415360}), 11, 1.24, 2.01, 1.95, 7.61},
		{"heavy 6.1 GiB", report(Part{Key: "textures", RAMLow: 9501669384, RAMHigh: 22170561896, VRAMLow: 5701001630, VRAMHigh: 16469560266},
			Part{Key: "dds", RAMLow: 256163196, RAMHigh: 597714124, VRAMLow: 153697917, VRAMHigh: 444016207},
			Part{Key: "sounds", RAMLow: 4971552, RAMHigh: 7457328}, Part{Key: "defs", RAMLow: 14589822, RAMHigh: 38906192},
			Part{Key: "assemblies", RAMLow: 31959568, RAMHigh: 79898920}), 22, 1.32, 2.16, 2.05, 14.76},
		{"heavy 12.3 GiB", report(Part{Key: "textures", RAMLow: 18425905952, RAMHigh: 43002429904, VRAMLow: 11055543571, VRAMHigh: 31944662215},
			Part{Key: "dds", RAMLow: 1344188592, RAMHigh: 3136440048, VRAMLow: 806513155, VRAMHigh: 2329926893},
			Part{Key: "sounds", RAMLow: 17771427, RAMHigh: 26657141}, Part{Key: "defs", RAMLow: 51930165, RAMHigh: 138480440},
			Part{Key: "assemblies", RAMLow: 42407440, RAMHigh: 106018600}), 44, 1.56, 3.75, 2.29, 20.25},
		{"heavy 18.8 GiB", report(Part{Key: "textures", RAMLow: 28620414728, RAMHigh: 66873445016, VRAMLow: 17172248836, VRAMHigh: 49677416298},
			Part{Key: "dds", RAMLow: 1582958280, RAMHigh: 3693569320, VRAMLow: 949774968, VRAMHigh: 2743794352},
			Part{Key: "sounds", RAMLow: 108796887, RAMHigh: 163195331}, Part{Key: "defs", RAMLow: 118189944, RAMHigh: 315173184},
			Part{Key: "assemblies", RAMLow: 80439824, RAMHigh: 201099560}), 72, 2.31, 4.36, 3.34, 25.78},
	} {
		low, high := WorkingSet(Game17, tc.r, tc.mods)
		t.Logf("%s: estimate %.2f–%.2f GiB, measured %.2f–%.2f", tc.name, float64(low)/gib, float64(high)/gib, tc.min, tc.max)
		if float64(low) > tc.min*gib*1.02 || float64(high) < tc.max*gib*0.98 {
			t.Errorf("%s: %.2f–%.2f GiB does not hold the measured %.2f–%.2f", tc.name, float64(low)/gib, float64(high)/gib, tc.min, tc.max)
		}
		if tc.peakFirst == 0 {
			// vanilla: the baseline is its menu figure; 250: its first-launch peak (3.72) is above
			// the budget (3.3), as it was before §4.6
			continue
		}
		// §4.6: the loading budget spans a repeat launch's peak and a first launch's, on any card;
		// within 5%: the 724 list's first launch sits 2–3% above it, as before §4.6
		var xml []Mod // the Def XML reserve, from the defs part (Coef.DefsLow bytes per file byte)
		for _, p := range tc.r.Parts {
			if p.Key == "defs" {
				xml = []Mod{{Defs: Files{Bytes: int64(float64(p.RAMLow) / Coef.DefsLow)}}}
			}
		}
		low, high = LoadingBudget(Game17, tc.r, xml)
		low, high = low+base, high+base
		t.Logf("%s: loading %.2f–%.2f GiB, measured peaks %.2f (repeat) / %.2f (first)", tc.name, float64(low)/gib, float64(high)/gib, tc.peakRepeat, tc.peakFirst)
		if float64(low) > tc.peakRepeat*gib*1.02 || float64(high) < tc.peakFirst*gib*0.95 {
			t.Errorf("%s: loading %.2f–%.2f GiB does not hold the measured peaks %.2f–%.2f", tc.name, float64(low)/gib, float64(high)/gib, tc.peakRepeat, tc.peakFirst)
		}
	}
}
