package xdoc

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// Event is one parsed token. Parsing (the slow part) produces events and can run on many
// goroutines; building a document from them is sequential and cheap.
type Event struct {
	Kind  uint8 // EvStart, EvEnd, EvText
	Name  string
	Attrs []xml.Attr
	Text  string
	Line  int32
}

// Event kinds.
const (
	EvStart uint8 = iota
	EvEnd
	EvText
)

// Parse tokenises an XML file like LoadableXmlAsset (IgnoreComments, IgnoreWhitespace).
func Parse(b []byte) ([]Event, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	evs := make([]Event, 0, len(b)/64)
	depth := 0
	var text strings.Builder
	textLine := int32(0)
	flush := func() {
		if text.Len() == 0 {
			return
		}
		s := text.String()
		text.Reset()
		if strings.TrimSpace(s) == "" {
			return
		}
		evs = append(evs, Event{Kind: EvText, Text: s, Line: textLine})
	}
	for {
		line, _ := dec.InputPos()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			flush()
			ev := Event{Kind: EvStart, Name: t.Name.Local, Line: int32(line)}
			if len(t.Attr) > 0 {
				ev.Attrs = make([]xml.Attr, 0, len(t.Attr))
				for _, a := range t.Attr {
					if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" && a.Name.Space == "" {
						continue
					}
					ev.Attrs = append(ev.Attrs, a)
				}
			}
			evs = append(evs, ev)
			depth++
		case xml.EndElement:
			flush()
			evs = append(evs, Event{Kind: EvEnd})
			depth--
		case xml.CharData:
			if depth > 0 {
				if text.Len() == 0 {
					textLine = int32(line)
				}
				text.Write(t)
			}
		}
	}
	return evs, nil
}

// Build appends the events' elements to parent. With skipRoot, the file's root element is
// not created and its children are appended instead (merging <Defs> files). It returns the
// elements appended directly under parent and their source lines.
func (d *Doc) Build(parent int32, evs []Event, skipRoot, keepLines bool) (tops []int32, lines []int32) {
	if keepLines && d.Lines == nil {
		d.Lines = map[int32]int32{}
	}
	stack := []int32{parent}
	depth := 0
	for _, ev := range evs {
		switch ev.Kind {
		case EvStart:
			depth++
			if skipRoot && depth == 1 {
				continue
			}
			e := d.NewElement(ev.Name)
			for _, a := range ev.Attrs {
				name := a.Name.Local
				if a.Name.Space != "" {
					name = a.Name.Space + ":" + name
				}
				d.setAttrRaw(e, name, a.Value)
			}
			top := stack[len(stack)-1]
			d.link(top, e)
			if top == parent {
				tops = append(tops, e)
				lines = append(lines, ev.Line)
			}
			if keepLines {
				d.Lines[e] = ev.Line
			}
			stack = append(stack, e)
		case EvEnd:
			if skipRoot && depth == 1 {
				depth--
				continue
			}
			depth--
			stack = stack[:len(stack)-1]
		case EvText:
			if skipRoot && depth <= 1 {
				continue
			}
			t := d.NewText(ev.Text)
			d.link(stack[len(stack)-1], t)
		}
	}
	return tops, lines
}

// link appends without the touch callback (building, not patching).
func (d *Doc) link(p, c int32) {
	pn, cn := d.at(p), d.at(c)
	cn.parent, cn.next, cn.prev = p, None, pn.last
	if pn.last != None {
		d.at(pn.last).next = c
	} else {
		pn.first = c
	}
	pn.last = c
}

func (d *Doc) setAttrRaw(i int32, name, value string) {
	id := d.names.ID(name)
	n := d.at(i)
	if n.attrs == 0 {
		d.attrs = append(d.attrs, nil)
		n.attrs = int32(len(d.attrs) - 1)
	}
	d.attrs[n.attrs] = append(d.attrs[n.attrs], Attr{Name: id, Value: value})
}

// ParseDoc parses a whole file into a new document (with lines).
func ParseDoc(b []byte, names *Names) (*Doc, error) {
	evs, err := Parse(b)
	if err != nil {
		return nil, err
	}
	d := New(names)
	d.Build(0, evs, false, true)
	return d, nil
}
