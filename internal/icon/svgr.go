package icon

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A small SVG renderer for the icon artwork (16 §8.6). It covers what the portrait uses:
// groups whose fill and stroke settings are inherited; transforms (matrix, translate, scale,
// rotate, skewX, skewY); paths (absolute and relative m, l, h, v, c, s, q, z), rect, circle,
// ellipse, line, polyline, polygon; nonzero and evenodd fills; strokes (round joins and
// caps); solid colours; linear and radial gradients, in user space or in the shape's bounding
// box, with gradientTransform; opacity, fill-opacity, stroke-opacity; clip-path; the Gaussian
// blur filter. The contents of defs, clipPath, gradients and filters are not drawn. No
// dependency, nothing binary in the repository.

type pt struct{ x, y float64 }

// mat is an affine transform: x' = a·x + c·y + e, y' = b·x + d·y + f.
type mat [6]float64

var ident = mat{1, 0, 0, 1, 0, 0}

// mul is m∘n: n applies first.
func (m mat) mul(n mat) mat {
	return mat{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m mat) apply(p pt) pt { return pt{m[0]*p.x + m[2]*p.y + m[4], m[1]*p.x + m[3]*p.y + m[5]} }

func (m mat) inv() mat {
	det := m[0]*m[3] - m[1]*m[2]
	if det == 0 {
		return ident
	}
	return mat{m[3] / det, -m[1] / det, -m[2] / det, m[0] / det,
		(m[2]*m[5] - m[3]*m[4]) / det, (m[1]*m[4] - m[0]*m[5]) / det}
}

// scale is the transform's average length scale, for stroke widths and blur radii.
func (m mat) scale() float64 { return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2])) }

var transformRe = regexp.MustCompile(`([A-Za-z]+)\s*\(([^)]*)\)`)

func numbers(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		v, _ := strconv.ParseFloat(f, 64)
		out = append(out, v)
	}
	return out
}

func parseTransform(s string) mat {
	m := ident
	for _, t := range transformRe.FindAllStringSubmatch(s, -1) {
		a := numbers(t[2])
		arg := func(i int, def float64) float64 {
			if i < len(a) {
				return a[i]
			}
			return def
		}
		var n mat
		switch t[1] {
		case "matrix":
			n = mat{arg(0, 1), arg(1, 0), arg(2, 0), arg(3, 1), arg(4, 0), arg(5, 0)}
		case "translate":
			n = mat{1, 0, 0, 1, arg(0, 0), arg(1, 0)}
		case "scale":
			sx := arg(0, 1)
			n = mat{sx, 0, 0, arg(1, sx), 0, 0}
		case "rotate":
			r := arg(0, 0) * math.Pi / 180
			cx, cy := arg(1, 0), arg(2, 0)
			n = mat{1, 0, 0, 1, cx, cy}.mul(mat{math.Cos(r), math.Sin(r), -math.Sin(r), math.Cos(r), 0, 0}).mul(mat{1, 0, 0, 1, -cx, -cy})
		case "skewX":
			n = mat{1, 0, math.Tan(arg(0, 0) * math.Pi / 180), 1, 0, 0}
		case "skewY":
			n = mat{1, math.Tan(arg(0, 0) * math.Pi / 180), 0, 1, 0, 0}
		default:
			continue
		}
		m = m.mul(n)
	}
	return m
}

// length reads a number or a percentage (as a fraction).
func length(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "%"), "px")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	if pct {
		v /= 100
	}
	return v
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

// node is an element of the document; style="" declarations are folded into attr.
type node struct {
	name string
	attr map[string]string
	kids []*node
}

func (n *node) num(name string, def float64) float64 { return length(n.attr[name], def) }

func parseTree(src []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	root := &node{name: "#document"}
	stack := []*node{root}
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local, attr: map[string]string{}}
			for _, a := range t.Attr {
				n.attr[a.Name.Local] = a.Value
			}
			for _, decl := range strings.Split(n.attr["style"], ";") {
				if k, v, ok := strings.Cut(decl, ":"); ok {
					n.attr[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
			top := stack[len(stack)-1]
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return root, nil
}

type stop struct {
	off float64
	c   color.NRGBA
}

type gradient struct {
	radial         bool
	bbox           bool // gradientUnits="objectBoundingBox" (the default)
	x1, y1, x2, y2 float64
	cx, cy, r      float64
	tf             mat
	stops          []stop
}

// at is the colour at a point in the gradient's own space (spread: pad).
func (g *gradient) at(p pt) color.NRGBA {
	if len(g.stops) == 0 {
		return color.NRGBA{}
	}
	var t float64
	if g.radial {
		if g.r > 0 {
			t = math.Hypot(p.x-g.cx, p.y-g.cy) / g.r
		}
	} else {
		dx, dy := g.x2-g.x1, g.y2-g.y1
		if l := dx*dx + dy*dy; l > 0 {
			t = ((p.x-g.x1)*dx + (p.y-g.y1)*dy) / l
		}
	}
	if t <= g.stops[0].off {
		return g.stops[0].c
	}
	for i := 1; i < len(g.stops); i++ {
		a, b := g.stops[i-1], g.stops[i]
		if t <= b.off {
			f := (t - a.off) / math.Max(b.off-a.off, 1e-9)
			mix := func(p, q uint8) uint8 { return uint8(math.Round(float64(p) + (float64(q)-float64(p))*f)) }
			return color.NRGBA{mix(a.c.R, b.c.R), mix(a.c.G, b.c.G), mix(a.c.B, b.c.B), mix(a.c.A, b.c.A)}
		}
	}
	return g.stops[len(g.stops)-1].c
}

func parseGradient(n *node) *gradient {
	g := &gradient{radial: n.name == "radialGradient", bbox: n.attr["gradientUnits"] != "userSpaceOnUse", tf: ident}
	if v, ok := n.attr["gradientTransform"]; ok {
		g.tf = parseTransform(v)
	}
	if g.radial {
		g.cx, g.cy, g.r = n.num("cx", .5), n.num("cy", .5), n.num("r", .5)
	} else {
		g.x1, g.y1, g.x2, g.y2 = n.num("x1", 0), n.num("y1", 0), n.num("x2", 1), n.num("y2", 0)
	}
	last := 0.0
	for _, k := range n.kids {
		if k.name != "stop" {
			continue
		}
		c, ok := parseColor(k.attr["stop-color"])
		if !ok {
			c = color.NRGBA{0, 0, 0, 255}
		}
		c.A = uint8(math.Round(255 * math.Max(0, math.Min(1, k.num("stop-opacity", 1)))))
		off := math.Max(last, math.Max(0, math.Min(1, k.num("offset", 0))))
		last = off
		g.stops = append(g.stops, stop{off: off, c: c})
	}
	return g
}

type paint struct {
	solid color.NRGBA
	grad  *gradient
	inv   mat // artwork coordinates → the gradient's space
	none  bool
}

// clipShape is a clipPath's geometry in artwork coordinates; a point is inside when it is
// inside any part.
type clipShape struct{ parts []fillShape }

type fillShape struct {
	polys   [][]pt
	evenodd bool
}

type drawOp struct {
	polys         [][]pt // subpaths in artwork coordinates, flattened
	evenodd       bool
	fill, stroke  paint
	strokeW       float64 // in artwork units
	opacity       float64
	fillOpacity   float64
	strokeOpacity float64
	clips         []*clipShape
	blur          float64 // Gaussian standard deviation in artwork units; 0 for none
}

type artwork struct {
	vx, vy, vw, vh float64
	ops            []drawOp
}

// inherited is the drawing state passed down the tree.
type inherited struct {
	fill, stroke               string
	fillOpacity, strokeOpacity float64
	strokeW                    float64
	fillRule, clipRule         string
	opacity                    float64 // group opacity, folded into the shapes
	ctm                        mat
	clips                      []*clipShape
}

type builder struct {
	art     *artwork
	ids     map[string]*node
	grads   map[string]*gradient
	clipped map[string]*clipShape
}

func parseArtwork(src []byte) (*artwork, error) {
	root, err := parseTree(src)
	if err != nil {
		return nil, err
	}
	b := &builder{art: &artwork{vw: 1, vh: 1}, ids: map[string]*node{}, grads: map[string]*gradient{}, clipped: map[string]*clipShape{}}
	var index func(n *node)
	index = func(n *node) {
		if id := n.attr["id"]; id != "" {
			b.ids[id] = n
		}
		for _, k := range n.kids {
			index(k)
		}
	}
	index(root)
	for _, n := range root.kids {
		if n.name == "svg" {
			if f := numbers(n.attr["viewBox"]); len(f) == 4 {
				b.art.vx, b.art.vy, b.art.vw, b.art.vh = f[0], f[1], f[2], f[3]
			}
		}
	}
	b.walk(root, inherited{fill: "black", stroke: "none", fillOpacity: 1, strokeOpacity: 1, strokeW: 1, opacity: 1, ctm: ident})
	return b.art, nil
}

func (b *builder) gradient(id string) *gradient {
	if g, ok := b.grads[id]; ok {
		return g
	}
	var g *gradient
	if n := b.ids[id]; n != nil && (n.name == "linearGradient" || n.name == "radialGradient") {
		g = parseGradient(n)
	}
	b.grads[id] = g
	return g
}

func urlID(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "url(") {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(v, "url("), ")"), " "), "#")
}

// walk draws n and its children. Definitions (defs, clipPath, gradients, filters, …) are
// only used by reference.
func (b *builder) walk(n *node, st inherited) {
	switch n.name {
	case "defs", "clipPath", "mask", "filter", "linearGradient", "radialGradient", "pattern",
		"symbol", "marker", "title", "desc", "metadata", "style", "script":
		return
	}
	if n.attr["display"] == "none" || n.attr["visibility"] == "hidden" {
		return
	}
	for _, k := range []string{"fill", "stroke", "fill-rule", "clip-rule"} {
		if v, ok := n.attr[k]; ok && v != "inherit" {
			switch k {
			case "fill":
				st.fill = v
			case "stroke":
				st.stroke = v
			case "fill-rule":
				st.fillRule = v
			case "clip-rule":
				st.clipRule = v
			}
		}
	}
	if _, ok := n.attr["fill-opacity"]; ok {
		st.fillOpacity = n.num("fill-opacity", 1)
	}
	if _, ok := n.attr["stroke-opacity"]; ok {
		st.strokeOpacity = n.num("stroke-opacity", 1)
	}
	if _, ok := n.attr["stroke-width"]; ok {
		st.strokeW = n.num("stroke-width", 1)
	}
	if v, ok := n.attr["transform"]; ok {
		st.ctm = st.ctm.mul(parseTransform(v))
	}
	if id := urlID(n.attr["clip-path"]); id != "" {
		if c := b.clip(id, st.ctm); c != nil {
			st.clips = append(append([]*clipShape(nil), st.clips...), c)
		}
	}
	opacity := n.num("opacity", 1)
	switch n.name {
	case "#document", "svg", "g", "a", "switch":
		st.opacity *= opacity
		for _, k := range n.kids {
			b.walk(k, st)
		}
		return
	}
	local := shapeOf(n)
	if local == nil {
		return
	}
	op := drawOp{
		evenodd:       st.fillRule == "evenodd",
		opacity:       st.opacity * opacity,
		fillOpacity:   st.fillOpacity,
		strokeOpacity: st.strokeOpacity,
		strokeW:       st.strokeW * st.ctm.scale(),
		clips:         st.clips,
	}
	bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, pl := range local {
		q := make([]pt, len(pl))
		for i, p := range pl {
			bx0, by0, bx1, by1 = math.Min(bx0, p.x), math.Min(by0, p.y), math.Max(bx1, p.x), math.Max(by1, p.y)
			q[i] = st.ctm.apply(p)
		}
		op.polys = append(op.polys, q)
	}
	bbox := mat{bx1 - bx0, 0, 0, by1 - by0, bx0, by0}
	op.fill = b.paint(st.fill, st.ctm, bbox)
	op.stroke = b.paint(st.stroke, st.ctm, bbox)
	if id := urlID(n.attr["filter"]); id != "" {
		if f := b.ids[id]; f != nil {
			for _, k := range f.kids {
				if k.name == "feGaussianBlur" {
					if sd := numbers(k.attr["stdDeviation"]); len(sd) > 0 {
						op.blur = sd[0] * st.ctm.scale()
					}
					break
				}
			}
		}
	}
	b.art.ops = append(b.art.ops, op)
}

func (b *builder) paint(v string, ctm, bbox mat) paint {
	v = strings.TrimSpace(v)
	if v == "" || v == "none" || v == "transparent" {
		return paint{none: true}
	}
	if id := urlID(v); id != "" {
		g := b.gradient(id)
		if g == nil {
			return paint{none: true}
		}
		toArt := ctm
		if g.bbox {
			if bbox[0] == 0 || bbox[3] == 0 {
				return paint{none: true} // a bounding-box gradient on a flat shape draws nothing
			}
			toArt = toArt.mul(bbox)
		}
		return paint{grad: g, inv: toArt.mul(g.tf).inv()}
	}
	if c, ok := parseColor(v); ok {
		return paint{solid: c}
	}
	return paint{none: true}
}

// clip resolves a clipPath referenced from an element whose transform is ctm.
func (b *builder) clip(id string, ctm mat) *clipShape {
	key := id + strconv.FormatFloat(ctm[0], 'g', -1, 64) + "," + strconv.FormatFloat(ctm[1], 'g', -1, 64) + "," +
		strconv.FormatFloat(ctm[2], 'g', -1, 64) + "," + strconv.FormatFloat(ctm[3], 'g', -1, 64) + "," +
		strconv.FormatFloat(ctm[4], 'g', -1, 64) + "," + strconv.FormatFloat(ctm[5], 'g', -1, 64)
	if c, ok := b.clipped[key]; ok {
		return c
	}
	n := b.ids[id]
	if n == nil || n.name != "clipPath" {
		b.clipped[key] = nil
		return nil
	}
	c := &clipShape{}
	if v, ok := n.attr["transform"]; ok {
		ctm = ctm.mul(parseTransform(v))
	}
	var add func(k *node, m mat, rule string)
	add = func(k *node, m mat, rule string) {
		if v, ok := k.attr["transform"]; ok {
			m = m.mul(parseTransform(v))
		}
		if v, ok := k.attr["clip-rule"]; ok {
			rule = v
		}
		if k.name == "g" || k.name == "use" {
			for _, kk := range k.kids {
				add(kk, m, rule)
			}
			return
		}
		local := shapeOf(k)
		if local == nil {
			return
		}
		fs := fillShape{evenodd: rule == "evenodd"}
		for _, pl := range local {
			q := make([]pt, len(pl))
			for i, p := range pl {
				q[i] = m.apply(p)
			}
			fs.polys = append(fs.polys, q)
		}
		c.parts = append(c.parts, fs)
	}
	for _, k := range n.kids {
		add(k, ctm, n.attr["clip-rule"])
	}
	b.clipped[key] = c
	return c
}

// shapeOf is a shape element's outline in its own coordinates, flattened; nil for anything
// else.
func shapeOf(n *node) [][]pt {
	switch n.name {
	case "path":
		return flattenPath(n.attr["d"])
	case "circle":
		r := n.num("r", 0)
		if r <= 0 {
			return nil
		}
		return [][]pt{ellipse(n.num("cx", 0), n.num("cy", 0), r, r)}
	case "ellipse":
		rx, ry := n.num("rx", 0), n.num("ry", 0)
		if rx <= 0 || ry <= 0 {
			return nil
		}
		return [][]pt{ellipse(n.num("cx", 0), n.num("cy", 0), rx, ry)}
	case "rect":
		x, y, w, h := n.num("x", 0), n.num("y", 0), n.num("width", 0), n.num("height", 0)
		if w <= 0 || h <= 0 {
			return nil
		}
		rx, okx := n.attr["rx"]
		ry, oky := n.attr["ry"]
		if !okx {
			rx = ry
		}
		if !oky {
			ry = rx
		}
		return [][]pt{roundRect(x, y, w, h, math.Min(length(rx, 0), w/2), math.Min(length(ry, 0), h/2))}
	case "line":
		return [][]pt{{{n.num("x1", 0), n.num("y1", 0)}, {n.num("x2", 0), n.num("y2", 0)}}}
	case "polyline", "polygon":
		f := numbers(n.attr["points"])
		var p []pt
		for i := 0; i+1 < len(f); i += 2 {
			p = append(p, pt{f[i], f[i+1]})
		}
		if len(p) < 2 {
			return nil
		}
		if n.name == "polygon" {
			return [][]pt{append(p, p[0])}
		}
		return [][]pt{p}
	}
	return nil
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

func roundRect(x, y, w, h, rx, ry float64) []pt {
	if rx <= 0 || ry <= 0 {
		return []pt{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}, {x, y}}
	}
	const n = 8
	var p []pt
	corner := func(cx, cy, a0 float64) {
		for i := 0; i <= n; i++ {
			a := a0 + math.Pi/2*float64(i)/n
			p = append(p, pt{cx + rx*math.Cos(a), cy + ry*math.Sin(a)})
		}
	}
	corner(x+w-rx, y+ry, -math.Pi/2)
	corner(x+w-rx, y+h-ry, 0)
	corner(x+rx, y+h-ry, math.Pi/2)
	corner(x+rx, y+ry, math.Pi)
	return append(p, p[0])
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

// flattenPath turns path data into polylines, one per subpath; z adds the closing point.
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

// render draws the artwork's region (bx, by, bw, bh) into an image of size×size. It
// rasterises at 4× and averages down.
func (art *artwork) render(size int, bx, by, bw, bh float64) *image.NRGBA {
	const ss = 4
	n := size * ss
	acc := make([]float64, n*n*4) // premultiplied RGBA
	scale := float64(n) / math.Max(bw, bh)
	toPx := func(p pt) pt { return pt{(p.x - bx) * scale, (p.y - by) * scale} }
	cov := make([]float32, n*n)
	tmp := make([]float32, n*n)
	masks := map[*clipShape][]float32{}
	maskOf := func(c *clipShape) []float32 {
		if m, ok := masks[c]; ok {
			return m
		}
		m := make([]float32, n*n)
		for _, part := range c.parts {
			y0, y1 := fillMask(tmp, n, pxPolys(part.polys, toPx), part.evenodd)
			for i := y0 * n; i < (y1+1)*n; i++ {
				m[i] = max(m[i], tmp[i])
			}
		}
		masks[c] = m
		return m
	}
	draw := func(op *drawOp, polys [][]pt, evenodd bool, p paint, alpha float64) {
		y0, y1 := fillMask(cov, n, polys, evenodd)
		if y0 > y1 {
			return
		}
		if op.blur > 0 {
			y0, y1 = blurMask(cov, tmp, n, y0, y1, op.blur*scale)
		}
		for _, c := range op.clips {
			m := maskOf(c)
			for i := y0 * n; i < (y1+1)*n; i++ {
				cov[i] *= m[i]
			}
		}
		blend(acc, cov, n, y0, y1, p, alpha, scale, bx, by)
	}
	for i := range art.ops {
		op := &art.ops[i]
		if !op.fill.none && len(op.polys) > 0 {
			draw(op, pxPolys(op.polys, toPx), op.evenodd, op.fill, op.opacity*op.fillOpacity)
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
			draw(op, polys, false, op.stroke, op.opacity*op.strokeOpacity)
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

func pxPolys(polys [][]pt, toPx func(pt) pt) [][]pt {
	out := make([][]pt, len(polys))
	for j, pl := range polys {
		q := make([]pt, len(pl))
		for i, p := range pl {
			q[i] = toPx(p)
		}
		out[j] = q
	}
	return out
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

// fillMask sets cov to 1 inside the polygons (each one closed; nonzero or evenodd, sampled
// at pixel centres) and 0 elsewhere, over the rows y0..y1 it returns; other rows are left
// as they were. y0 > y1 when nothing is in view.
func fillMask(cov []float32, n int, polys [][]pt, evenodd bool) (int, int) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range polys {
		for _, q := range p {
			minY, maxY = math.Min(minY, q.y), math.Max(maxY, q.y)
		}
	}
	if len(polys) == 0 || maxY < 0 || minY > float64(n) {
		return 0, -1
	}
	y0, y1 := max(0, int(math.Floor(minY))), min(n-1, int(math.Ceil(maxY)))
	clear(cov[y0*n : (y1+1)*n])
	type cross struct {
		x float64
		w int
	}
	var xs []cross
	for y := y0; y <= y1; y++ {
		sy := float64(y) + 0.5
		xs = xs[:0]
		for _, p := range polys {
			for i := range p {
				a, b := p[i], p[(i+1)%len(p)] // the last point joins the first
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
			if evenodd {
				wind ^= 1
			} else {
				wind += xs[i].w
			}
			if wind == 0 {
				continue
			}
			xa, xb := max(0, int(math.Ceil(xs[i].x-0.5))), min(n-1, int(math.Floor(xs[i+1].x-0.5)))
			for x := xa; x <= xb; x++ {
				cov[y*n+x] = 1
			}
		}
	}
	return y0, y1
}

// blurMask blurs the coverage in rows y0..y1 with a Gaussian of standard deviation sd
// pixels (three box blurs), and returns the rows it now covers.
func blurMask(cov, tmp []float32, n, y0, y1 int, sd float64) (int, int) {
	r := int(math.Round(sd))
	if r < 1 {
		return y0, y1
	}
	Y0, Y1 := max(0, y0-3*r), min(n-1, y1+3*r)
	clear(cov[Y0*n : y0*n])
	clear(cov[(y1+1)*n : (Y1+1)*n])
	box := func(src, dst []float32, start, stride, count int) {
		var s float32
		k := float32(2*r + 1)
		at := func(i int) float32 {
			if i < 0 || i >= count {
				return 0
			}
			return src[start+i*stride]
		}
		for i := -r; i <= r; i++ {
			s += at(i)
		}
		for i := 0; i < count; i++ {
			dst[start+i*stride] = s / k
			s += at(i+r+1) - at(i-r)
		}
	}
	for pass := 0; pass < 3; pass++ {
		for y := Y0; y <= Y1; y++ {
			box(cov, tmp, y*n, 1, n)
		}
		copy(cov[Y0*n:(Y1+1)*n], tmp[Y0*n:(Y1+1)*n])
	}
	for pass := 0; pass < 3; pass++ {
		for x := 0; x < n; x++ {
			box(cov, tmp, Y0*n+x, n, Y1-Y0+1)
		}
		copy(cov[Y0*n:(Y1+1)*n], tmp[Y0*n:(Y1+1)*n])
	}
	return Y0, Y1
}

// blend composites paint through the coverage in rows y0..y1 onto acc (source-over,
// premultiplied).
func blend(acc []float64, cov []float32, n, y0, y1 int, p paint, opacity, scale, bx, by float64) {
	for i := y0 * n; i < (y1+1)*n; i++ {
		c := cov[i]
		if c == 0 {
			continue
		}
		col := p.solid
		if p.grad != nil {
			x, y := float64(i%n)+0.5, float64(i/n)+0.5
			col = p.grad.at(p.inv.apply(pt{x/scale + bx, y/scale + by}))
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
