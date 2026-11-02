package loadcost

// Baseline returns vanilla + all-DLC resident bytes. Unity 1.6: 2026-10-31 VM private
// bytes with measurement-mod contributions separated (1.43 GiB), split on 2026-11-01
// into the committed GPU memory measured at the main menu (0.66 GiB, VRAM) and the
// rest (RAM); see the texture memory breakdown research. Callers add these to the
// mod-only Report, and Spill decides how much of the VRAM is held in RAM.
func Baseline(g Game) (ram, vram int64) {
	const gib = int64(1 << 30)
	if g == Game17 {
		return gib * 23 / 10, gib * 12 / 10
	}
	return gib * 77 / 100, gib * 66 / 100
}
