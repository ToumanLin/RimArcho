package loadcost

import (
	"context"
	"path/filepath"
	"testing"
)

func TestLoadingBudget720SyntheticMods(t *testing.T) {
	mods := make([]Mod, 720)
	for i := range mods {
		mods[i] = Mod{Defs: Files{Count: 1, Bytes: 512 << 10}, MaxPNGDecode: 16 << 20}
	}
	resident := Estimate(Game17, mods)
	low, high := LoadingBudget(Game17, resident, mods)
	// 720 XML-heavy synthetic mods produce about 3–5 GiB resident including
	// baseline, but loading reserves must not be mistaken for resident bytes.
	baseline, _ := Baseline(Game17)
	if resident.RAMLow+baseline < 3<<30 || resident.RAMHigh+baseline > 6<<30 {
		t.Fatalf("unexpected resident fixture: %+v", resident)
	}
	if low != resident.RAMLow+(720*512<<10)*4+16<<20 || high != resident.RAMHigh+(720*512<<10)*12+16<<20 {
		t.Fatalf("budget %d–%d", low, high)
	}
	if high+baseline <= 8<<30 {
		t.Fatalf("loading reserve omitted: %d", high+baseline)
	}
	t.Logf("720 synthetic mods: resident %.2f–%.2f GiB; loading budget %.2f–%.2f GiB", float64(resident.RAMLow+baseline)/(1<<30), float64(resident.RAMHigh+baseline)/(1<<30), float64(low+baseline)/(1<<30), float64(high+baseline)/(1<<30))
}

func TestLoadingReserveUsesLargestDecodeNotSum(t *testing.T) {
	mods := []Mod{{MaxPNGDecode: 128, Languages: Files{Bytes: 10}}, {MaxPNGDecode: 256, Languages: Files{Bytes: 20}}}
	low, high := LoadingBudget(Game17, Report{RAMLow: 10, RAMHigh: 20}, mods)
	if low != 266 || high != 366 {
		t.Fatalf("reserve should use max decode and language upper only: %d %d", low, high)
	}
	low, high = LoadingBudget(Game17, Report{}, nil)
	if low != 0 || high != 0 {
		t.Fatalf("empty reserve %d %d", low, high)
	}
}

func TestMeasureDecodeReserveRespectsDDSShadowing(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "Textures", "big.png"), 128, 128)
	writePNG(t, filepath.Join(root, "Textures", "small.png"), 32, 32)
	writeBytes(t, filepath.Join(root, "Textures", "big.dds"), dds(false, 64))
	m, err := Measure(context.Background(), root, []string{root}, filepath.Join(t.TempDir(), "cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.PNG.Count != 1 || m.MaxPNGDecode != mipPixels(32, 32)*4 {
		t.Fatalf("shadowed PNG included: %+v", m)
	}
}

// The 1.6 VM curve is split, not changed: RAM + VRAM per bound is the calibrated
// private-byte total, and Spill decides where the VRAM share lives.
func TestUnity16TextureVRAMSplit(t *testing.T) {
	mod := Mod{PNG: Files{Count: 1, Bytes: 1 << 20}, PNGPixelsMip: 4 << 30, DDS: Files{Count: 1, Bytes: 1 << 30}}
	r := Estimate(Game16, []Mod{mod})
	payload := float64(5 << 30)
	rateLow := textureRAM16Rate(payload, Coef.TextureRAM16Low, Coef.TextureRAM16TailLow)
	var ramLow, vramLow int64
	for _, p := range r.Parts {
		if p.Key == "textures" || p.Key == "dds" {
			ramLow += p.RAMLow
			vramLow += p.VRAMLow
		}
	}
	if want := int64(payload * rateLow); ramLow+vramLow < want-2 || ramLow+vramLow > want {
		t.Fatalf("RAM %d + VRAM %d, want the VM total %d", ramLow, vramLow, want)
	}
	if want := int64(payload * Coef.TextureGPU16); vramLow < want-1 || vramLow > want {
		t.Fatalf("VRAM %d, want payload × TextureGPU16 = %d", vramLow, want)
	}
	for _, tc := range []struct {
		name                 string
		game                 Game
		dedicated, low, high int64
	}{
		{"no dedicated VRAM, as in the VM", Game16, 0, 6 << 30, 8 << 30},
		{"unreadable adapter", Game16, -1, 6 << 30, 8 << 30},
		{"card holds everything", Game16, 10 << 30, 0, 0},
		{"card holds part", Game16, 5 << 30, 6<<30 - int64(float64(5<<30)*Coef.UsableVRAM), 8<<30 - int64(float64(5<<30)*Coef.UsableVRAM)},
		{"1.7 keeps its measured RAM rates", Game17, 0, 0, 0},
	} {
		low, high := Spill(tc.game, 6<<30, 8<<30, tc.dedicated)
		if low != tc.low || high != tc.high {
			t.Fatalf("%s: spill %d–%d, want %d–%d", tc.name, low, high, tc.low, tc.high)
		}
	}
}

// The 24 GB card's repeat launches stopped at 15,283 MB of VRAM (research 2026-10-24
// §4.2); the watermark must reproduce that, and an 8 GiB card keeps far less.
func TestWatermark17MatchesMeasuredCap(t *testing.T) {
	const mib = int64(1 << 20)
	if w := Watermark17(24564 * mib); w < 15000*mib || w > 15600*mib {
		t.Fatalf("24 GB watermark %d MiB, measured 15283", w/mib)
	}
	if w := Watermark17(8192 * mib); w < 4800*mib || w > 5200*mib {
		t.Fatalf("8 GiB watermark %d MiB", w/mib)
	}
	if Watermark17(0) != 0 {
		t.Fatal("unknown card has a watermark")
	}
}

func TestCap17(t *testing.T) {
	const gib = int64(1 << 30)
	r := Report{Parts: []Part{
		{Key: "textures", RAMLow: 9 * gib, RAMHigh: 21 * gib, VRAMLow: 5 * gib, VRAMHigh: 15 * gib},
		{Key: "dds", RAMLow: 6 * gib, RAMHigh: 14 * gib, VRAMLow: 4 * gib, VRAMHigh: 11 * gib},
		{Key: "defs", RAMLow: gib, RAMHigh: 2 * gib},
	}}
	if a, b, c, d := Cap17(Game16, r, 8*gib); a+b+c+d != 0 {
		t.Fatal("1.6 capped")
	}
	if a, b, c, d := Cap17(Game17, r, 0); a+b+c+d != 0 {
		t.Fatal("unknown card capped")
	}
	_, base := Baseline(Game17)
	room := Watermark17(8*gib) - base
	ramLow, ramHigh, vramLow, vramHigh := Cap17(Game17, r, 8*gib)
	if 9*gib-vramLow != room || 26*gib-vramHigh != room {
		t.Fatalf("texture VRAM not cut to the room %d: %d %d", room, vramLow, vramHigh)
	}
	if left := 15*gib - ramLow; left != int64(float64(room)*Coef.Tex17RAMPerVRAMLow) {
		t.Fatalf("low RAM left %d", left)
	}
	if left := 35*gib - ramHigh; left != int64(float64(room)*Coef.Tex17RAMPerVRAMHigh) {
		t.Fatalf("high RAM left %d", left)
	}
	// A big card with room cuts nothing.
	if a, b, c, d := Cap17(Game17, Report{Parts: r.Parts[2:]}, 24*gib); a+b+c+d != 0 {
		t.Fatal("cut without textures")
	}
}
