package loadcost

// Baseline returns placeholder vanilla + all-DLC resident bytes for calibration.
// Callers add these to the mod-only Report.
func Baseline(g Game) (ram, vram int64) {
	const gib = int64(1 << 30)
	if g == Game17 {
		return gib * 23 / 10, gib * 12 / 10
	}
	return 2 * gib, gib
}
