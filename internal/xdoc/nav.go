package xdoc

import (
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

// Select evaluates a compiled expression with node i as the context and returns the
// selected nodes in document order (xpath.Expr.Select semantics). A panic inside the
// engine is returned as ok=false.
func (d *Doc) Select(e *xpath.Expr, i int32, one bool) (out []Ref, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			out, ok = nil, false
		}
	}()
	it := e.Select(d.NewNav(i))
	for it.MoveNext() {
		out = append(out, it.Current().(*Nav).Ref())
		if one {
			break
		}
	}
	return out, true
}
