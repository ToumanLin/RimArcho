// Package xdoc is a compact, mutable XML node store for the Patch simulation (07 §6.1
// item 2): nodes live in chunked arenas linked by int32 indices, element and attribute
// names are interned, and text is appended to fixed-size pages. It implements
// xpath.NodeNavigator, so antchfx/xpath evaluates expressions over it directly.
//
// Parsing follows the game's LoadableXmlAsset: comments, processing instructions and
// whitespace-only text are dropped.
package xdoc

import (
	"unsafe"
)

// Kind of a node.
type Kind uint8

const (
	KindDocument Kind = iota
	KindElement
	KindText
)

// None is the null node index.
const None int32 = -1

type node struct {
	parent, first, last, next, prev int32
	name                            int32 // interned element name (elements only)
	textOff, textLen                uint32
	attrs                           int32 // index into Doc.attrs; 0 = none
	kind                            Kind
}

// Attr is one attribute.
type Attr struct {
	Name  int32
	Value string
}

const chunkBits = 16
const chunkSize = 1 << chunkBits

// Doc is one XML document (or several merged under one root).
type Doc struct {
	chunks [][]node
	n      int32
	pages  [][]byte // text pages, append-only; strings never span pages
	big    []string // texts too large for a page
	tbytes int
	names  *Names
	attrs  [][]Attr // attrs[0] is unused

	// Lines maps element index → source line, when the document was built with lines.
	Lines map[int32]int32
	// OnTouch, if set, is called before p's child list or n's attributes change (index
	// invalidation); attr is the attribute name for attribute changes, else -1.
	OnTouch func(p, n int32, attr int32)
}

// Names interns element and attribute names; documents that exchange nodes share one.
type Names struct {
	ids  map[string]int32
	strs []string
}

// NewNames returns an empty name table.
func NewNames() *Names { return &Names{ids: map[string]int32{}} }

// ID interns s.
func (t *Names) ID(s string) int32 {
	if id, ok := t.ids[s]; ok {
		return id
	}
	id := int32(len(t.strs))
	t.strs = append(t.strs, s)
	t.ids[s] = id
	return id
}

// Lookup returns s's ID without interning it (-1 if unknown).
func (t *Names) Lookup(s string) int32 {
	if id, ok := t.ids[s]; ok {
		return id
	}
	return -1
}

// Str returns the name with the given ID.
func (t *Names) Str(id int32) string {
	if id < 0 || int(id) >= len(t.strs) {
		return ""
	}
	return t.strs[id]
}

// New returns a document holding only the document node (index 0).
func New(names *Names) *Doc {
	if names == nil {
		names = NewNames()
	}
	d := &Doc{names: names, attrs: make([][]Attr, 1)}
	d.alloc(KindDocument)
	return d
}

// Names returns the document's name table.
func (d *Doc) Names() *Names { return d.names }

// Len is the number of nodes ever allocated (including detached ones).
func (d *Doc) Len() int { return int(d.n) }

// TextBytes is the size of the text buffer.
func (d *Doc) TextBytes() int { return d.tbytes }

const (
	pageBits = 20
	pageSize = 1 << pageBits
	bigFlag  = 1 << 31
)

// putText stores s and returns its offset code. Pages are never reallocated, so strings
// returned by Text stay valid and keep only their own page alive.
func (d *Doc) putText(s string) uint32 {
	d.tbytes += len(s)
	if len(s) >= pageSize/16 {
		d.big = append(d.big, s)
		return uint32(len(d.big)-1) | bigFlag
	}
	if len(d.pages) == 0 || len(d.pages[len(d.pages)-1])+len(s) > cap(d.pages[len(d.pages)-1]) {
		size := pageSize
		if len(d.pages) == 0 {
			size = 4096 // small documents stay small
		} else if c := cap(d.pages[len(d.pages)-1]); c < pageSize {
			size = min(c*4, pageSize)
		}
		d.pages = append(d.pages, make([]byte, 0, size))
	}
	pg := len(d.pages) - 1
	off := len(d.pages[pg])
	d.pages[pg] = append(d.pages[pg], s...)
	return uint32(pg)<<pageBits | uint32(off)
}

func (d *Doc) at(i int32) *node { return &d.chunks[i>>chunkBits][i&(chunkSize-1)] }

// alloc appends a node. The first chunk grows like a slice (small documents stay small);
// later chunks are allocated at full size. Pointers from at() are invalid after alloc.
func (d *Doc) alloc(k Kind) int32 {
	i := d.n
	nd := node{parent: None, first: None, last: None, next: None, prev: None, name: -1, kind: k}
	c := int(i >> chunkBits)
	switch {
	case c == len(d.chunks) && c == 0:
		d.chunks = append(d.chunks, make([]node, 0, 16))
		fallthrough
	case c == 0:
		d.chunks[0] = append(d.chunks[0], nd)
	case c == len(d.chunks):
		d.chunks = append(d.chunks, make([]node, chunkSize))
		fallthrough
	default:
		d.chunks[c][i&(chunkSize-1)] = nd
	}
	d.n++
	return i
}

// NewElement allocates a detached element.
func (d *Doc) NewElement(name string) int32 {
	i := d.alloc(KindElement)
	d.at(i).name = d.names.ID(name)
	return i
}

// NewText allocates a detached text node.
func (d *Doc) NewText(s string) int32 {
	i := d.alloc(KindText)
	n := d.at(i)
	n.textOff, n.textLen = d.putText(s), uint32(len(s))
	return i
}

// Root is the document node.
func (d *Doc) Root() int32 { return 0 }

// Kind of node i.
func (d *Doc) Kind(i int32) Kind { return d.at(i).kind }

// Parent of node i (None for the document node or a detached node).
func (d *Doc) Parent(i int32) int32 { return d.at(i).parent }

// FirstChild of node i.
func (d *Doc) FirstChild(i int32) int32 { return d.at(i).first }

// LastChild of node i.
func (d *Doc) LastChild(i int32) int32 { return d.at(i).last }

// Next sibling of node i.
func (d *Doc) Next(i int32) int32 { return d.at(i).next }

// Prev sibling of node i.
func (d *Doc) Prev(i int32) int32 { return d.at(i).prev }

// NameID of element i.
func (d *Doc) NameID(i int32) int32 { return d.at(i).name }

// Name of element i ("" for other nodes).
func (d *Doc) Name(i int32) string {
	n := d.at(i)
	if n.kind != KindElement {
		return ""
	}
	return d.names.strs[n.name]
}

// Text of text node i. The string aliases the text buffer, which is append-only.
func (d *Doc) Text(i int32) string {
	n := d.at(i)
	if n.textLen == 0 {
		return ""
	}
	if n.textOff&bigFlag != 0 {
		return d.big[n.textOff&^bigFlag]
	}
	pg := d.pages[n.textOff>>pageBits]
	return unsafe.String(&pg[n.textOff&(pageSize-1)], int(n.textLen))
}

// InnerText concatenates the text of i's descendants (XmlNode.InnerText).
func (d *Doc) InnerText(i int32) string {
	n := d.at(i)
	if n.kind == KindText {
		return d.Text(i)
	}
	if n.first != None && n.first == n.last && d.at(n.first).kind == KindText {
		return d.Text(n.first) // the common case: <defName>X</defName>
	}
	var b []byte
	d.appendText(&b, i)
	return string(b)
}

func (d *Doc) appendText(b *[]byte, i int32) {
	for c := d.at(i).first; c != None; c = d.at(c).next {
		cn := d.at(c)
		if cn.kind == KindText {
			*b = append(*b, d.Text(c)...)
		} else {
			d.appendText(b, c)
		}
	}
}

// Attrs of element i (shared; do not modify).
func (d *Doc) Attrs(i int32) []Attr {
	a := d.at(i).attrs
	if a == 0 {
		return nil
	}
	return d.attrs[a]
}

// Attr returns the value of attribute name on element i.
func (d *Doc) Attr(i int32, name string) (string, bool) {
	id := d.names.Lookup(name)
	if id < 0 {
		return "", false
	}
	for _, a := range d.Attrs(i) {
		if a.Name == id {
			return a.Value, true
		}
	}
	return "", false
}

// SetAttr sets (or adds) an attribute on element i.
func (d *Doc) SetAttr(i int32, name, value string) {
	id := d.names.ID(name)
	d.touch(d.at(i).parent, i, id)
	n := d.at(i)
	if n.attrs == 0 {
		d.attrs = append(d.attrs, nil)
		n.attrs = int32(len(d.attrs) - 1)
	}
	l := d.attrs[n.attrs]
	for k := range l {
		if l[k].Name == id {
			l[k].Value = value
			return
		}
	}
	d.attrs[n.attrs] = append(l, Attr{Name: id, Value: value})
}

// RemoveAttr removes an attribute; it reports whether it existed.
func (d *Doc) RemoveAttr(i int32, name string) bool {
	id := d.names.Lookup(name)
	n := d.at(i)
	if id < 0 || n.attrs == 0 {
		return false
	}
	l := d.attrs[n.attrs]
	for k := range l {
		if l[k].Name == id {
			d.touch(n.parent, i, id)
			d.attrs[n.attrs] = append(l[:k:k], l[k+1:]...)
			return true
		}
	}
	return false
}

func (d *Doc) touch(p, n, attr int32) {
	if d.OnTouch != nil {
		d.OnTouch(p, n, attr)
	}
}

// Detach unlinks node i from its parent.
func (d *Doc) Detach(i int32) {
	n := d.at(i)
	if n.parent == None {
		return
	}
	d.touch(n.parent, i, -1)
	p := d.at(n.parent)
	if n.prev != None {
		d.at(n.prev).next = n.next
	} else {
		p.first = n.next
	}
	if n.next != None {
		d.at(n.next).prev = n.prev
	} else {
		p.last = n.prev
	}
	n.parent, n.prev, n.next = None, None, None
}

// AppendChild makes detached node c the last child of p.
func (d *Doc) AppendChild(p, c int32) {
	d.touch(p, c, -1)
	pn, cn := d.at(p), d.at(c)
	cn.parent, cn.next, cn.prev = p, None, pn.last
	if pn.last != None {
		d.at(pn.last).next = c
	} else {
		pn.first = c
	}
	pn.last = c
}

// PrependChild makes detached node c the first child of p.
func (d *Doc) PrependChild(p, c int32) {
	if f := d.at(p).first; f != None {
		d.InsertBefore(f, c)
		return
	}
	d.AppendChild(p, c)
}

// InsertBefore inserts detached node c before ref.
func (d *Doc) InsertBefore(ref, c int32) {
	rn := d.at(ref)
	p := rn.parent
	d.touch(p, c, -1)
	cn := d.at(c)
	cn.parent, cn.next, cn.prev = p, ref, rn.prev
	if rn.prev != None {
		d.at(rn.prev).next = c
	} else {
		d.at(p).first = c
	}
	rn.prev = c
}

// InsertAfter inserts detached node c after ref.
func (d *Doc) InsertAfter(ref, c int32) {
	if nx := d.at(ref).next; nx != None {
		d.InsertBefore(nx, c)
		return
	}
	d.AppendChild(d.at(ref).parent, c)
}

// Clone deep-copies node i of src into d (detached) and returns the copy.
func (d *Doc) Clone(src *Doc, i int32) int32 {
	sn := *src.at(i) // a copy: allocation below may move src's nodes when src == d
	var c int32
	switch sn.kind {
	case KindText:
		c = d.NewText(src.Text(i))
	default:
		c = d.alloc(sn.kind)
		name := sn.name
		if src.names != d.names && name >= 0 {
			name = d.names.ID(src.names.strs[name])
		}
		d.at(c).name = name
		if sn.attrs != 0 {
			l := make([]Attr, len(src.attrs[sn.attrs]))
			copy(l, src.attrs[sn.attrs])
			if src.names != d.names {
				for k := range l {
					l[k].Name = d.names.ID(src.names.strs[l[k].Name])
				}
			}
			d.attrs = append(d.attrs, l)
			d.at(c).attrs = int32(len(d.attrs) - 1)
		}
		for ch := sn.first; ch != None; ch = src.at(ch).next {
			cc := d.Clone(src, ch)
			// Link directly: the copy is detached, so no index can see it yet.
			cn, ccn := d.at(c), d.at(cc)
			ccn.parent, ccn.prev = c, cn.last
			if cn.last != None {
				d.at(cn.last).next = cc
			} else {
				cn.first = cc
			}
			cn.last = cc
		}
	}
	return c
}

// Normalize merges adjacent text children of p (XmlNode.Normalize).
func (d *Doc) Normalize(p int32) {
	for c := d.at(p).first; c != None; {
		cn := d.at(c)
		nx := cn.next
		if nx == None {
			return
		}
		if cn.kind == KindText && d.at(nx).kind == KindText {
			s := d.Text(c) + d.Text(nx)
			off := d.putText(s)
			cn = d.at(c)
			cn.textOff, cn.textLen = off, uint32(len(s))
			d.Detach(nx)
			continue
		}
		c = nx
	}
}

// Children returns the element children of i.
func (d *Doc) Children(i int32) []int32 {
	var out []int32
	for c := d.at(i).first; c != None; c = d.at(c).next {
		if d.at(c).kind == KindElement {
			out = append(out, c)
		}
	}
	return out
}

// ChildNodes returns all children of i.
func (d *Doc) ChildNodes(i int32) []int32 {
	var out []int32
	for c := d.at(i).first; c != None; c = d.at(c).next {
		out = append(out, c)
	}
	return out
}

// Child returns the first element child of i named name.
func (d *Doc) Child(i int32, name string) int32 {
	id := d.names.Lookup(name)
	if id < 0 {
		return None
	}
	for c := d.at(i).first; c != None; c = d.at(c).next {
		if cn := d.at(c); cn.kind == KindElement && cn.name == id {
			return c
		}
	}
	return None
}

// DocElement returns the first element child of the document node.
func (d *Doc) DocElement() int32 {
	for c := d.at(0).first; c != None; c = d.at(c).next {
		if d.at(c).kind == KindElement {
			return c
		}
	}
	return None
}

// Line returns the source line of element i (0 if unknown).
func (d *Doc) Line(i int32) int { return int(d.Lines[i]) }
