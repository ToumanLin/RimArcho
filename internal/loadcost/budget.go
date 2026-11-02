package loadcost

// LoadingBudget estimates startup peak private bytes, not peak working set.
// Unity 1.6: 2026-10-31 baseline/light/texture runs and 2026-11-01 heavy
// textures up to 7.5 GiB payload. The resident curve also shapes this budget.
// See docs/research/2026-11-01_unity16-heavy-texture-memory.md for fit and limits.
// 1.6 excludes the texture VRAM, like Estimate; add Spill for the share held in RAM.
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
		// The reserve was fitted on the payload; VRAM is payload × TextureGPU16.
		payloadLow, payloadHigh := float64(texturesLow)/Coef.TextureGPU16, float64(texturesHigh)/Coef.TextureGPU16
		return r.RAMLow + (9*(1<<30))/10 + int64(payloadLow*0.15) + m.MaxPNGDecode,
			r.RAMHigh + (13*(1<<30))/10 + int64(payloadHigh*0.45) + m.MaxPNGDecode + xml*4 + m.Languages.Bytes*3
	}
	return r.RAMLow + xml*4 + m.MaxPNGDecode, r.RAMHigh + xml*12 + m.MaxPNGDecode + m.Languages.Bytes*3
}

// Spill returns how much of a VRAM estimate (baseline included) the game holds in
// system memory. Unity 1.6 on D3D11 keeps no CPU copy of a loaded texture; the
// driver's allocation stays in dedicated VRAM while it fits. Without dedicated
// memory (a VM, integrated graphics, or an adapter that could not be read:
// dedicated <= 0) all of it is committed process memory, as measured in the VMware
// VM; beyond the card's capacity Windows pages the rest to system memory. 1.7's RAM
// rates were measured on a real GPU and already include its host-side memory, so
// 1.7 spills nothing.
func Spill(g Game, vramLow, vramHigh, dedicated int64) (low, high int64) {
	if g != Game16 {
		return 0, 0
	}
	usable := int64(float64(max(dedicated, 0)) * Coef.UsableVRAM)
	return max(vramLow-usable, 0), max(vramHigh-usable, 0)
}
