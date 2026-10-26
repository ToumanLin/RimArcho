package menubg

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/klauspost/compress/zstd"
)

func goldenBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func splitHeader(w, h, mips int) []byte {
	h0 := make([]byte, 148)
	copy(h0, "DDS ")
	for offset, value := range map[int]int{4: 124, 8: 0xa1007, 12: h, 16: w, 28: mips, 76: 32, 80: 4, 108: 0x401008, 128: 98, 132: 3, 140: 1} {
		binary.LittleEndian.PutUint32(h0[offset:], uint32(value))
	}
	copy(h0[84:], "DX10")
	return h0
}
func splitFixture(t *testing.T, w, h, mips int, commands []byte) []byte {
	t.Helper()
	out := append(splitHeader(w, h, mips), commands...)
	// The wire order is part of the fixture, independent of the decoder tables.
	order := []byte{9, 6, 10, 5, 1, 3, 4, 0, 2, 7, 8}
	for _, stream := range []bool{false, true} {
		for _, kind := range order {
			for _, c := range commands {
				if c == kind {
					record := splitGoldens[c].endpoints
					if stream {
						record = splitGoldens[c].indices
					}
					out = append(out, goldenBytes(t, record)...)
				}
			}
		}
	}
	return out
}
func mixedSplitFixture(t *testing.T) []byte {
	return splitFixture(t, 44, 4, 2, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 9, 10, 5, 6, 0, 7})
}
func splitReference(t *testing.T) *image.NRGBA {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 44, 4))
	for x, g := range splitGoldens {
		rgba := goldenBytes(t, g.rgba)
		for y := 0; y < 4; y++ {
			copy(img.Pix[y*img.Stride+x*16:], rgba[y*16:y*16+16])
		}
	}
	return img
}
func TestSplitBC7GoldenPixelsAndMipChain(t *testing.T) {
	raw := mixedSplitFixture(t)
	for _, dxgi := range []uint32{98, 99} {
		t.Run(strconv.Itoa(int(dxgi)), func(t *testing.T) {
			binary.LittleEndian.PutUint32(raw[128:], dxgi)
			d, err := readSplitDDS(context.Background(), bytes.NewReader(raw), uint64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			compareRows(t, io.MultiReader(bytes.NewReader(raw[:148]), d.r), splitReference(t))
			tail, err := io.ReadAll(d.r)
			if err != nil {
				t.Fatal(err)
			}
			var want []byte
			for _, c := range []byte{9, 10, 5, 6, 0, 7} {
				want = append(want, goldenBytes(t, splitGoldens[c].block)...)
			}
			if !bytes.Equal(tail, want) {
				t.Fatal("unused mip blocks differ from independent golden bytes")
			}
		})
	}
	for _, g := range splitGoldens {
		block, err := restoreSplitBlock(g.command, goldenBytes(t, g.endpoints), goldenBytes(t, g.indices))
		if err != nil || !bytes.Equal(block[:], goldenBytes(t, g.block)) {
			t.Fatalf("command %d: block=%x err=%v", g.command, block, err)
		}
	}
}

// Odd dimensions exercise partial edge blocks, clamped 1x1 mip dimensions,
// and byte-sized reads across header, block and record boundaries.
func TestSplitBC7OddDimensionsAndShortReads(t *testing.T) {
	commands := []byte{6, 9, 5, 10, 7, 8} // 7x5, 3x2, 1x1.
	raw := splitFixture(t, 7, 5, 3, commands)
	d, err := readSplitDDS(context.Background(), iotest.OneByteReader(bytes.NewReader(raw)), uint64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	ref := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			pixels := goldenBytes(t, splitGoldens[commands[(y/4)*2+x/4]].rgba)
			src := ((y%4)*4 + x%4) * 4
			copy(ref.Pix[y*ref.Stride+x*4:][:4], pixels[src:src+4])
		}
	}
	compareRows(t, io.MultiReader(bytes.NewReader(raw[:148]), iotest.OneByteReader(d.r)), ref)
	tail, err := io.ReadAll(iotest.OneByteReader(d.r))
	want := append(goldenBytes(t, splitGoldens[7].block), goldenBytes(t, splitGoldens[8].block)...)
	if err != nil || !bytes.Equal(tail, want) {
		t.Fatalf("lower mip blocks: got %x want %x err=%v", tail, want, err)
	}
}

func TestSplitBC7RejectsMalformed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"magic", func(b []byte) []byte { b[0] = 0; return b }},
		{"oversize-image", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[16:], 16385); return b }},
		{"pixel-limit", func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[16:], 8001)
			binary.LittleEndian.PutUint32(b[12:], 5000)
			return b
		}},
		{"zero-mips", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[28:], 0); return b }},
		{"too-many-mips", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[28:], 32); return b }},
		{"unsupported-bc1", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[128:], 71); return b }},
		{"array", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[140:], 2); return b }},
		{"command-in-unused-mip", func(b []byte) []byte { b[148+16] = 11; return b }},
		{"record-layout", func(b []byte) []byte { b[148+9] = 0; return b }},
		{"truncated", func(b []byte) []byte { return b[:len(b)-1] }},
		{"trailing-byte", func(b []byte) []byte { return append(b, 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mutate(mixedSplitFixture(t))
			if _, err := readSplitDDS(context.Background(), bytes.NewReader(b), uint64(len(b))); err == nil {
				t.Fatal("accepted malformed split DDS")
			}
		})
	}
	for _, size := range []uint64{0, 147, maxMenuTexture + 1} {
		if _, err := readSplitDDS(context.Background(), failedReader{errors.New("must not read")}, size); err == nil || strings.Contains(err.Error(), "must not read") {
			t.Fatalf("size %d: %v", size, err)
		}
	}
	b := mixedSplitFixture(t)
	if _, err := readSplitDDS(context.Background(), bytes.NewReader(b[:len(b)-1]), uint64(len(b))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short entry: %v", err)
	}
	if _, err := readSplitDDS(context.Background(), bytes.NewReader(append(b, 0)), uint64(len(b))); err == nil {
		t.Fatal("accepted data beyond declared size")
	}
}
func TestSplitBC7RejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		command            byte
		epOffset, ixOffset int
	}{{5, 0, -1}, {5, 1, -1}, {6, 6, -1}, {6, 8, -1}, {9, 0, -1}, {10, 0, -1}, {5, -1, 0}, {10, -1, 0}} {
		ep := goldenBytes(t, splitGoldens[tc.command].endpoints)
		ix := goldenBytes(t, splitGoldens[tc.command].indices)
		if tc.epOffset >= 0 {
			ep[tc.epOffset] = 255
		}
		if tc.ixOffset >= 0 {
			ix[tc.ixOffset] |= 2
		}
		if _, err := restoreSplitBlock(tc.command, ep, ix); err == nil {
			t.Fatalf("command %d accepted invalid fields", tc.command)
		}
	}
}

type finalErrorReader struct {
	data []byte
	err  error
}

func (r *finalErrorReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

type cancelReader struct {
	r      io.Reader
	cancel context.CancelFunc
	after  int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.after -= n
	if r.after <= 0 {
		r.cancel()
	}
	return n, err
}
func TestSplitBC7CancellationAndTerminalChecksum(t *testing.T) {
	b := mixedSplitFixture(t)
	if _, err := readSplitDDS(context.Background(), &finalErrorReader{data: b, err: zip.ErrChecksum}, uint64(len(b))); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("terminal checksum lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readSplitDDS(ctx, failedReader{errors.New("must not read")}, uint64(len(b))); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if _, err := readSplitDDS(ctx, &cancelReader{r: bytes.NewReader(b), cancel: cancel, after: 149}, uint64(len(b))); !errors.Is(err, context.Canceled) {
		t.Fatalf("during payload: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	d, err := readSplitDDS(ctx, bytes.NewReader(b), uint64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := d.Scaled(ctx, 32); !errors.Is(err, context.Canceled) {
		t.Fatalf("during decode: %v", err)
	}
	// A malformed field in a lower mip must fail even though only level 0 is drawn.
	b = splitFixture(t, 4, 4, 2, []byte{0, 5})
	b[148+2] = 255 // command 5's rotation is the first endpoint record.
	d, err = readSplitDDS(context.Background(), bytes.NewReader(b), uint64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Scaled(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(io.Discard, d.r); err == nil {
		t.Fatal("did not validate the unused mip")
	}
}

func writeSplitZIP(t *testing.T, game string, method uint16, entries map[string][]byte, corrupt bool) {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	z.RegisterCompressor(93, func(w io.Writer) (io.WriteCloser, error) { return zstd.NewWriter(w, zstd.WithEncoderConcurrency(1)) })
	for name, data := range entries {
		h := &zip.FileHeader{Name: name, Method: method}
		var w io.Writer
		var err error
		if corrupt {
			h.Method = zip.Store
			h.CRC32 = crc32.ChecksumIEEE(data) ^ 1
			h.UncompressedSize64 = uint64(len(data))
			h.CompressedSize64 = uint64(len(data))
			w, err = z.CreateRaw(h)
		} else {
			w, err = z.CreateHeader(h)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(game, "Assets.zip"), buf.Bytes())
}
func singleSplitGame(t *testing.T) string {
	t.Helper()
	game := t.TempDir()
	put(t, filepath.Join(game, "Data/Core/Defs/Misc/ExpansionDefs/ExpansionDefs.xml"), []byte(`<Defs><ExpansionDef><defName>Core</defName><backgroundPath>UI/MenuCore</backgroundPath></ExpansionDef><ExpansionDef><defName>Royalty</defName><linkedMod>Ludeon.RimWorld.Royalty</linkedMod><backgroundPath>UI/MenuRoyalty</backgroundPath></ExpansionDef></Defs>`))
	return game
}

const coreSplitName = "Resources/Textures/UI/MenuCore.png.dds.split"

func TestPrepareSplitCachePixelsAndFailures(t *testing.T) {
	for _, method := range []uint16{zip.Store, zip.Deflate, 93} {
		t.Run(map[uint16]string{zip.Store: "stored", zip.Deflate: "deflate", 93: "zstd"}[method], func(t *testing.T) {
			game, cache := singleSplitGame(t), t.TempDir()
			data := mixedSplitFixture(t)
			writeSplitZIP(t, game, method, map[string][]byte{coreSplitName: data}, false)
			items, err := Prepare(context.Background(), game, cache, "en")
			if err != nil || len(items) != 1 {
				t.Fatalf("items=%v err=%v", items, err)
			}
			if items[0].Key != "game:ludeon.rimworld" {
				t.Fatal(items[0].Key)
			}
			var expected bytes.Buffer
			ref := splitReference(t)
			opaque := image.NewRGBA(ref.Rect)
			for y := 0; y < 4; y++ {
				for x := 0; x < 44; x++ {
					c := ref.NRGBAAt(x, y)
					off := y*opaque.Stride + x*4
					opaque.Pix[off] = byte(uint32(c.R) * uint32(c.A) / 255)
					opaque.Pix[off+1] = byte(uint32(c.G) * uint32(c.A) / 255)
					opaque.Pix[off+2] = byte(uint32(c.B) * uint32(c.A) / 255)
					opaque.Pix[off+3] = 255
				}
			}
			if err = jpeg.Encode(&expected, opaque, &jpeg.Options{Quality: 86}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{items[0].Full, items[0].Thumb} {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, expected.Bytes()) {
					t.Fatalf("JPEG does not match independent golden pixels: %s %v", path, err)
				}
			}
			old := time.Unix(100, 0)
			if err = os.Chtimes(items[0].Full, old, old); err != nil {
				t.Fatal(err)
			}
			again, err := Prepare(context.Background(), game, cache, "zh")
			if err != nil || len(again) != 1 || again[0].Full != items[0].Full {
				t.Fatal("cache reuse", err)
			}
			info, err := os.Stat(items[0].Full)
			if err != nil || !info.ModTime().Equal(old) {
				t.Fatal("rewrote cache", err)
			}
			if err = os.Remove(items[0].Thumb); err != nil {
				t.Fatal(err)
			}
			if _, err = Prepare(context.Background(), game, cache, "en"); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(items[0].Thumb); err != nil {
				t.Fatal("pair repair", err)
			}
		})
	}
	for _, kind := range []string{"crc", "unsupported", "unused-mip"} {
		t.Run(kind, func(t *testing.T) {
			game, cache := singleSplitGame(t), t.TempDir()
			data := mixedSplitFixture(t)
			if kind == "unsupported" {
				binary.LittleEndian.PutUint32(data[128:], 71)
			}
			if kind == "unused-mip" {
				data = splitFixture(t, 4, 4, 2, []byte{0, 5})
				data[150] = 255
			}
			writeSplitZIP(t, game, 93, map[string][]byte{coreSplitName: data}, kind == "crc")
			items, err := Prepare(context.Background(), game, cache, "en")
			if len(items) != 0 || err == nil {
				t.Fatalf("published invalid image: %v %v", items, err)
			}
			if !strings.Contains(err.Error(), coreSplitName) || !strings.Contains(err.Error(), "Preview.png") {
				t.Fatal("diagnostic missing candidate or fallback", err)
			}
			files, e := os.ReadDir(cache)
			if e != nil || len(files) != 0 {
				t.Fatal("published partial output", files, e)
			}
		})
	}
	game, cache := singleSplitGame(t), t.TempDir()
	put(t, filepath.Join(game, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	bad := mixedSplitFixture(t)
	binary.LittleEndian.PutUint32(bad[128:], 71)
	writeSplitZIP(t, game, 93, map[string][]byte{coreSplitName: mixedSplitFixture(t), "Resources/Textures/UI/MenuRoyalty.jpg.dds.split": bad}, false)
	items, err := Prepare(context.Background(), game, cache, "en")
	if len(items) != 1 || err == nil || items[0].Key != "game:ludeon.rimworld" {
		t.Fatal("per-package fallback", items, err)
	}
}
func TestSplitCatalogAmbiguityAndUnsupportedDiagnostic(t *testing.T) {
	game := singleSplitGame(t)
	raw := append(splitHeader(4, 4, 1), goldenBytes(t, splitGoldens[0].block)...)
	for _, names := range [][]string{{coreSplitName, "Resources/Textures/UI/MenuCore.jpg.dds"}, {coreSplitName, "Resources/Textures/UI/MenuCore.jpg.dds.split"}, {"Resources/Textures/UI/MenuCore.png.unknown"}} {
		entries := map[string][]byte{}
		for _, name := range names {
			entries[name] = raw
		}
		writeSplitZIP(t, game, zip.Store, entries, false)
		var logs bytes.Buffer
		previous := log.Writer()
		log.SetOutput(&logs)
		items, err := Prepare(context.Background(), game, t.TempDir(), "en")
		log.SetOutput(previous)
		if err != nil || len(items) != 0 {
			t.Fatal("ambiguous/unsupported candidate selected", items, err)
		}
		for _, text := range []string{"MenuCore", "Preview.png", game, names[0]} {
			if !strings.Contains(logs.String(), text) {
				t.Fatalf("missing actionable diagnostic %q in %s", text, logs.String())
			}
		}
	}
	writeSplitZIP(t, game, zip.Store, map[string][]byte{strings.ToUpper(coreSplitName): mixedSplitFixture(t)}, false)
	if items, err := Prepare(context.Background(), game, t.TempDir(), "en"); err != nil || len(items) != 1 {
		t.Fatal("case insensitive split entry", items, err)
	}
}
