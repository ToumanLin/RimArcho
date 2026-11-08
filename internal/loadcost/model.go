package loadcost

import "math"

// Game selects the loading rules used by Estimate.
type Game int

const (
	Game16 Game = 16
	Game17 Game = 17
)

// Coef holds all model coefficients. Texture rates are bytes per pixel; unknown
// dimensions are pixels per file byte, with a geometric mip-chain multiplier.
// The Tex17 rates multiply the 1.7 texture payload (BC7 with mips, plus DDS payload);
// they are measured, not derived: docs/research/2026-10-24_small-file-reads-and-load-memory.md §4.
// RAM rates multiply the texture payload, XML/DLL bytes, or sound bytes.
// Sound16Decoded rates multiply PCM16 bytes; streamed sounds use a fixed buffer.
// AIFF-family and unreadable WAV files approximate PCM16 by file size.
// Re-measure cached Mods after changing audio thresholds or CompressedPCM16.
// Configure before concurrent use; concurrent mutation is unsupported.
var Coef = struct {
	CompressedBytesPerPixel float64
	RawBytesPerPixel        float64
	AtlasBytesPerPixel      float64
	UnknownPixelsLow        float64
	UnknownPixelsHigh       float64
	UnknownMips             float64
	Tex17VRAMLow            float64
	Tex17VRAMHigh           float64
	Tex17RAMLow             float64
	Tex17RAMHigh            float64
	Tex17Budget             float64
	Tex17RAMPerVRAMLow      float64
	Tex17RAMPerVRAMHigh     float64
	TextureRAM16Low         float64
	TextureRAM16High        float64
	TextureRAM16Knee        int64
	TextureRAM16TailLow     float64
	TextureRAM16TailHigh    float64
	TextureGPU16            float64
	UsableVRAM              float64
	CompressedPCM16         float64
	Sound16FileLimit        int64
	Sound17PCM16Limit       int64
	Sound16DecodedLow       float64
	Sound16DecodedHigh      float64
	Sound16StreamBuffer     int64
	Sound17ResidentLow      float64
	Sound17ResidentHigh     float64
	DefsLow                 float64
	DefsHigh                float64
	PatchesLow              float64
	PatchesHigh             float64
	AssembliesLow           float64
	AssembliesHigh          float64
}{
	CompressedBytesPerPixel: 1, // BC7 (1.7) or DXT (1.6), with mips measured separately.
	RawBytesPerPixel:        4, // RGBA32: tiny PNGs in 1.7; non-block-aligned PNGs in 1.6.
	AtlasBytesPerPixel:      1, // Extra 1.6 static atlas copy, upper bound only.
	UnknownPixelsLow:        4, // JPG/PSD dimensions: midpoint would be 8 pixels/file byte.
	UnknownPixelsHigh:       12,
	UnknownMips:             4.0 / 3,
	// 1.7, RTX 4090, 100 / 250 / 484 mods, first and later launches: VRAM 0.9–2.6× and
	// private bytes 1.5–3.5× the payload (the engine also keeps Vulkan host memory).
	Tex17VRAMLow:  0.9,
	Tex17VRAMHigh: 2.6,
	Tex17RAMLow:   1.5,
	Tex17RAMHigh:  3.5,
	// Those rates hold only while the card has room: 1.7 uploads textures on demand and
	// stops at a watermark of the driver's VRAM budget (Cap17). The 24 GB run's repeat
	// launches stopped at 15.3 GB, which the watermark gives for a budget of 0.78 × the
	// dedicated memory. Across the measured runs, mod RAM was 1.1–2.1× the mod VRAM
	// actually resident (repeat launches 1.1–1.5, the 484-mod first launch 2.1).
	Tex17Budget:         0.78,
	Tex17RAMPerVRAMLow:  1.1,
	Tex17RAMPerVRAMHigh: 2.1,
	// Unity 1.6 VM calibration: 2026-10-31 baseline and light/texture groups,
	// extended on 2026-11-01 to 7.5 GiB combined texture payload. Above the
	// original 3 GiB coverage, retained private bytes grow more slowly.
	// See docs/research/2026-11-01_unity16-heavy-texture-memory.md.
	TextureRAM16Low:      1.6,
	TextureRAM16High:     1.8,
	TextureRAM16Knee:     3 << 30,
	TextureRAM16TailLow:  1.3,
	TextureRAM16TailHigh: 1.6,
	// Part of that VM curve is the driver's copy of each texture: GPU memory, which
	// the VMware adapter (no dedicated VRAM) commits as process memory. Per payload
	// byte, committed GPU memory grew by TextureGPU16; Estimate moves it to VRAM and
	// Spill returns the share that does not fit in UsableVRAM of the dedicated memory.
	// See docs/research/2026-11-01_unity16-texture-memory-breakdown.md.
	TextureGPU16:        1.2,
	UsableVRAM:          0.9, // Assumption: other programs and the desktop keep about a tenth.
	CompressedPCM16:     11,  // OGG/MP3: roughly 128 kbit/s versus 1411 kbit/s PCM16.
	Sound16FileLimit:    300 << 10,
	Sound17PCM16Limit:   2 << 20,
	Sound16DecodedLow:   1, // PCM16 versus float32 at the upper bound.
	Sound16DecodedHigh:  2,
	Sound16StreamBuffer: 64 << 10,
	Sound17ResidentLow:  1, // File bytes of the decoded-size-selected subset.
	Sound17ResidentHigh: 1.5,
	DefsLow:             3,
	DefsHigh:            8,
	PatchesLow:          0.5, // The merged XmlDocument is freed after loading.
	PatchesHigh:         2,
	AssembliesLow:       2,
	AssembliesHigh:      5,
}

// Part attributes a range to one resource category. Bytes is on-disk size,
// except for DDS payload bytes. Languages are informational and not modeled.
type Part struct {
	Key      string `json:"key"`
	Count    int    `json:"count"`
	Bytes    int64  `json:"bytes"`
	RAMLow   int64  `json:"ramLow"`
	RAMHigh  int64  `json:"ramHigh"`
	VRAMLow  int64  `json:"vramLow"`
	VRAMHigh int64  `json:"vramHigh"`
}

// Report contains mod-only byte ranges; callers add a measured vanilla baseline.
type Report struct {
	RAMLow   int64  `json:"ramLow"`
	RAMHigh  int64  `json:"ramHigh"`
	VRAMLow  int64  `json:"vramLow"`
	VRAMHigh int64  `json:"vramHigh"`
	Parts    []Part `json:"parts"`
}

// Estimate combines measurements using Coef. It panics for an unsupported Game.
// Bounds round outward to whole bytes, after summing all mods in each part.
func Estimate(g Game, mods []Mod) Report {
	if g != Game16 && g != Game17 {
		panic("loadcost: unsupported game version")
	}
	var m Mod
	for _, mod := range mods {
		m.Plus(mod)
	}
	c := Coef
	textures := m.PNG
	textures.add(m.JPG)
	textures.add(m.PSD)
	unknown := float64(m.JPG.Bytes+m.PSD.Bytes) * c.UnknownMips * c.CompressedBytesPerPixel
	png := float64(m.PNGPixelsMip) * c.CompressedBytesPerPixel
	raw := m.PNGNot4
	if g == Game17 {
		raw = m.PNGRaw16.PixelsMip
	}
	png += float64(raw) * (c.RawBytesPerPixel - c.CompressedBytesPerPixel)
	vLow := png + unknown*c.UnknownPixelsLow
	vHigh := png + unknown*c.UnknownPixelsHigh
	dLow, dHigh := float64(m.DDS.Bytes), float64(m.DDS.Bytes)
	// 1.6 keeps small originals resident beside their static atlas copy.
	if g == Game16 {
		vHigh += float64(m.PNGSmall512Pixels) * c.AtlasBytesPerPixel
	}
	rateLow, rateHigh := c.TextureRAM16Low, c.TextureRAM16High
	if g == Game16 {
		// PNG and DDS share one curve; attribute the result by payload share.
		rateLow = textureRAM16Rate(vLow+dLow, c.TextureRAM16Low, c.TextureRAM16TailLow)
		rateHigh = textureRAM16Rate(vHigh+dHigh, c.TextureRAM16High, c.TextureRAM16TailHigh)
	}
	tRAMLow, tRAMHigh := vLow*rateLow, vHigh*rateHigh
	dRAMLow, dRAMHigh := dLow*rateLow, dHigh*rateHigh
	if g == Game16 {
		// The calibrated curve includes the driver's copy (committed GPU memory in the
		// VM). Report it as VRAM; RAM keeps the engine and Mono share.
		gpu := c.TextureGPU16
		tRAMLow, tRAMHigh = math.Max(0, tRAMLow-vLow*gpu), math.Max(0, tRAMHigh-vHigh*gpu)
		dRAMLow, dRAMHigh = math.Max(0, dRAMLow-dLow*gpu), math.Max(0, dRAMHigh-dHigh*gpu)
		vLow, vHigh, dLow, dHigh = vLow*gpu, vHigh*gpu, dLow*gpu, dHigh*gpu
	}
	if g == Game17 {
		tRAMLow, tRAMHigh = vLow*c.Tex17RAMLow, vHigh*c.Tex17RAMHigh
		dRAMLow, dRAMHigh = dLow*c.Tex17RAMLow, dHigh*c.Tex17RAMHigh
		vLow, vHigh = vLow*c.Tex17VRAMLow, vHigh*c.Tex17VRAMHigh
		dLow, dHigh = dLow*c.Tex17VRAMLow, dHigh*c.Tex17VRAMHigh
	}
	var sounds Files
	var smallPCM, residentBytes int64
	var streamed int
	for _, s := range m.Sounds {
		sounds.add(s.Files)
		smallPCM += s.SmallPCM16Bytes
		residentBytes += s.Resident.Bytes
		streamed += s.Count - s.Small.Count
	}
	sLow := float64(residentBytes) * c.Sound17ResidentLow
	sHigh := float64(residentBytes) * c.Sound17ResidentHigh
	if g == Game16 {
		buffer := float64(streamed) * float64(c.Sound16StreamBuffer)
		sLow = float64(smallPCM)*c.Sound16DecodedLow + buffer
		sHigh = float64(smallPCM)*c.Sound16DecodedHigh + buffer
	}
	r := Report{Parts: []Part{
		part("textures", textures, tRAMLow, tRAMHigh, vLow, vHigh),
		part("dds", m.DDS, dRAMLow, dRAMHigh, dLow, dHigh),
		part("sounds", sounds, sLow, sHigh, 0, 0),
		part("defs", m.Defs, float64(m.Defs.Bytes)*c.DefsLow, float64(m.Defs.Bytes)*c.DefsHigh, 0, 0),
		part("patches", m.Patches, float64(m.Patches.Bytes)*c.PatchesLow, float64(m.Patches.Bytes)*c.PatchesHigh, 0, 0),
		part("assemblies", m.Assemblies, float64(m.Assemblies.Bytes)*c.AssembliesLow, float64(m.Assemblies.Bytes)*c.AssembliesHigh, 0, 0),
	}}
	if g == Game16 {
		// Fixed per non-official Mod: fitted separately using equal-count texture groups.
		r.Parts = append(r.Parts, part("mod overhead", Files{Count: len(mods)}, float64(len(mods))*(1.8*(1<<20)), float64(len(mods))*(2.5*(1<<20)), 0, 0))
	}
	for _, p := range r.Parts {
		r.RAMLow += p.RAMLow
		r.RAMHigh += p.RAMHigh
		r.VRAMLow += p.VRAMLow
		r.VRAMHigh += p.VRAMHigh
	}
	return r
}

func textureRAM16Rate(payload, initial, tail float64) float64 {
	knee := float64(Coef.TextureRAM16Knee)
	if payload <= knee {
		return initial
	}
	return (knee*initial + (payload-knee)*tail) / payload
}

func part(key string, files Files, rLow, rHigh, vLow, vHigh float64) Part {
	return Part{key, files.Count, files.Bytes, int64(math.Floor(rLow)), int64(math.Ceil(rHigh)), int64(math.Floor(vLow)), int64(math.Ceil(vHigh))}
}
