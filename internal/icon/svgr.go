package icon

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
)

// A small SVG renderer for the icon artwork (08 §15): paths (absolute and relative m, l, h,
// v, c, s, q, z), circles and ellipses, fills (nonzero), strokes (round joins), solid
// colours and linear gradients in user space, opacity. Enough for the portrait, no
// dependency, nothing binary in the repository.

type paint struct {
	solid color.NRGBA
	grad  *gradient
	none  bool
}

type gradient struct {
	x1, y1, x2, y2 float64
	stops          []stop
}

type stop struct {
	off float64
	c   color.NRGBA
}

type pt struct{ x, y float64 }

type drawOp struct {
	polys       [][]pt // subpaths, flattened
	fill        paint
	stroke      paint
	strokeW     float64
	opacity     float64
	fillOpacity float64
}

type artwork struct {
	vx, vy, vw, vh float64
	ops            []drawOp
}

func parseColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	switch s {
	case "white":
		return color.NRGBA{255, 255, 255, 255}, true
	case "black":
		return color.NRGBA{0, 0, 0, 255}, true
	}
	if !strings.HasPrefix(s, "#") {
		return color.NRGBA{}, false
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil || len(h) != 6 {
		return color.NRGBA{}, false
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
}

func attr(se xml.StartElement, name string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func num(se xml.StartElement, name string, def float64) float64 {
	if v, ok := attr(se, name); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

func parseArtwork(src []byte) (*artwork, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	art := &artwork{vw: 1, vh: 1}
	grads := map[string]*gradient{}
	var cur *gradient
	var pending []xml.StartElement // shapes, resolved after the gradients are known
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "svg":
				if vb, ok := attr(t, "viewBox"); ok {
					f := strings.Fields(strings.ReplaceAll(vb, ",", " "))
					if len(f) == 4 {
						art.vx, _ = strconv.ParseFloat(f[0], 64)
						art.vy, _ = strconv.ParseFloat(f[1], 64)
						art.vw, _ = strconv.ParseFloat(f[2], 64)
						art.vh, _ = strconv.ParseFloat(f[3], 64)
					}
				}
			case "linearGradient":
				id, _ := attr(t, "id")
				cur = &gradient{x1: num(t, "x1", 0), y1: num(t, "y1", 0), x2: num(t, "x2", 1), y2: num(t, "y2", 0)}
				grads[id] = cur
			case "stop":
				if cur != nil {
					c, _ := parseColor(func() string { v, _ := attr(t, "stop-color"); return v }())
					c.A = uint8(255 * num(t, "stop-opacity", 1))
					cur.stops = append(cur.stops, stop{off: num(t, "offset", 0), c: c})
				}
			case "path", "circle", "ellipse", "rect":
				pending = append(pending, t.Copy())
			}
		case xml.EndElement:
			if t.Name.Local == "linearGradient" {
				cur = nil
			}
		}
	}
	paintOf := func(v string, ok bool, def paint) paint {
		if !ok {
			return def
		}
		v = strings.TrimSpace(v)
		if v == "none" {
			return paint{none: true}
		}
		if strings.HasPrefix(v, "url(#") {
			if g := grads[strings.TrimSuffix(strings.TrimPrefix(v, "url(#"), ")")]; g != nil {
				return paint{grad: g}
			}
			return paint{none: true}
		}
		if c, ok := parseColor(v); ok {
			return paint{solid: c}
		}
		return def
	}
	for _, se := range pending {
		// The root sets fill="none"; a shape without fill inherits it.
		op := drawOp{opacity: num(se, "opacity", 1), fillOpacity: num(se, "fill-opacity", 1), strokeW: num(se, "stroke-width", 1)}
		fv, fok := attr(se, "fill")
		op.fill = paintOf(fv, fok, paint{none: true})
		sv, sok := attr(se, "stroke")
		op.stroke = paintOf(sv, sok, paint{none: true})
		switch se.Name.Local {
		case "path":
			dv, _ := attr(se, "d")
			op.polys = flattenPath(dv)
		case "circle":
			r := num(se, "r", 0)
			op.polys = [][]pt{ellipse(num(se, "cx", 0), num(se, "cy", 0), r, r)}
		case "ellipse":
			op.polys = [][]pt{ellipse(num(se, "cx", 0), num(se, "cy", 0), num(se, "rx", 0), num(se, "ry", 0))}
		case "rect":
			x, y, w, h := num(se, "x", 0), num(se, "y", 0), num(se, "width", 0), num(se, "height", 0)
			op.polys = [][]pt{{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}, {x, y}}}
		}
		art.ops = append(art.ops, op)
	}
	return art, nil
}

func ellipse(cx, cy, rx, ry float64) []pt {
	const n = 48
	p := make([]pt, 0, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		p = append(p, pt{cx + rx*math.Cos(a), cy + ry*math.Sin(a)})
	}
	return p
}

// pathTokens splits path data into commands and numbers ("-.5.5" is two numbers).
func pathTokens(d string) []string {
	var out []string
	i := 0
	for i < len(d) {
		c := d[i]
		switch {
		case c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r':
			i++
		case (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') && c != 'e' && c != 'E':
			out = append(out, string(c))
			i++
		default:
			j := i
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			dot := false
			for j < len(d) {
				ch := d[j]
				if ch >= '0' && ch <= '9' {
					j++
				} else if ch == '.' && !dot {
					dot = true
					j++
				} else if (ch == 'e' || ch == 'E') && j+1 < len(d) {
					j++
					if d[j] == '-' || d[j] == '+' {
						j++
					}
				} else {
					break
				}
			}
			if j == i {
				j++ // skip an unknown character
			}
			out = append(out, d[i:j])
			i = j
		}
	}
	return out
}

func flattenPath(d string) [][]pt {
	toks := pathTokens(d)
	var polys [][]pt
	var cur []pt
	var x, y, sx, sy, lcx, lcy float64 // current point, subpath start, last control point
	var cmd byte
	k := 0
	next := func() float64 {
		if k >= len(toks) {
			return 0
		}
		f, _ := strconv.ParseFloat(toks[k], 64)
		k++
		return f
	}
	isNum := func() bool {
		if k >= len(toks) {
			return false
		}
		c := toks[k][0]
		return c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9')
	}
	flush := func() {
		if len(cur) > 1 {
			polys = append(polys, cur)
		}
		cur = nil
	}
	cubic := func(x1, y1, x2, y2, x3, y3 float64) {
		const n = 12
		for i := 1; i <= n; i++ {
			t := float64(i) / n
			u := 1 - t
			cur = append(cur, pt{u*u*u*x + 3*u*u*t*x1 + 3*u*t*t*x2 + t*t*t*x3, u*u*u*y + 3*u*u*t*y1 + 3*u*t*t*y2 + t*t*t*y3})
		}
		lcx, lcy, x, y = x2, y2, x3, y3
	}
	for k < len(toks) {
		if !isNum() {
			cmd = toks[k][0]
			k++
		}
		rel := cmd >= 'a'
		ox, oy := 0.0, 0.0
		if rel {
			ox, oy = x, y
		}
		switch cmd | 0x20 {
		case 'm':
			flush()
			x, y = ox+next(), oy+next()
			sx, sy = x, y
			cur = []pt{{x, y}}
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
			lcx, lcy = x, y
			continue
		case 'l':
			x, y = ox+next(), oy+next()
		case 'h':
			x = ox + next()
		case 'v':
			y = oy + next()
		case 'c':
			x1, y1 := ox+next(), oy+next()
			x2, y2 := ox+next(), oy+next()
			x3, y3 := ox+next(), oy+next()
			cubic(x1, y1, x2, y2, x3, y3)
			continue
		case 's':
			x1, y1 := 2*x-lcx, 2*y-lcy
			x2, y2 := ox+next(), oy+next()
			x3, y3 := ox+next(), oy+next()
			cubic(x1, y1, x2, y2, x3, y3)
			continue
		case 'q':
			qx, qy := ox+next(), oy+next()
			x3, y3 := ox+next(), oy+next()
			cubic(x+2.0/3*(qx-x), y+2.0/3*(qy-y), x3+2.0/3*(qx-x3), y3+2.0/3*(qy-y3), x3, y3)
			continue
		case 'z':
			cur = append(cur, pt{sx, sy})
			x, y = sx, sy
			flush()
			cur = []pt{{x, y}}
			lcx, lcy = x, y
			continue
		default:
			k++ // unsupported command: skip a token
			continue
		}
		cur = append(cur, pt{x, y})
		lcx, lcy = x, y
	}
	flush()
	return polys
}

// render draws the artwork's viewBox region (bx, by, bw, bh) into an image of size×size.
// It rasterises at 4× and averages down.
func (art *artwork) render(size int, bx, by, bw, bh float64) *image.NRGBA {
	const ss = 4
	n := size * ss
	acc := make([]float64, n*n*4) // premultiplied RGBA
	scale := float64(n) / math.Max(bw, bh)
	toPx := func(p pt) pt { return pt{(p.x - bx) * scale, (p.y - by) * scale} }
	cov := make([]float32, n*n)
	for _, op := range art.ops {
		if !op.fill.none && len(op.polys) > 0 {
			var polys [][]pt
			for _, pl := range op.polys {
				q := make([]pt, len(pl))
				for i, p := range pl {
					q[i] = toPx(p)
				}
				polys = append(polys, q)
			}
			fillMask(cov, n, polys)
			blend(acc, cov, n, op.fill, op.opacity*op.fillOpacity, scale, bx, by)
		}
		if !op.stroke.none && op.strokeW > 0 {
			w := math.Max(op.strokeW*scale, 1.0)
			var polys [][]pt
			for _, pl := range op.polys {
				for i := 1; i < len(pl); i++ {
					a, b := toPx(pl[i-1]), toPx(pl[i])
					if q := segmentQuad(a, b, w/2); q != nil {
						polys = append(polys, q)
					}
				}
				for _, p := range pl {
					c := toPx(p)
					polys = append(polys, ellipse(c.x, c.y, w/2, w/2))
				}
			}
			for _, p := range polys {
				clockwise(p) // one orientation: overlapping pieces add up instead of cancelling
			}
			fillMask(cov, n, polys)
			blend(acc, cov, n, op.stroke, op.opacity, scale, bx, by)
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					i := ((y*ss+dy)*n + x*ss + dx) * 4
					r, g, b, a = r+acc[i], g+acc[i+1], b+acc[i+2], a+acc[i+3]
				}
			}
			k := float64(ss * ss)
			r, g, b, a = r/k, g/k, b/k, a/k
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(r / a * 255)), uint8(math.Round(g / a * 255)), uint8(math.Round(b / a * 255)), uint8(math.Round(a * 255))})
			}
		}
	}
	return img
}

// clockwise reverses p in place when its signed area is negative.
func clockwise(p []pt) {
	var s float64
	for i := 1; i < len(p); i++ {
		s += p[i-1].x*p[i].y - p[i].x*p[i-1].y
	}
	if s < 0 {
		for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
			p[i], p[j] = p[j], p[i]
		}
	}
}

func segmentQuad(a, b pt, hw float64) []pt {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	nx, ny := -dy/l*hw, dx/l*hw
	return []pt{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}, {a.x + nx, a.y + ny}}
}

// fillMask sets cov to 1 inside the polygons (nonzero rule, sampled at pixel centres) and 0
// elsewhere; overlapping stroke pieces count once.
func fillMask(cov []float32, n int, polys [][]pt) {
	for i := range cov {
		cov[i] = 0
	}
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range polys {
		for _, q := range p {
			minY, maxY = math.Min(minY, q.y), math.Max(maxY, q.y)
		}
	}
	y0, y1 := max(0, int(math.Floor(minY))), min(n-1, int(math.Ceil(maxY)))
	type cross struct {
		x float64
		w int
	}
	var xs []cross
	for y := y0; y <= y1; y++ {
		sy := float64(y) + 0.5
		xs = xs[:0]
		for _, p := range polys {
			for i := 1; i < len(p); i++ {
				a, b := p[i-1], p[i]
				if (a.y <= sy) == (b.y <= sy) {
					continue
				}
				w := 1
				if b.y < a.y {
					w = -1
				}
				xs = append(xs, cross{a.x + (sy-a.y)/(b.y-a.y)*(b.x-a.x), w})
			}
		}
		if len(xs) < 2 {
			continue
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
		wind := 0
		for i := 0; i < len(xs)-1; i++ {
			wind += xs[i].w
			if wind == 0 {
				continue
			}
			xa, xb := max(0, int(math.Ceil(xs[i].x-0.5))), min(n-1, int(math.Floor(xs[i+1].x-0.5)))
			for x := xa; x <= xb; x++ {
				cov[y*n+x] = 1
			}
		}
	}
}

func (g *gradient) at(x, y float64) color.NRGBA {
	dx, dy := g.x2-g.x1, g.y2-g.y1
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = ((x-g.x1)*dx + (y-g.y1)*dy) / l
	}
	if len(g.stops) == 0 {
		return color.NRGBA{}
	}
	if t <= g.stops[0].off {
		return g.stops[0].c
	}
	for i := 1; i < len(g.stops); i++ {
		a, b := g.stops[i-1], g.stops[i]
		if t <= b.off {
			f := (t - a.off) / math.Max(b.off-a.off, 1e-9)
			mix := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*f) }
			return color.NRGBA{mix(a.c.R, b.c.R), mix(a.c.G, b.c.G), mix(a.c.B, b.c.B), mix(a.c.A, b.c.A)}
		}
	}
	return g.stops[len(g.stops)-1].c
}

// blend composites paint through the mask onto acc (source-over, premultiplied).
func blend(acc []float64, cov []float32, n int, p paint, opacity, scale, bx, by float64) {
	for i, c := range cov {
		if c == 0 {
			continue
		}
		col := p.solid
		if p.grad != nil {
			x, y := float64(i%n)+0.5, float64(i/n)+0.5
			col = p.grad.at(x/scale+bx, y/scale+by)
		}
		a := float64(col.A) / 255 * opacity * float64(c)
		j := i * 4
		k := 1 - a
		acc[j] = float64(col.R)/255*a + acc[j]*k
		acc[j+1] = float64(col.G)/255*a + acc[j+1]*k
		acc[j+2] = float64(col.B)/255*a + acc[j+2]*k
		acc[j+3] = a + acc[j+3]*k
	}
}
