// Package icon draws RimArcho's icon (08 §15): the portrait artwork (portrait.svg) on a dark
// rounded base. It renders PNGs at any size for the window and the Windows resources, and
// the same composition as SVG for the page. Nothing binary is kept in the repository.
package icon

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strings"
	"sync"
)

// Portrait is the artwork, an 80×80 SVG (provided by the author, 2026-10-20).
//
//go:embed portrait.svg
var Portrait []byte

// Base is the rounded square behind the portrait (08 §15.1).
var Base = color.NRGBA{0x1B, 0x1D, 0x23, 0xFF}

// Sizes are the icon sizes of the Windows icon group (08 §15.3).
var Sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

const cornerR = 0.22 // rounded base, as a fraction of the size

// crop is the part of the artwork's viewBox shown at a size: the whole portrait from 48 px,
// the face below that so it stays readable.
func crop(size int) (x, y, w float64) {
	if size < 48 {
		return 20, 24, 44
	}
	return 0, 0, 80
}

var (
	artOnce sync.Once
	art     *artwork
)

func portrait() *artwork {
	artOnce.Do(func() {
		var err error
		if art, err = parseArtwork(Portrait); err != nil {
			panic(err) // the embedded file is checked by the tests
		}
	})
	return art
}

func inRoundRect(x, y float64) bool {
	if x < 0 || y < 0 || x > 1 || y > 1 {
		return false
	}
	qx := math.Max(math.Abs(x-0.5)-(0.5-cornerR), 0)
	qy := math.Max(math.Abs(y-0.5)-(0.5-cornerR), 0)
	return qx*qx+qy*qy <= cornerR*cornerR
}

// roundRectCoverage is the share of a pixel inside the rounded base (4×4 samples).
func roundRectCoverage(px, py, size int) float64 {
	const ss = 4
	in := 0
	for sy := 0; sy < ss; sy++ {
		for sx := 0; sx < ss; sx++ {
			x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size)
			y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size)
			if inRoundRect(x, y) {
				in++
			}
		}
	}
	return float64(in) / ss / ss
}

// Image renders the icon at size×size: the base, the portrait over it, both clipped to the
// rounded square.
func Image(size int) *image.NRGBA {
	cx, cy, cw := crop(size)
	fg := portrait().render(size, cx, cy, cw, cw)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			cov := roundRectCoverage(x, y, size)
			if cov == 0 {
				continue
			}
			f := fg.NRGBAAt(x, y)
			a := float64(f.A) / 255
			mix := func(top, base uint8) uint8 { return uint8(math.Round(float64(top)*a + float64(base)*(1-a))) }
			img.SetNRGBA(x, y, color.NRGBA{mix(f.R, Base.R), mix(f.G, Base.G), mix(f.B, Base.B), uint8(math.Round(cov * 255))})
		}
	}
	return img
}

// PNG encodes the icon at a size.
func PNG(size int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, Image(size)); err != nil {
		panic(err) // encoding an in-memory NRGBA cannot fail
	}
	return buf.Bytes()
}

var svgOpen = regexp.MustCompile(`(?s)^.*?<svg[^>]*>`)

// SVG is the icon as a 256×256 SVG document showing what a size×size image shows (the face
// only below 48 px).
func SVG(size int) string {
	const k = 256.0
	cx, cy, cw := crop(size)
	inner := svgOpen.ReplaceAllString(string(Portrait), "")
	inner = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(inner), "</svg>"))
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256">`+"\n")
	fmt.Fprintf(&b, `<title>RimArcho</title>`+"\n")
	fmt.Fprintf(&b, `<clipPath id="rimarcho-base"><rect width="256" height="256" rx="%.1f"/></clipPath>`+"\n", cornerR*k)
	fmt.Fprintf(&b, `<g clip-path="url(#rimarcho-base)">`+"\n")
	fmt.Fprintf(&b, `<rect width="256" height="256" fill="#%02X%02X%02X"/>`+"\n", Base.R, Base.G, Base.B)
	fmt.Fprintf(&b, `<svg width="256" height="256" viewBox="%g %g %g %g" fill="none">`+"\n%s\n</svg>\n", cx, cy, cw, cw, inner)
	b.WriteString("</g>\n</svg>\n")
	return b.String()
}
