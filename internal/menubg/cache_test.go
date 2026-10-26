package menubg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const testDefs = `<Defs>
<ExpansionDef><defName>Royalty</defName><label>Royalty label</label><linkedMod>Ludeon.RimWorld.Royalty</linkedMod><backgroundPath>UI/MenuRoyalty</backgroundPath></ExpansionDef>
<ExpansionDef><defName>Core</defName><label>Core label</label><linkedMod>Ludeon.RimWorld</linkedMod><backgroundPath>UI/MenuCore</backgroundPath></ExpansionDef>
<ExpansionDef><defName>MissingDLC</defName><backgroundPath>UI/Missing</backgroundPath></ExpansionDef>
<ExpansionDef><defName>Ambiguous</defName><backgroundPath>UI/Duplicate</backgroundPath></ExpansionDef>
</Defs>`

func gameFixture(t *testing.T) string {
	t.Helper()
	game := t.TempDir()
	put(t, filepath.Join(game, "Data/Core/Defs/Misc/ExpansionDefs/ExpansionDefs.xml"), []byte(testDefs))
	put(t, filepath.Join(game, "Data/Ambiguous/About/About.xml"), []byte("<ModMetaData/>"))
	put(t, filepath.Join(game, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	return game
}

func fixtureZIP(t *testing.T, game string, method uint16, corrupt bool) {
	t.Helper()
	raw, err := os.ReadFile("testdata/bc7.dds")
	if err != nil {
		t.Fatal(err)
	}
	// Unused mip data must still be consumed and covered by CRC validation.
	raw = append(raw, bytes.Repeat([]byte{1, 2, 3, 4}, 100)...)
	var dst bytes.Buffer
	w := zip.NewWriter(&dst)
	w.RegisterCompressor(93, func(out io.Writer) (io.WriteCloser, error) {
		return zstd.NewWriter(out, zstd.WithEncoderConcurrency(1))
	})
	for _, name := range []string{"UI/MenuCore.png.dds", "UI/MenuRoyalty.jpg.dds", "UI/Duplicate.jpg.dds", "UI/Duplicate.png.dds"} {
		h := &zip.FileHeader{Name: "Resources/Textures/" + name, Method: method}
		h.Extra = []byte{0x34, 0x12, 3, 0, 1, 2, 3} // exercise variable local-header offsets
		if corrupt {
			h.CRC32, h.CompressedSize64, h.UncompressedSize64 = crc32.ChecksumIEEE(raw)^1, uint64(len(raw)), uint64(len(raw))
			h.Method = zip.Store
			f, err := w.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(raw)
		} else {
			f, err := w.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(raw)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(game, "Assets.zip"), dst.Bytes())
}

func TestPrepareCacheAndVersions(t *testing.T) {
	for _, method := range []uint16{zip.Deflate, 93} {
		t.Run(map[uint16]string{zip.Deflate: "deflate", 93: "zstd"}[method], func(t *testing.T) {
			game, cache := gameFixture(t), t.TempDir()
			fixtureZIP(t, game, method, false)
			items, err := Prepare(context.Background(), game, cache, "en")
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 2 || items[0].Key != "game:ludeon.rimworld" || items[1].Key != "game:ludeon.rimworld.royalty" || items[1].Label != "Royalty label" {
				t.Fatalf("incorrect catalog: %+v", items)
			}
			for _, it := range items {
				for _, path := range []string{it.Full, it.Thumb} {
					f, err := os.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					cfg, err := jpeg.DecodeConfig(f)
					f.Close()
					if err != nil || cfg.Width != 32 || cfg.Height != 4 {
						t.Fatalf("invalid cached image: %+v %v", cfg, err)
					}
				}
			}
			// A cache hit must not decode or rewrite, even after a language switch.
			old := time.Unix(100, 0)
			os.Chtimes(items[0].Full, old, old)
			again, err := Prepare(context.Background(), game, cache, "zh")
			if err != nil || len(again) != 2 || again[0].Full != items[0].Full || again[0].Label != "Core" {
				t.Fatalf("cache/language: %+v %v", again, err)
			}
			st, _ := os.Stat(items[0].Full)
			if !st.ModTime().Equal(old) {
				t.Fatal("cache hit rewrote the image")
			}
			// A missing member repairs the pair; source changes and other installs reuse it.
			os.Remove(items[0].Thumb)
			if _, err := Prepare(context.Background(), game, cache, "en"); err != nil || !validPair(items[0]) {
				t.Fatal("did not repair incomplete pair", err)
			}
			os.Chtimes(filepath.Join(game, "Assets.zip"), old, old)
			changed, err := Prepare(context.Background(), game, cache, "en")
			if err != nil || changed[0].Full != items[0].Full {
				t.Fatal("archive change replaced shared cache", err)
			}
			other := gameFixture(t)
			fixtureZIP(t, other, method, false)
			different, err := Prepare(context.Background(), other, cache, "en")
			if err != nil || different[0].Full != items[0].Full {
				t.Fatal("another game did not reuse shared cache", err)
			}
		})
	}
}

func TestCRCAndCancellation(t *testing.T) {
	game, cache := gameFixture(t), t.TempDir()
	fixtureZIP(t, game, zip.Store, true)
	items, err := Prepare(context.Background(), game, cache, "en")
	if err == nil || !strings.Contains(err.Error(), "checksum") || len(items) != 0 {
		t.Fatalf("CRC: %+v %v", items, err)
	}
	files, _ := os.ReadDir(cache)
	if len(files) != 0 {
		t.Fatal("published JPEG from corrupt DDS")
	}
	fixtureZIP(t, game, 93, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, game, cache, "en"); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestChineseLabels(t *testing.T) {
	game := gameFixture(t)
	fixtureZIP(t, game, 93, false)
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	text := []byte(`<LanguageData><Core.label>核心</Core.label><Royalty.label>皇权</Royalty.label></LanguageData>`)
	w.WriteHeader(&tar.Header{Name: "DefInjected/ExpansionDef/ExpansionDefs.xml", Mode: 0o644, Size: int64(len(text))})
	w.Write(text)
	w.Close()
	path := filepath.Join(game, "Data/Core/Languages/ChineseSimplified (简体中文).tar")
	put(t, path, data.Bytes())
	items, err := Prepare(context.Background(), game, t.TempDir(), "zh")
	if err != nil || len(items) != 2 || items[0].Label != "核心" || items[1].Label != "皇权" {
		t.Fatalf("tar labels: %+v %v", items, err)
	}
	os.Remove(path)
	put(t, strings.TrimSuffix(path, ".tar")+"/DefInjected/ExpansionDef/ExpansionDefs.xml", text)
	items, err = Prepare(context.Background(), game, t.TempDir(), "zh")
	if err != nil || items[1].Label != "皇权" {
		t.Fatalf("directory labels: %+v %v", items, err)
	}
}

func TestOddDDSRows(t *testing.T) {
	raw, _ := os.ReadFile("testdata/bc7.dds")
	// The block layout is still 8 blocks per row; the padded last texels are ignored.
	binary.LittleEndian.PutUint32(raw[12:], 3)
	binary.LittleEndian.PutUint32(raw[16:], 31)
	d, err := ReadDDS(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	images, err := d.Scaled(context.Background(), 17, 5)
	if err != nil || len(images) != 2 || images[0].Rect.Dx() != 17 || images[0].Rect.Dy() != 1 {
		t.Fatal("odd image edges", err)
	}
}
