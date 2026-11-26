package xdoc

import (
	"errors"

	"github.com/antchfx/xpath"
)

// Ref is a selected node: an element/text/document node, or attribute A of element N.
type Ref struct {
	N int32
	A int32 // attribute index, -1 for the node itself
}

// IsAttr reports whether the reference is an attribute.
func (r Ref) IsAttr() bool { return r.A >= 0 }

// Nav implements xpath.NodeNavigator over a Doc.
type Nav struct {
	d    *Doc
	cur  int32
	attr int32
}

// NewNav returns a navigator positioned at node i.
func (d *Doc) NewNav(i int32) *Nav { return &Nav{d: d, cur: i, attr: -1} }

// Ref is the navigator's current position.
func (n *Nav) Ref() Ref { return Ref{N: n.cur, A: n.attr} }

func (n *Nav) NodeType() xpath.NodeType {
	if n.attr >= 0 {
		return xpath.AttributeNode
	}
	switch n.d.at(n.cur).kind {
	case KindDocument:
		return xpath.RootNode
	case KindText:
		return xpath.TextNode
	}
	return xpath.ElementNode
}

func (n *Nav) LocalName() string {
	if n.attr >= 0 {
		return n.d.names.strs[n.d.attrs[n.d.at(n.cur).attrs][n.attr].Name]
	}
	return n.d.Name(n.cur)
}

func (n *Nav) Prefix() string { return "" }

func (n *Nav) NamespaceURL() string { return "" }

func (n *Nav) Value() string {
	if n.attr >= 0 {
		return n.d.attrs[n.d.at(n.cur).attrs][n.attr].Value
	}
	return n.d.InnerText(n.cur)
}

func (n *Nav) Copy() xpath.NodeNavigator { c := *n; return &c }

func (n *Nav) MoveToRoot() { n.cur, n.attr = 0, -1 }

func (n *Nav) MoveToParent() bool {
	n.d.step()
	if n.attr >= 0 {
		n.attr = -1
		return true
	}
	p := n.d.at(n.cur).parent
	if p == None {
		return false
	}
	n.cur = p
	return true
}

func (n *Nav) MoveToNextAttribute() bool {
	n.d.step()
	nd := n.d.at(n.cur)
	if nd.kind != KindElement || nd.attrs == 0 {
		return false
	}
	if int(n.attr+1) >= len(n.d.attrs[nd.attrs]) {
		return false
	}
	n.attr++
	return true
}

func (n *Nav) MoveToChild() bool {
	n.d.step()
	if n.attr >= 0 {
		return false
	}
	f := n.d.at(n.cur).first
	if f == None {
		return false
	}
	n.cur = f
	return true
}

func (n *Nav) MoveToFirst() bool {
	n.d.step()
	if n.attr >= 0 {
		return false
	}
	p := n.d.at(n.cur).parent
	if p == None {
		return false
	}
	f := n.d.at(p).first
	if f == n.cur {
		return false
	}
	n.cur = f
	return true
}

func (n *Nav) MoveToNext() bool {
	n.d.step()
	if n.attr >= 0 {
		return false
	}
	nx := n.d.at(n.cur).next
	if nx == None {
		return false
	}
	n.cur = nx
	return true
}

func (n *Nav) MoveToPrevious() bool {
	n.d.step()
	if n.attr >= 0 {
		return false
	}
	pv := n.d.at(n.cur).prev
	if pv == None {
		return false
	}
	n.cur = pv
	return true
}

func (n *Nav) MoveTo(other xpath.NodeNavigator) bool {
	o, ok := other.(*Nav)
	if !ok || o.d != n.d {
		return false
	}
	n.cur, n.attr = o.cur, o.attr
	return true
}

// ErrSteps: an evaluation made more navigator moves than SelectLimit allowed.
var ErrSteps = errors.New("xpath evaluation over its step budget")

// ErrEngine: the engine panicked during an evaluation.
var ErrEngine = errors.New("xpath engine error")

type overBudget struct{}

// step counts one navigator move against the running evaluation's budget. A count, not a
// clock, bounds an evaluation, so the same document and expression stop at the same place
// on every machine and at every priority (05 §5.2).
func (d *Doc) step() {
	d.steps++
	if d.limit > 0 && d.steps > d.limit {
		panic(overBudget{})
	}
}

// Select evaluates a compiled expression with node i as the context and returns the
// selected nodes in document order (xpath.Expr.Select semantics). A panic inside the
// engine is returned as ok=false.
func (d *Doc) Select(e *xpath.Expr, i int32, one bool) (out []Ref, ok bool) {
	out, err := d.SelectLimit(e, i, one, 0)
	return out, err == nil
}

// SelectLimit is Select with a budget of navigator moves (0: none). Past the budget the
// evaluation stops with ErrSteps; a panic inside the engine is ErrEngine.
func (d *Doc) SelectLimit(e *xpath.Expr, i int32, one bool, limit int64) (out []Ref, err error) {
	d.steps, d.limit = 0, limit
	defer func() {
		d.limit = 0
		if r := recover(); r != nil {
			out, err = nil, ErrEngine
			if _, over := r.(overBudget); over {
				err = ErrSteps
			}
		}
	}()
	it := e.Select(d.NewNav(i))
	for it.MoveNext() {
		out = append(out, it.Current().(*Nav).Ref())
		if one {
			break
		}
	}
	return out, nil
}

// Steps is how many navigator moves the last SelectLimit made.
func (d *Doc) Steps() int64 { return d.steps }
