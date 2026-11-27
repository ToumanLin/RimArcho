package loadcost

// WS holds the working-set coefficients (16 §2.17). The working set here is the private
// working set: the game's own memory in physical RAM, what Task Manager's Memory column
// shows and what has to fit in the RAM available now. The Report's RAM ranges are private
// bytes (commit), which Windows backs with RAM plus the page file and which run higher.
// Fit and measurements: docs/research/2026-10-24_small-file-reads-and-load-memory.md §4.5–4.6.
var WS = struct {
	Base17                    int64   // vanilla + all DLC at the main menu, 1.7.5021
	Tex17High                 float64 // per byte of texture + DDS payload, upper bound only
	Tex17Cap                  int64   // where the texture term stops growing
	PerMod17Low, PerMod17High int64   // per non-official Mod
	Other17Low, Other17High   float64 // per byte of the other parts' private-bytes range
	Other16Low, Other16High   float64 // 1.6: per byte of the RAM estimate (CPU-side memory)
}{
	Base17:       1180 << 20,
	Tex17High:    1.6,
	Tex17Cap:     7 << 29, // 3.5 GiB
	PerMod17Low:  400 << 10,
	PerMod17High: 2 << 20,
	Other17Low:   1,
	Other17High:  1,
	Other16Low:   0.75,
	Other16High:  0.95,
}

// WorkingSet estimates the game's private working set a minute after it reached the main
// menu, first or repeat launch, the vanilla baseline included, for a mod-only Report of mods
// non-official Mods.
//
// 1.7 keeps no CPU copy of a texture and uploads on demand, so the settled working set
// hardly follows the texture payload: from 0.9 to 18.8 GiB of payload the repeat launch
// stayed within 1.2 GiB above vanilla, and the first launch (textures encoded into the
// cache) within 3.2 GiB (calibration §4.6). The texture term is therefore 0 at the lower
// bound and Tex17High × the payload, capped at Tex17Cap, at the upper bound. Neither figure
// follows the card (§4.5). 1.6 takes a share of its RAM estimate; the caller adds the VRAM
// 1.6 holds in RAM (Spill). An empty Report gives the baseline.
func WorkingSet(g Game, r Report, mods int) (low, high int64) {
	if g == Game17 {
		var l, h, payload float64
		for _, p := range r.Parts {
			if p.Key == "textures" || p.Key == "dds" {
				// the payload: the VRAM range divided by its per-payload rate
				payload += float64(p.VRAMHigh) / Coef.Tex17VRAMHigh
				continue
			}
			l += float64(p.RAMLow) * WS.Other17Low
			h += float64(p.RAMHigh) * WS.Other17High
		}
		h += min(payload*WS.Tex17High, float64(WS.Tex17Cap))
		n := int64(max(mods, 0))
		return WS.Base17 + n*WS.PerMod17Low + int64(l), WS.Base17 + n*WS.PerMod17High + int64(h)
	}
	base, _ := Baseline(g)
	return int64(float64(base+r.RAMLow) * WS.Other16Low), int64(float64(base+r.RAMHigh) * WS.Other16High)
}
