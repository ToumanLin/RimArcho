package looks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"testing"
)

// ddsFile builds a DDS file: a FourCC, or with fourCC "" an uncompressed layout of bits
// per pixel with the given R, G, B, A masks.
func ddsFile(w, h int, fourCC string, bits uint32, masks [4]uint32, data []byte) []byte {
	b := make([]byte, 128)
	copy(b, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(b[4:], 124)
	le.PutUint32(b[12:], uint32(h))
	le.PutUint32(b[16:], uint32(w))
	le.PutUint32(b[76:], 32)
	if fourCC != "" {
		le.PutUint32(b[80:], 0x4)
		copy(b[84:], fourCC)
	} else {
		flags := uint32(0x40)
		if masks[3] != 0 {
			flags |= 0x1
		}
		le.PutUint32(b[80:], flags)
		le.PutUint32(b[88:], bits)
		for i, m := range masks {
			le.PutUint32(b[92+4*i:], m)
		}
	}
	return append(b, data...)
}

func TestDecodeDDS(t *testing.T) {
	red, blue := []byte{0x00, 0xF8}, []byte{0x1F, 0x00} // 565, little endian
	// colour block: pixel 0 → c0, pixel 1 → c1, pixel 2 → index 2, pixel 3 → index 3
	colour := func(c0, c1 []byte) []byte {
		return append(append(append([]byte{}, c0...), c1...), 0b11100100, 0, 0, 0)
	}
	none := [4]uint32{}
	bc2 := func(alpha ...byte) []byte {
		return append(append(alpha, make([]byte, 8-len(alpha))...), colour(blue, red)...)
	}
	bc3 := func(a0, a1, idx byte) []byte {
		return append([]byte{a0, a1, idx, 0, 0, 0, 0, 0}, colour(red, blue)...)
	}
	for _, c := range []struct {
		name string
		data []byte
		x    int
		want color.NRGBA
	}{
		// BC1, four colours (c0 > c1): index 2 = 2/3 c0 + 1/3 c1, index 3 = 1/3 c0 + 2/3 c1
		{"bc1 c0", ddsFile(4, 4, "DXT1", 0, none, colour(red, blue)), 0, color.NRGBA{255, 0, 0, 255}},
		{"bc1 c1", ddsFile(4, 4, "DXT1", 0, none, colour(red, blue)), 1, color.NRGBA{0, 0, 255, 255}},
		{"bc1 two thirds", ddsFile(4, 4, "DXT1", 0, none, colour(red, blue)), 2, color.NRGBA{170, 0, 85, 255}},
		{"bc1 one third", ddsFile(4, 4, "DXT1", 0, none, colour(red, blue)), 3, color.NRGBA{85, 0, 170, 255}},
		// BC1, three colours (c0 <= c1): index 2 = the middle, index 3 = transparent
		{"bc1 middle", ddsFile(4, 4, "DXT1", 0, none, colour(blue, red)), 2, color.NRGBA{127, 0, 127, 255}},
		{"bc1 transparent", ddsFile(4, 4, "DXT1", 0, none, colour(blue, red)), 3, color.NRGBA{0, 0, 0, 0}},
		// BC2: 4-bit alpha per pixel (pixel 0: 0xF, pixel 1: 0x8); the colour is always four-colour
		{"bc2 opaque", ddsFile(4, 4, "DXT3", 0, none, bc2(0x8F)), 0, color.NRGBA{0, 0, 255, 255}},
		{"bc2 half", ddsFile(4, 4, "DXT3", 0, none, bc2(0x8F)), 1, color.NRGBA{255, 0, 0, 136}},
		{"bc2 four colours", ddsFile(4, 4, "DXT3", 0, none, bc2(0xFF, 0xFF)), 3, color.NRGBA{170, 0, 85, 255}},
		// BC3: a0 200 > a1 100; indices 0, 1, 2 for pixels 0..2: 200, 100, (6·200+100)/7 = 185
		{"bc3 a0", ddsFile(4, 4, "DXT5", 0, none, bc3(200, 100, 0b10001000)), 0, color.NRGBA{255, 0, 0, 200}},
		{"bc3 a1", ddsFile(4, 4, "DXT5", 0, none, bc3(200, 100, 0b10001000)), 1, color.NRGBA{0, 0, 255, 100}},
		{"bc3 interpolated", ddsFile(4, 4, "DXT5", 0, none, bc3(200, 100, 0b10001000)), 2, color.NRGBA{170, 0, 85, 185}},
		// BC3 with a0 <= a1: index 6 = 0, index 7 = 255
		{"bc3 zero", ddsFile(4, 4, "DXT5", 0, none, bc3(100, 200, 0b00111110)), 0, color.NRGBA{255, 0, 0, 0}},
		{"bc3 full", ddsFile(4, 4, "DXT5", 0, none, bc3(100, 200, 0b00111110)), 1, color.NRGBA{0, 0, 255, 255}},
		// uncompressed: BGRA bytes (the A8R8G8B8 masks), BGR in 24 bits, and RGBA bytes
		{"bgra", ddsFile(2, 1, "", 32, [4]uint32{0xFF0000, 0xFF00, 0xFF, 0xFF000000}, []byte{1, 2, 3, 4, 5, 6, 7, 8}), 1, color.NRGBA{7, 6, 5, 8}},
		{"bgr", ddsFile(2, 1, "", 24, [4]uint32{0xFF0000, 0xFF00, 0xFF, 0}, []byte{1, 2, 3, 5, 6, 7}), 1, color.NRGBA{7, 6, 5, 255}},
		{"rgba", ddsFile(2, 1, "", 32, [4]uint32{0xFF, 0xFF00, 0xFF0000, 0xFF000000}, []byte{1, 2, 3, 4, 5, 6, 7, 8}), 0, color.NRGBA{1, 2, 3, 4}},
	} {
		img, err := DecodeDDS(bytes.NewReader(c.data))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if got := img.NRGBAAt(c.x, 0); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// a size that is no multiple of 4 keeps only the pixels inside it; only the top mip is read
	two := append(colour(red, blue), colour(blue, red)...)
	img, err := DecodeDDS(bytes.NewReader(append(ddsFile(5, 2, "DXT1", 0, none, two), 0xEE, 0xEE)))
	if err != nil || img.Rect.Dx() != 5 || img.Rect.Dy() != 2 || img.NRGBAAt(4, 0) != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatalf("5×2: %v %v", img, err)
	}
	FlipV(img)
	if img.NRGBAAt(0, 1) != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("flipped: %v", img.NRGBAAt(0, 1))
	}
	luminance := ddsFile(4, 4, "", 8, [4]uint32{0xFF, 0, 0, 0}, make([]byte, 16))
	luminance[80] = 0x2
	for name, data := range map[string][]byte{
		"not a dds":    []byte("PNG and then some more bytes"),
		"dx10":         ddsFile(4, 4, "DX10", 0, none, make([]byte, 64)),
		"luminance":    luminance,
		"short":        ddsFile(8, 8, "DXT5", 0, none, make([]byte, 16)),
		"too large":    ddsFile(65537, 4, "DXT1", 0, none, nil),
		"over 40 MP":   ddsFile(8000, 6000, "DXT1", 0, none, nil),
		"zero":         ddsFile(0, 4, "DXT1", 0, none, nil),
		"short header": []byte("DDS "),
	} {
		_, err := DecodeDDS(bytes.NewReader(data))
		if err == nil {
			t.Errorf("%s: decoded", name)
		} else if (name == "dx10" || name == "luminance" || name == "not a dds") && !errors.Is(err, ErrDDS) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestShown(t *testing.T) {
	png, dds, bc7, psd := &File{Format: "png"}, &File{Format: "dds:DXT5"}, &File{Format: "dds:DX10"}, &File{Format: "psd"}
	with := func(f, alt *File) *File { c := *f; c.Alt = alt; return &c }
	if Shown(with(dds, png)) != png || Shown(with(bc7, png)) != png {
		t.Fatal("the PNG twin is not preferred")
	}
	if f := with(dds, nil); Shown(f) != f {
		t.Fatal("a DXT5 file alone is not shown")
	}
	if Shown(with(psd, dds)) != dds {
		t.Fatal("the decodable twin is not used")
	}
	if Shown(with(bc7, nil)) != nil || Shown(psd) != nil || Shown(nil) != nil {
		t.Fatal("a format without a decoder is shown")
	}
	if Previewable("dds:DX10") || Previewable("psd") || Previewable("dds:?") || !Previewable("jpg") || !Previewable("dds:RGBA") {
		t.Fatal("Previewable")
	}
}
