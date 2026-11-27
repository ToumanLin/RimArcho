package loadcost

// WS holds the working-set coefficients (16 §2.17). The working set here is the private
// working set: the game's own memory in physical RAM, what Task Manager's Memory column
// shows and what has to fit in the RAM available now. The Report's RAM ranges are private
// bytes (commit), which Windows backs with RAM plus the page file and which run higher.
// Fit and measurements: docs/research/2026-10-24_small-file-reads-and-load-memory.md §4.5.
var WS = struct {
	Base17                    int64   // vanilla + all DLC at the main menu, 1.7.5021
	Tex17Low, Tex17High       float64 // per byte of texture + DDS payload, independent of the card
	PerMod17Low, PerMod17High int64   // per non-official Mod
	Other17Low, Other17High   float64 // per byte of the other parts' private-bytes range
	Other16Low, Other16High   float64 // 1.6: per byte of the RAM estimate (CPU-side memory)
}{
	Base17:       1180 << 20,
	Tex17Low:     0.5,
	Tex17High:    1.6,
	PerMod17Low:  400 << 10,
	PerMod17High: 2 << 20,
	Other17Low:   1,
	Other17High:  1,
	Other16Low:   0.75,
	Other16High:  0.95,
}

// WorkingSet estimates the game's private working set a minute after it reached the main
// menu, the vanilla baseline included, for a mod-only Report of mods non-official Mods.
// 1.7 does not scale it with the graphics card: from 24 GiB down to an emulated 2 GiB card
// the measured working set stayed within 0.6 GiB (calibration §4.5), as did private bytes;
// textures the engine evicts live in its disk cache, which it maps as shared pages. 1.6
// takes a share of its RAM estimate; the caller adds the VRAM 1.6 holds in RAM (Spill).
// An empty Report gives the baseline.
func WorkingSet(g Game, r Report, mods int) (low, high int64) {
	if g == Game17 {
		var l, h float64
		for _, p := range r.Parts {
			if p.Key == "textures" || p.Key == "dds" {
				// the payload: the VRAM range divided by its per-payload rates
				l += float64(p.VRAMLow) / Coef.Tex17VRAMLow * WS.Tex17Low
				h += float64(p.VRAMHigh) / Coef.Tex17VRAMHigh * WS.Tex17High
				continue
			}
			l += float64(p.RAMLow) * WS.Other17Low
			h += float64(p.RAMHigh) * WS.Other17High
		}
		n := int64(max(mods, 0))
		return WS.Base17 + n*WS.PerMod17Low + int64(l), WS.Base17 + n*WS.PerMod17High + int64(h)
	}
	base, _ := Baseline(g)
	return int64(float64(base+r.RAMLow) * WS.Other16Low), int64(float64(base+r.RAMHigh) * WS.Other16High)
}
