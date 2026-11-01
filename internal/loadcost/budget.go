package loadcost

// LoadingBudget estimates startup peak private bytes, not peak working set.
// Unity 1.6: 2026-10-31 baseline/light/texture runs and 2026-11-01 heavy
// textures up to 7.5 GiB payload. The resident curve also shapes this budget.
// See docs/research/2026-11-01_unity16-heavy-texture-memory.md for fit and limits.
// 1.7 retains the uncalibrated XML/decode reserve. Arbitrary Mod allocations,
// AssetBundles, saves and play growth can exceed either model.
func LoadingBudget(g Game, r Report, mods []Mod) (low, high int64) {
	var m Mod
	for _, mod := range mods {
		m.Plus(mod)
	}
	xml := m.Defs.Bytes + m.Patches.Bytes
	if g == Game16 {
		var texturesLow, texturesHigh int64
		for _, p := range r.Parts {
			if p.Key == "textures" || p.Key == "dds" {
				texturesLow += p.VRAMLow
				texturesHigh += p.VRAMHigh
			}
		}
		return r.RAMLow + (9*(1<<30))/10 + texturesLow*15/100 + m.MaxPNGDecode,
			r.RAMHigh + (13*(1<<30))/10 + texturesHigh*45/100 + m.MaxPNGDecode + xml*4 + m.Languages.Bytes*3
	}
	return r.RAMLow + xml*4 + m.MaxPNGDecode, r.RAMHigh + xml*12 + m.MaxPNGDecode + m.Languages.Bytes*3
}
