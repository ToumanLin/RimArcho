package loadcost

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeBytes(t *testing.T, path string, b []byte) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return int64(len(b))
}

func writePNG(t *testing.T, path string, w, h int) int64 {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return writeBytes(t, path, b.Bytes())
}

func dds(dx10 bool, payload int) []byte {
	header := 128
	if dx10 {
		header = 148
	}
	b := make([]byte, header+payload)
	copy(b, "DDS ")
	if dx10 {
		copy(b[84:88], "DX10")
	}
	return b
}

func wav(bits uint16, data int, junk bool) []byte {
	var chunks bytes.Buffer
	if junk {
		chunks.WriteString("JUNK")
		binary.Write(&chunks, binary.LittleEndian, uint32(1))
		chunks.Write([]byte{0, 0})
	}
	chunks.WriteString("fmt ")
	binary.Write(&chunks, binary.LittleEndian, uint32(16))
	for _, v := range []any{uint16(1), uint16(2), uint32(44100), uint32(44100 * 2 * uint32(bits/8)), uint16(2 * (bits / 8)), bits} {
		binary.Write(&chunks, binary.LittleEndian, v)
	}
	chunks.WriteString("data")
	binary.Write(&chunks, binary.LittleEndian, uint32(data))
	chunks.Write(make([]byte, data))
	if data%2 != 0 {
		chunks.WriteByte(0)
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(chunks.Len()+4))
	b.WriteString("WAVE")
	b.Write(chunks.Bytes())
	return b.Bytes()
}

func TestMeasure(t *testing.T) {
	root := t.TempDir()
	high := filepath.Join(root, "1.6")
	put := func(base, rel string, b []byte) int64 {
		return writeBytes(t, filepath.Join(base, filepath.FromSlash(rel)), b)
	}
	pngBytes := writePNG(t, filepath.Join(high, "Textures", "A.PNG"), 64, 64)
	writePNG(t, filepath.Join(root, "Textures", "a.png"), 600, 600) // Shadowed by higher priority, case folded.
	pngBytes += writePNG(t, filepath.Join(root, "Textures", "tiny.png"), 6, 6)
	pngBytes += writePNG(t, filepath.Join(root, "Textures", "large.png"), 600, 600)
	writePNG(t, filepath.Join(high, "Textures", "shadow.png"), 16, 16)
	put(root, "Textures/shadow.dds", dds(true, 1000)) // Lower DDS shadows higher PNG.
	put(high, "Textures/legacy.dds", dds(false, 256))
	put(root, "Textures/legacy.dds", dds(true, 9999))
	put(root, "Textures/truncated.dds", []byte("DDS ")) // Truncated payload clamps to zero.
	put(root, "Textures/photo.jpg", make([]byte, 100))
	put(root, "Textures/photo.jpeg", make([]byte, 50))
	put(root, "Textures/art.psd", make([]byte, 200))
	writePNG(t, filepath.Join(root, "Textures", "Source", "work.png"), 64, 64)
	writePNG(t, filepath.Join(root, "Textures", ".git", "work.png"), 64, 64)
	put(root, "Textures/invalid.png", []byte("invalid"))

	voice := put(high, "Sounds/Voice/clip.WAV", wav(24, 600, true))
	put(root, "Sounds/voice/clip.wav", make([]byte, 10000))
	stream := put(root, "Sounds/stream.wav", wav(16, 400<<10, false))
	put(root, "Sounds/sample.OGG", make([]byte, 1000))
	put(root, "Sounds/not-resident.ogg", make([]byte, 200000))
	put(root, "Sounds/stream.ogg", make([]byte, 400000))
	put(root, "Sounds/music.MP3", make([]byte, 100))
	for _, ext := range []string{"aif", "aiff", "aifc"} {
		put(root, "Sounds/sample."+ext, make([]byte, 50))
	}
	put(root, "Sounds/ignored.txt", make([]byte, 999))
	put(root, "Sounds/.svn/skip.wav", wav(16, 100, false))
	put(high, "Defs/A.xml", []byte("<Defs/>"))
	put(root, "Defs/A.xml", []byte("<Defs/>")) // XML counts include both load layers.
	put(root, "Defs/Nested/B.XML", []byte("<Defs/>"))
	put(root, "Defs/ignored.txt", []byte("ignored"))
	put(root, "Patches/Nested/p.xml", []byte("<Patch/>"))
	put(root, "Languages/English/Keyed/a.xml", []byte("123"))
	put(root, "Languages/Chinese/info.txt", []byte("12345"))
	put(high, "Assemblies/Nested/A.DLL", make([]byte, 100))
	put(root, "Assemblies/nested/a.dll", make([]byte, 999))
	put(root, "Assemblies/B.dll", make([]byte, 200))
	put(root, "Assemblies/.rimarcho/skip.dll", make([]byte, 999))
	put(root, "1.5/Defs/inactive.xml", make([]byte, 999))
	put(root, ".git/Defs/skip.xml", make([]byte, 999))

	cache := filepath.Join(t.TempDir(), "png.json")
	folders := []string{high, root}
	got, err := Measure(context.Background(), root, folders, cache)
	if err != nil {
		t.Fatal(err)
	}
	want := Mod{
		MaxPNGDecode: mipPixels(600, 600) * 4,
		PNG:          Files{3, pngBytes}, PNGPixels: 364132, PNGPixelsMip: 486656,
		PNGRaw16: MipFiles{1, 96}, PNGNot4: 96, PNGSmall512Pixels: 4132,
		DDS: Files{3, 1256}, JPG: Files{2, 150}, PSD: Files{1, 200},
		Sounds: map[string]Sound{
			".wav":  {Files{2, voice + stream}, Files{1, voice}, 400, Files{2, voice + stream}},
			".ogg":  {Files{3, 601000}, Files{2, 201000}, 2211000, Files{1, 1000}},
			".mp3":  {Files{1, 100}, Files{1, 100}, 1100, Files{1, 100}},
			".aif":  {Files{1, 50}, Files{1, 50}, 50, Files{1, 50}},
			".aiff": {Files{1, 50}, Files{1, 50}, 50, Files{1, 50}},
			".aifc": {Files{1, 50}, Files{1, 50}, 50, Files{1, 50}},
		},
		Defs: Files{3, 21}, Patches: Files{1, 8}, Languages: Files{2, 8}, Assemblies: Files{2, 300},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Measure:\n got %+v\nwant %+v", got, want)
	}
	for _, tc := range []struct {
		game                               Game
		ramLow, ramHigh, vramLow, vramHigh int64
	}{
		{Game16, 5015931, 8075774, 490066, 497932},
		{Game17, 1147315, 2347306, 441059, 1283881},
	} {
		r := Estimate(tc.game, []Mod{got})
		if r.RAMLow != tc.ramLow || r.RAMHigh != tc.ramHigh || r.VRAMLow != tc.vramLow || r.VRAMHigh != tc.vramHigh {
			t.Fatalf("measured mod, game %d: %+v", tc.game, r)
		}
	}
	again, err := Measure(context.Background(), root, folders, cache)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("cached Measure differs: %+v, %v", again, err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Mod
	if err := json.Unmarshal(data, &decoded); err != nil || !reflect.DeepEqual(decoded, got) {
		t.Fatalf("JSON round trip: %s, %v", data, err)
	}
	// A changed header invalidates the PNG cache and its measured pixel totals.
	writePNG(t, filepath.Join(root, "Textures", "tiny.png"), 16, 16)
	changed, err := Measure(context.Background(), root, folders, cache)
	if err != nil || changed.PNGPixels != got.PNGPixels-36+256 {
		t.Fatalf("changed PNG: %+v, %v", changed, err)
	}
}

func TestWAVAndDDSHeaders(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		data []byte
		want int64
	}{
		{"pcm8", wav(8, 101, true), 202},
		{"pcm16", wav(16, 600, false), 600},
		{"pcm24", wav(24, 600, true), 400},
		{"pcm32", wav(32, 600, false), 300},
		{"empty", wav(16, 0, false), 0},
		{"invalid", []byte("bad WAV"), 7},
		{"truncated", wav(16, 100, false)[:44], 44},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name)
			size := writeBytes(t, path, tc.data)
			if got := wavPCM16(context.Background(), path, size); got != tc.want {
				t.Fatalf("PCM16 = %d, want %d", got, tc.want)
			}
		})
	}
	if got := ddsPayload(filepath.Join(root, "missing"), 200); got != 72 {
		t.Fatalf("DDS read fallback = %d", got)
	}
	shortDX10 := dds(true, 0)[:140]
	path := filepath.Join(root, "short.dds")
	writeBytes(t, path, shortDX10)
	if got := ddsPayload(path, 140); got != 12 {
		t.Fatalf("short DX10 payload = %d", got)
	}
}

func TestJPEGShadowing(t *testing.T) {
	root := t.TempDir()
	writeBytes(t, filepath.Join(root, "Textures", "a.jpg"), make([]byte, 10))
	writeBytes(t, filepath.Join(root, "Textures", "a.jpeg"), make([]byte, 20))
	writeBytes(t, filepath.Join(root, "Textures", "a.dds"), dds(false, 100))
	writeBytes(t, filepath.Join(root, "Textures", "b.jpeg"), make([]byte, 30))
	writeBytes(t, filepath.Join(root, "Textures", "b..dds"), dds(false, 200))
	m, err := Measure(context.Background(), root, []string{root}, filepath.Join(t.TempDir(), "png.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.JPG != (Files{1, 20}) || m.DDS != (Files{2, 300}) {
		t.Fatalf("JPEG shadowing: %+v", m)
	}
}

func TestSoundThresholds(t *testing.T) {
	root := t.TempDir()
	writeBytes(t, filepath.Join(root, "Sounds", "at300.wav"), wav(16, (300<<10)-44, false))
	writeBytes(t, filepath.Join(root, "Sounds", "over300.wav"), wav(16, (300<<10)-42, false))
	writeBytes(t, filepath.Join(root, "Sounds", "at2m.wav"), wav(16, 2<<20, false))
	writeBytes(t, filepath.Join(root, "Sounds", "over2m.wav"), wav(16, (2<<20)+2, false))
	m, err := Measure(context.Background(), root, []string{root}, filepath.Join(t.TempDir(), "png.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := m.Sounds[".wav"]
	if s.Count != 4 || s.Small != (Files{1, 300 << 10}) || s.Resident.Count != 3 || s.SmallPCM16Bytes != (300<<10)-44 {
		t.Fatalf("thresholds: %+v", s)
	}
}

func TestPNGSizeRules(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "Textures", "raw.png"), 16, 16)
	writePNG(t, filepath.Join(root, "Textures", "tall.png"), 16, 17)
	writePNG(t, filepath.Join(root, "Textures", "boundary.png"), 512, 16)
	m, err := Measure(context.Background(), root, []string{root}, filepath.Join(t.TempDir(), "png.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.PNGRaw16 != (MipFiles{1, 368}) || m.PNGSmall512Pixels != 528 || m.PNGNot4 != 432 {
		t.Fatalf("PNG size rules: %+v", m)
	}
}

func TestMeasureCancellationAndMissingFolder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m, err := Measure(ctx, t.TempDir(), nil, ""); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(m, Mod{}) {
		t.Fatalf("cancellation: %+v, %v", m, err)
	}
	root := t.TempDir()
	if m, err := Measure(context.Background(), root, []string{filepath.Join(root, "missing")}, filepath.Join(t.TempDir(), "png.json")); err != nil || !reflect.DeepEqual(m, Mod{}) {
		t.Fatalf("missing folder: %+v, %v", m, err)
	}
	writePNG(t, filepath.Join(root, "Textures", "a.png"), 64, 64)
	writeBytes(t, filepath.Join(root, "Defs", "a.xml"), []byte("<Defs/>"))
	cache := filepath.Join(t.TempDir(), "png.json")
	want, err := Measure(context.Background(), root, []string{root}, cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Measure(context.Background(), root, []string{filepath.Join(root, "missing-high"), root, filepath.Join(root, "missing-low")}, cache)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("missing folders must preserve existing assets: got %+v, want %+v, error %v", got, want, err)
	}
}
