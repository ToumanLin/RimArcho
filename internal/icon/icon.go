// Package icon draws RimArcho's icon, a keystone arch (08 §15), from geometry: a PNG at any
// size for the window and the Windows resources, and the same shapes as SVG. Nothing binary
// is kept in the repository.
package icon

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
)

// Colors (08 §15.1).
var (
	Base     = color.NRGBA{0x1B, 0x1D, 0x23, 0xFF}
	Stone    = color.NRGBA{0xE8, 0xE4, 0xDA, 0xFF}
	Keystone = color.NRGBA{0xE8, 0xA9, 0x3A, 0xFF}
)

// Sizes are the icon sizes of the Windows icon group (08 §15.3).
var Sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

// Geometry in the unit square.
const (
	cornerR = 0.22 // rounded base
	cx      = 0.5
)

// layout is the arch for one level of detail (08 §15.1).
type layout struct {
	cy, outerR, innerR float64 // arch centre height and radii
	keyOut             float64 // the keystone stands out this much
	footY              float64 // bottom of the pillars
	bounds             []float64
	key                int  // index of the keystone in bounds
	pillarJoint        bool // a joint between the springing stones and the pillars
}

var (
	large = layout{cy: 0.60, outerR: 0.36, innerR: 0.20, keyOut: 0.035, footY: 0.80,
		bounds: []float64{180, 144, 108, 72, 36, 0}, key: 2, pillarJoint: true}
	// Small sizes: three stones, a heavier arch, and only the keystone's joints.
	small = layout{cy: 0.58, outerR: 0.40, innerR: 0.17, keyOut: 0.03, footY: 0.86,
		bounds: []float64{180, 114, 66, 0}, key: 1}
)

type shape struct {
	pts []pt // polygon; nil for the base
	col color.NRGBA
}

type pt struct{ x, y float64 }

// shapes lays out the icon for a pixel size: small sizes (≤ 24 px) get three arch stones
// instead of five, and the joints are at least one pixel wide.
func shapes(size int) []shape {
	gap := math.Max(0.018, 1.1/float64(size)) // joint width
	l := large
	if size <= 24 {
		l = small
	}
	cy, outerR, innerR, bounds, key := l.cy, l.outerR, l.innerR, l.bounds, l.key
	out := []shape{{col: Base}}
	at := func(r, deg float64) pt {
		a := deg * math.Pi / 180
		return pt{cx + r*math.Cos(a), cy - r*math.Sin(a)}
	}
	for i := 0; i+1 < len(bounds); i++ {
		a1, a2 := bounds[i], bounds[i+1] // a1 > a2: left to right
		ro, col := outerR, Stone
		if i == key {
			ro, col = outerR+l.keyOut, Keystone
		}
		// Each inner radial edge moves in by half a joint, measured along the arc at radius r;
		// the springing edges (180° and 0°) stay on the pillars.
		d := func(r float64) float64 { return gap / 2 / r * 180 / math.Pi }
		var poly []pt
		ins := func(r float64, first bool) float64 {
			switch {
			case first && i == 0:
				return a1
			case first:
				return a1 - d(r)
			case i == len(bounds)-2:
				return a2
			}
			return a2 + d(r)
		}
		// Outer edge from a1 to a2 (a few points so the keystone reads as an arc), then inner back.
		const steps = 4
		s, e := ins(ro, true), ins(ro, false)
		for k := 0; k <= steps; k++ {
			poly = append(poly, at(ro, s+(e-s)*float64(k)/steps))
		}
		s, e = ins(innerR, false), ins(innerR, true)
		for k := 0; k <= steps; k++ {
			poly = append(poly, at(innerR, s+(e-s)*float64(k)/steps))
		}
		out = append(out, shape{poly, col})
	}
	// Pillars under the springing stones, a joint below them.
	top := cy
	if l.pillarJoint {
		top += gap
	}
	for _, x := range [][2]float64{{cx - outerR, cx - innerR}, {cx + innerR, cx + outerR}} {
		out = append(out, shape{[]pt{{x[0], top}, {x[1], top}, {x[1], l.footY}, {x[0], l.footY}}, Stone})
	}
	return out
}

func inRoundRect(x, y float64) bool {
	if x < 0 || y < 0 || x > 1 || y > 1 {
		return false
	}
	qx := math.Max(math.Abs(x-0.5)-(0.5-cornerR), 0)
	qy := math.Max(math.Abs(y-0.5)-(0.5-cornerR), 0)
	return qx*qx+qy*qy <= cornerR*cornerR
}

func inPoly(p []pt, x, y float64) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		if (p[i].y > y) != (p[j].y > y) && x < (p[j].x-p[i].x)*(y-p[i].y)/(p[j].y-p[i].y)+p[i].x {
			in = !in
		}
	}
	return in
}

// Image renders the icon at size×size with supersampled anti-aliasing.
func Image(size int) *image.NRGBA {
	ss := 8
	if size <= 64 {
		ss = 12
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	sh := shapes(size)
	n := float64(ss * ss)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/float64(ss)) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/float64(ss)) / float64(size)
					if !inRoundRect(x, y) {
						continue
					}
					c := sh[0].col
					for _, s := range sh[1:] {
						if inPoly(s.pts, x, y) {
							c = s.col
						}
					}
					r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(b/a + 0.5), uint8(a/n*255 + 0.5)})
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

// SVG is the large-size icon as a 256×256 SVG document.
func SVG() string {
	const k = 256.0
	hex := func(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256">`+"\n")
	fmt.Fprintf(&b, `<title>RimArcho</title>`+"\n")
	fmt.Fprintf(&b, `<rect width="256" height="256" rx="%.1f" fill="%s"/>`+"\n", cornerR*k, hex(Base))
	for _, s := range shapes(256)[1:] {
		var p []string
		for _, q := range s.pts {
			p = append(p, fmt.Sprintf("%.1f,%.1f", q.x*k, q.y*k))
		}
		fmt.Fprintf(&b, `<polygon points="%s" fill="%s"/>`+"\n", strings.Join(p, " "), hex(s.col))
	}
	b.WriteString("</svg>\n")
	return b.String()
}
