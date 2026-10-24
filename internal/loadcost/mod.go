// Package loadcost estimates mod memory use as a range, excluding the vanilla baseline.
package loadcost

// Files holds file counts and on-disk bytes, except DDS.Bytes which holds payload bytes.
type Files struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

func (f *Files) add(o Files) { f.Count += o.Count; f.Bytes += o.Bytes }

// MipFiles counts PNGs and their block-rounded mip-chain pixels.
type MipFiles struct {
	Count     int   `json:"count"`
	PixelsMip int64 `json:"pixelsMip"`
}

// Sound records one format. Small is the 1.6 file-size subset; Resident is the
// 1.7 decoded-size subset. Both retain on-disk bytes. SmallPCM16Bytes records
// decoded PCM16 bytes for Small, using header data for WAV when available.
type Sound struct {
	Files
	Small           Files `json:"small"`
	SmallPCM16Bytes int64 `json:"smallPcm16Bytes"`
	Resident        Files `json:"resident"`
}

// Mod contains measurements only, so callers can cache it as JSON independently
// of the model. Sounds is keyed by lower-case extension, including the dot.
type Mod struct {
	PNG               Files            `json:"png"`
	PNGPixels         int64            `json:"pngPixels"`
	PNGPixelsMip      int64            `json:"pngPixelsMip"`
	PNGRaw16          MipFiles         `json:"pngRaw16"`
	PNGNot4           int64            `json:"pngNot4"`
	PNGSmall512Pixels int64            `json:"pngSmall512Pixels"`
	DDS               Files            `json:"dds"`
	JPG               Files            `json:"jpg"`
	PSD               Files            `json:"psd"`
	Sounds            map[string]Sound `json:"sounds"`
	Defs              Files            `json:"defs"`
	Patches           Files            `json:"patches"`
	Languages         Files            `json:"languages"`
	Assemblies        Files            `json:"assemblies"`
}

// Plus adds measurements without retaining any maps owned by o.
func (m *Mod) Plus(o Mod) {
	m.PNG.add(o.PNG)
	m.PNGPixels += o.PNGPixels
	m.PNGPixelsMip += o.PNGPixelsMip
	m.PNGRaw16.Count += o.PNGRaw16.Count
	m.PNGRaw16.PixelsMip += o.PNGRaw16.PixelsMip
	m.PNGNot4 += o.PNGNot4
	m.PNGSmall512Pixels += o.PNGSmall512Pixels
	m.DDS.add(o.DDS)
	m.JPG.add(o.JPG)
	m.PSD.add(o.PSD)
	if len(o.Sounds) > 0 && m.Sounds == nil {
		m.Sounds = make(map[string]Sound)
	}
	for ext, s := range o.Sounds {
		t := m.Sounds[ext]
		t.Files.add(s.Files)
		t.Small.add(s.Small)
		t.SmallPCM16Bytes += s.SmallPCM16Bytes
		t.Resident.add(s.Resident)
		m.Sounds[ext] = t
	}
	m.Defs.add(o.Defs)
	m.Patches.add(o.Patches)
	m.Languages.add(o.Languages)
	m.Assemblies.add(o.Assemblies)
}
