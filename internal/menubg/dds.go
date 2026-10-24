// Package menubg reads RimWorld menu art without writing to the game (06 §2.14).
package menubg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/woozymasta/bcn"
)

// DDS holds only a header and a reader, never a full-size decoded image.
type DDS struct {
	Width, Height int
	r             io.Reader
	format        bcn.Format
	blockSize     int
}

// ReadDDS accepts single 2D BC1/BC3/BC7 textures, including DX10 headers.
// Mipmaps are not needed for the menu; the archive reader still checks their CRC.
func ReadDDS(r io.Reader) (*DDS, error) {
	var h [128]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(h[i:]) }
	if string(h[:4]) != "DDS " || u32(4) != 124 || u32(76) != 32 {
		return nil, errors.New("invalid DDS header")
	}
	d := &DDS{Width: int(u32(16)), Height: int(u32(12)), r: r, blockSize: 16}
	if d.Width < 1 || d.Height < 1 || d.Width > 16384 || d.Height > 16384 || int64(d.Width)*int64(d.Height) > 40e6 || u32(112)&0x20fe00 != 0 || u32(24) > 1 {
		return nil, errors.New("DDS is not a supported 2D image size")
	}
	switch string(h[84:88]) {
	case "DXT1":
		d.format, d.blockSize = bcn.FormatBC1, 8
	case "DXT5":
		d.format = bcn.FormatBC3
	case "DX10":
		var dx [20]byte
		if _, err := io.ReadFull(r, dx[:]); err != nil {
			return nil, err
		}
		if binary.LittleEndian.Uint32(dx[4:]) != 3 || binary.LittleEndian.Uint32(dx[8:])&4 != 0 || binary.LittleEndian.Uint32(dx[12:]) != 1 {
			return nil, errors.New("DDS arrays, cubemaps and volumes are unsupported")
		}
		switch binary.LittleEndian.Uint32(dx[:]) {
		case 71, 72:
			d.format, d.blockSize = bcn.FormatBC1, 8
		case 77, 78:
			d.format = bcn.FormatBC3
		case 98, 99:
			d.format = bcn.FormatBC7
		default:
			return nil, errors.New("DDS format is not BC1, BC3 or BC7")
		}
	default:
		return nil, errors.New("DDS format is not BC1, BC3 or BC7")
	}
	return d, nil
}

// Rows decodes four source rows at a time. The callback must not retain the reused
// pixels. This bounds the full-resolution storage to 4×width pixels (06 §2.14).
func (d *DDS) Rows(ctx context.Context, visit func(y int, rows *image.NRGBA) error) error {
	blocks := make([]byte, ((d.Width+3)/4)*d.blockSize)
	var rows *image.NRGBA
	for y := 0; y < d.Height; y += 4 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.ReadFull(d.r, blocks); err != nil {
			return fmt.Errorf("DDS row %d: %w", y, err)
		}
		var err error
		rows, err = bcn.DecodeImageInto(rows, blocks, d.Width, min(4, d.Height-y), d.format, &bcn.DecodeOptions{Workers: 1})
		if err != nil {
			return err
		}
		if err := visit(y, rows); err != nil {
			return err
		}
	}
	return nil
}

// Scaled produces the two cached images in one streaming decode, using the same box
// filter and alpha-on-black rule as other backgrounds (06 §2.14).
func (d *DDS) Scaled(ctx context.Context, widths ...int) ([]*image.RGBA, error) {
	scalers := make([]*boxScaler, len(widths))
	for i, w := range widths {
		if w < 1 {
			return nil, errors.New("invalid background width")
		}
		w = min(w, d.Width)
		scalers[i] = &boxScaler{image.NewRGBA(image.Rect(0, 0, w, max(1, d.Height*w/d.Width))), make([]uint32, w*3), d.Width, d.Height}
	}
	err := d.Rows(ctx, func(y int, rows *image.NRGBA) error {
		for yy := 0; yy < rows.Rect.Dy(); yy++ {
			for _, s := range scalers {
				s.row(y+yy, rows.Pix[yy*rows.Stride:][:d.Width*4])
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]*image.RGBA, len(scalers))
	for i, s := range scalers {
		out[i] = s.dst
	}
	return out, nil
}

type boxScaler struct {
	dst  *image.RGBA
	sums []uint32
	w, h int
}

func (s *boxScaler) row(y int, pixels []byte) {
	w, h := s.dst.Rect.Dx(), s.dst.Rect.Dy()
	dy := ((y+1)*h - 1) / s.h
	y0, y1 := dy*s.h/h, (dy+1)*s.h/h
	for x := 0; x < w; x++ {
		x0, x1 := x*s.w/w, (x+1)*s.w/w
		i := x * 3
		for sx := x0; sx < x1; sx++ {
			p := pixels[sx*4:][:4]
			a := uint32(p[3])
			s.sums[i] += uint32(p[0]) * a / 255
			s.sums[i+1] += uint32(p[1]) * a / 255
			s.sums[i+2] += uint32(p[2]) * a / 255
		}
		if y+1 == y1 {
			n := uint32((x1 - x0) * (y1 - y0))
			p := s.dst.Pix[dy*s.dst.Stride+x*4:][:4]
			p[0], p[1], p[2], p[3] = uint8(s.sums[i]/n), uint8(s.sums[i+1]/n), uint8(s.sums[i+2]/n), 255
			s.sums[i], s.sums[i+1], s.sums[i+2] = 0, 0, 0
		}
	}
}
