package loadcost

// Baseline returns vanilla + all-DLC resident bytes. Unity 1.6 private bytes:
// 2026-10-31 VM, with measurement-mod contributions separated; see calibration research.
// Callers add these to the mod-only Report.
func Baseline(g Game) (ram, vram int64) {
	const gib = int64(1 << 30)
	if g == Game17 {
		return gib * 23 / 10, gib * 12 / 10
	}
	return gib * 143 / 100, gib
}
