package looks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math/bits"
)

// A small DDS reader for showing a texture (16 §4.4): the top mip of uncompressed BGR(A) /
// RGB(A) files and of BC1 (DXT1), BC2 (DXT3) and BC3 (DXT5). BC7 and every other DX10
// format are not decoded: Previewable says so and the interface shows no preview.

// MaxPixels is the largest picture any image code decodes (16 §4.4): 40 megapixels.
const MaxPixels = 40e6

// ErrDDS: the file is not a DDS this reader decodes.
var ErrDDS = errors.New("dds: not a format this reader decodes")

// Previewable reports whether a File.Format can be shown: PNG and JPEG, and the DDS
// layouts DecodeDDS reads.
func Previewable(format string) bool {
	switch format {
	case "png", "jpg", "dds:DXT1", "dds:DXT3", "dds:DXT5", "dds:RGB", "dds:RGBA":
		return true
	}
	return false
}

// Shown is the file an interface shows for f: the PNG when the provider has both formats,
// else whichever of the two can be decoded; nil when neither can.
func Shown(f *File) *File {
	if f == nil {
		return nil
	}
	isPNG := func(g *File) bool { return g.Format == "png" }
	any := func(g *File) bool { return Previewable(g.Format) }
	for _, want := range []func(*File) bool{isPNG, any} {
		for _, g := range []*File{f, f.Alt} {
			if g != nil && want(g) {
				return g
			}
		}
	}
	return nil
}

// DecodeDDS reads the top mip of a DDS file as stored: row 0 of the result is the file's
// first row. Files converted for the game hold the rows bottom first (10 §2), so a caller
// that shows one flips it (FlipV). It reads no more than the header and that mip, and
// refuses a picture over MaxPixels before allocating it.
func DecodeDDS(r io.Reader) (*image.NRGBA, error) {
	var hd [128]byte
	if _, err := io.ReadFull(r, hd[:]); err != nil || string(hd[:4]) != "DDS " {
		return nil, ErrDDS
	}
	le := binary.LittleEndian
	h, w := int(le.Uint32(hd[12:16])), int(le.Uint32(hd[16:20]))
	if w < 1 || h < 1 || w > 1<<16 || h > 1<<16 || float64(w)*float64(h) > MaxPixels {
		return nil, fmt.Errorf("dds: size %d×%d out of range", w, h)
	}
	flags := le.Uint32(hd[80:84])
	if flags&0x4 != 0 { // DDPF_FOURCC
		return decodeBC(r, string(hd[84:88]), w, h)
	}
	if flags&0x40 == 0 { // not DDPF_RGB: luminance, alpha only, …
		return nil, ErrDDS
	}
	bitCount := le.Uint32(hd[88:92])
	n := int(bitCount) / 8
	if n < 2 || n > 4 || bitCount%8 != 0 {
		return nil, ErrDDS
	}
	var masks [4]uint32 // R, G, B, A
	for i := range masks {
		masks[i] = le.Uint32(hd[92+4*i:])
	}
	if flags&0x1 == 0 { // no DDPF_ALPHAPIXELS
		masks[3] = 0
	}
	if masks[0]|masks[1]|masks[2] == 0 {
		return nil, ErrDDS
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	row := make([]byte, w*n)
	for y := 0; y < h; y++ {
		if _, err := io.ReadFull(r, row); err != nil {
			return nil, fmt.Errorf("dds: short file: %w", err)
		}
		for x := 0; x < w; x++ {
			var v uint32
			for i := n - 1; i >= 0; i-- {
				v = v<<8 | uint32(row[x*n+i])
			}
			o := y*img.Stride + x*4
			for c, m := range masks {
				switch {
				case m != 0:
					shift := bits.TrailingZeros32(m)
					img.Pix[o+c] = uint8(uint64((v&m)>>shift) * 255 / uint64(m>>shift))
				case c == 3:
					img.Pix[o+c] = 255
				}
			}
		}
	}
	return img, nil
}

// decodeBC reads the top mip of a block-compressed file.
func decodeBC(r io.Reader, kind string, w, h int) (*image.NRGBA, error) {
	var block int
	switch kind {
	case "DXT1":
		block = 8
	case "DXT3", "DXT5":
		block = 16
	default:
		return nil, ErrDDS
	}
	bw, bh := (w+3)/4, (h+3)/4
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	line := make([]byte, bw*block)
	var px [16][4]uint8
	for by := 0; by < bh; by++ {
		if _, err := io.ReadFull(r, line); err != nil {
			return nil, fmt.Errorf("dds: short file: %w", err)
		}
		for bx := 0; bx < bw; bx++ {
			b := line[bx*block:][:block]
			switch kind {
			case "DXT1":
				bcColor(b, &px, true)
			case "DXT3":
				bcColor(b[8:], &px, false)
				for i := 0; i < 16; i++ {
					px[i][3] = (b[i/2] >> (4 * (i % 2)) & 0xF) * 17
				}
			default:
				bcColor(b[8:], &px, false)
				bc3Alpha(b, &px)
			}
			for i := 0; i < 16; i++ {
				if x, y := bx*4+i%4, by*4+i/4; x < w && y < h {
					copy(img.Pix[y*img.Stride+x*4:], px[i][:])
				}
			}
		}
	}
	return img, nil
}

func rgb565(c uint16) [4]uint8 {
	r, g, b := uint32(c>>11), uint32(c>>5&0x3F), uint32(c&0x1F)
	return [4]uint8{uint8(r * 255 / 31), uint8(g * 255 / 63), uint8(b * 255 / 31), 255}
}

// bcColor decodes the 8-byte colour block of BC1, BC2 and BC3. Only BC1 has the
// three-colour mode whose fourth entry is transparent (punch, when c0 <= c1).
func bcColor(b []byte, px *[16][4]uint8, punch bool) {
	c0, c1 := binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:])
	three := punch && c0 <= c1
	var pal [4][4]uint8
	pal[0], pal[1] = rgb565(c0), rgb565(c1)
	for i := 0; i < 3; i++ {
		a, c := uint32(pal[0][i]), uint32(pal[1][i])
		if three {
			pal[2][i] = uint8((a + c) / 2)
		} else {
			pal[2][i], pal[3][i] = uint8((2*a+c)/3), uint8((a+2*c)/3)
		}
	}
	pal[2][3] = 255
	if !three {
		pal[3][3] = 255
	}
	idx := binary.LittleEndian.Uint32(b[4:])
	for i := 0; i < 16; i++ {
		px[i] = pal[idx>>(2*i)&3]
	}
}

// bc3Alpha decodes the 8-byte alpha block of BC3 into px's alpha.
func bc3Alpha(b []byte, px *[16][4]uint8) {
	var pal [8]uint32
	pal[0], pal[1] = uint32(b[0]), uint32(b[1])
	if pal[0] > pal[1] {
		for i := uint32(1); i < 7; i++ {
			pal[i+1] = ((7-i)*pal[0] + i*pal[1]) / 7
		}
	} else {
		for i := uint32(1); i < 5; i++ {
			pal[i+1] = ((5-i)*pal[0] + i*pal[1]) / 5
		}
		pal[6], pal[7] = 0, 255
	}
	var idx uint64
	for i := 0; i < 6; i++ {
		idx |= uint64(b[2+i]) << (8 * i)
	}
	for i := 0; i < 16; i++ {
		px[i][3] = uint8(pal[idx>>(3*i)&7])
	}
}

// FlipV turns a picture upside down in place.
func FlipV(img *image.NRGBA) {
	h, n := img.Rect.Dy(), img.Rect.Dx()*4
	tmp := make([]byte, n)
	for y := 0; y < h/2; y++ {
		a, b := img.Pix[y*img.Stride:][:n], img.Pix[(h-1-y)*img.Stride:][:n]
		copy(tmp, a)
		copy(a, b)
		copy(b, tmp)
	}
}
