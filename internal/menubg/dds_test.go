package menubg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func compareRows(t *testing.T, r io.Reader, reference image.Image) {
	t.Helper()
	d, err := ReadDDS(r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Width != reference.Bounds().Dx() || d.Height != reference.Bounds().Dy() {
		t.Fatal("reference dimensions differ")
	}
	err = d.Rows(context.Background(), func(y int, rows *image.NRGBA) error {
		for yy := 0; yy < rows.Rect.Dy(); yy++ {
			var expected []byte
			switch p := reference.(type) {
			case *image.NRGBA:
				expected = p.Pix[(y+yy)*p.Stride:][:d.Width*4]
			case *image.RGBA:
				expected = p.Pix[(y+yy)*p.Stride:][:d.Width*4]
			default:
				t.Fatalf("unexpected PNG type %T", p)
			}
			got := rows.Pix[yy*rows.Stride:][:d.Width*4]
			if !bytes.Equal(got, expected) {
				for x := 0; x < d.Width; x++ {
					want := color.NRGBAModel.Convert(reference.At(x, y+yy)).(color.NRGBA)
					if actual := rows.NRGBAAt(x, yy); actual != want {
						t.Fatalf("pixel (%d,%d): got %v want %v", x, y+yy, actual, want)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pngReference(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Required, tiny independent reference: original BC7 blocks, including all modes.
func TestBC7Fixture(t *testing.T) {
	f, err := os.Open("testdata/bc7.dds")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	compareRows(t, f, pngReference(t, "testdata/bc7.png"))
}

// The player's independent Pillow exports are the pixel oracle, not another path
// through the Go decoder. The test is skipped only when the reference is absent.
func TestMenuReferencePixels(t *testing.T) {
	root := os.Getenv("MENU_BG_REFERENCE")
	if root == "" {
		root = `D:\Game\RimModManager_Upgrade\extracted-menu-backgrounds`
	}
	b, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if os.IsNotExist(err) {
		t.Skip("menu reference directory not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Backgrounds []struct {
			Name, PNG, DDS string
			Hash           string `json:"dds_sha256"`
		}
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Backgrounds) != 6 {
		t.Fatalf("expected six references, got %d", len(manifest.Backgrounds))
	}
	for _, ref := range manifest.Backgrounds {
		t.Run(ref.Name, func(t *testing.T) {
			defer debug.FreeOSMemory()
			f, err := os.Open(filepath.Join(root, filepath.FromSlash(ref.DDS)))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			hash := sha256.New()
			r := io.TeeReader(f, hash)
			compareRows(t, r, pngReference(t, filepath.Join(root, ref.PNG)))
			if _, err := io.Copy(io.Discard, r); err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(hash.Sum(nil)) != ref.Hash {
				t.Fatal("DDS does not match manifest SHA-256")
			}
		})
	}
}

func TestStreamingScale(t *testing.T) {
	raw, err := os.ReadFile("testdata/bc7.dds")
	if err != nil {
		t.Fatal(err)
	}
	ref := pngReference(t, "testdata/bc7.png")
	for _, w := range []int{1, 7, 9, 1920} {
		d, err := ReadDDS(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		out, err := d.Scaled(context.Background(), w)
		if err != nil {
			t.Fatal(err)
		}
		img := out[0]
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				var sums [3]uint32
				n := uint32(0)
				for sy := y * d.Height / img.Rect.Dy(); sy < (y+1)*d.Height/img.Rect.Dy(); sy++ {
					for sx := x * d.Width / img.Rect.Dx(); sx < (x+1)*d.Width/img.Rect.Dx(); sx++ {
						c := color.NRGBAModel.Convert(ref.At(sx, sy)).(color.NRGBA)
						for i, v := range []byte{c.R, c.G, c.B} {
							sums[i] += uint32(v) * uint32(c.A) / 255
						}
						n++
					}
				}
				want := color.RGBA{byte(sums[0] / n), byte(sums[1] / n), byte(sums[2] / n), 255}
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("width %d pixel %d,%d: %v != %v", w, x, y, got, want)
				}
			}
		}
	}
}

func TestBC1BC3AndInvalidDDS(t *testing.T) {
	raw, _ := os.ReadFile("testdata/bc7.dds")
	for _, format := range []string{"DXT1", "DXT5"} {
		h := bytes.Clone(raw[:128])
		binary.LittleEndian.PutUint32(h[12:], 4)
		binary.LittleEndian.PutUint32(h[16:], 4)
		copy(h[84:], format)
		block := []byte{0x00, 0xf8, 0xe0, 0x07, 0, 0, 0, 0} // RGB565 red and green, all red indices
		if format == "DXT5" {
			block = append([]byte{255, 0, 0, 0, 0, 0, 0, 0}, block...)
		}
		d, err := ReadDDS(bytes.NewReader(append(h, block...)))
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Rows(context.Background(), func(_ int, rows *image.NRGBA) error {
			for y := 0; y < 4; y++ {
				for x := 0; x < 4; x++ {
					if rows.NRGBAAt(x, y) != (color.NRGBA{255, 0, 0, 255}) {
						t.Fatal("incorrect red BC block")
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 0 },
		func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 0xffffffff) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[140:], 2) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[128:], 2) },
	} {
		bad := bytes.Clone(raw)
		mutate(bad)
		if _, err := ReadDDS(bytes.NewReader(bad)); err == nil {
			t.Fatal("accepted invalid DDS")
		}
	}
	d, _ := ReadDDS(bytes.NewReader(raw[:148]))
	if err := d.Rows(context.Background(), func(int, *image.NRGBA) error { return nil }); err == nil {
		t.Fatal("accepted truncated DDS")
	}
}
