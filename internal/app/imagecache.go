package app

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// Thumbnails and description images share one way to be made (16 §4.4): decode once,
// scale down with a box filter, keep the small PNG in the cache.

// decodeSlots bounds how many full-size pictures are decoded at once. A 4K picture
// takes 30–45 MB while it is decoded and scaled, and a page of thumbnails asks for
// many together; the rest wait their turn instead of adding up.
var decodeSlots = make(chan struct{}, 2)

// errImageSize: the picture is empty or larger than 40 megapixels.
var errImageSize = errors.New("size out of range")

// decodeError: the file is not a picture the decoders read.
type decodeError struct{ error }

// cachedPNG reports the size of a cached PNG, if it is there and readable.
func cachedPNG(path string) (w, h int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	return cfg.Width, cfg.Height, err == nil
}

// makeCachedPNG writes the picture read by open, scaled to at most maxW wide, to path,
// unless another request made it while this one waited for a decode slot. open is
// called only once a slot is free.
func makeCachedPNG(path string, maxW int, open func() (io.ReadSeeker, func(), error)) (w, h int, err error) {
	decodeSlots <- struct{}{}
	defer func() { <-decodeSlots }()
	if w, h, ok := cachedPNG(path); ok {
		return w, h, nil
	}
	r, done, err := open()
	if err != nil {
		return 0, 0, err
	}
	defer done()
	// Refuse absurd sizes before the decoder allocates the pixels.
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, decodeError{err}
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > 40e6 {
		return 0, 0, errImageSize
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, 0, err
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return 0, 0, decodeError{err}
	}
	b := img.Bounds()
	w = min(b.Dx(), maxW)
	h = max(b.Dy()*w/b.Dx(), 1)
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, ScaleBox(img, w, h)); err != nil {
		return 0, 0, err
	}
	if err := writeCacheFile(path, buf.Bytes()); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// writeCacheFile replaces path whole: a reader never sees half a file, and two
// requests for the same picture never write into one temporary file.
func writeCacheFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0o755)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// ScaleBox scales with a box filter (averaging the source pixels under each target pixel).
// A picture that is not RGBA is converted a few rows at a time, the rows under one target
// row, so scaling needs no full-size copy beside the decoded picture.
func ScaleBox(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	rows, direct := src.(*image.RGBA)
	var strip []uint8
	for y := 0; y < h; y++ {
		y0, y1 := y*b.Dy()/h, (y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if !direct {
			r := image.Rect(b.Min.X, b.Min.Y+y0, b.Max.X, b.Min.Y+y1)
			n := 4 * r.Dx() * r.Dy()
			if cap(strip) < n {
				strip = make([]uint8, n)
			}
			rows = &image.RGBA{Pix: strip[:n], Stride: 4 * r.Dx(), Rect: r}
			draw.Draw(rows, r, src, r.Min, draw.Src)
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*b.Dx()/w, (x+1)*b.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, al, n uint32
			for yy := y0; yy < y1; yy++ {
				i := rows.PixOffset(b.Min.X+x0, b.Min.Y+yy)
				for xx := x0; xx < x1; xx++ {
					r, g, bl, al, n = r+uint32(rows.Pix[i]), g+uint32(rows.Pix[i+1]), bl+uint32(rows.Pix[i+2]), al+uint32(rows.Pix[i+3]), n+1
					i += 4
				}
			}
			j := dst.PixOffset(x, y)
			dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(bl/n), uint8(al/n)
		}
	}
	return dst
}
