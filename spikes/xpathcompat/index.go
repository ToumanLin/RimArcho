package main

import (
	"flag"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
)

// Fast path: most patch XPaths start with Defs/<Type>[defName="X"] or [@Name="X"].
// Instead of scanning every top-level def, look candidates up in an index of
// top-level nodes by defName text / Name attribute, then evaluate the remainder of the
// expression with each candidate as the context node. The index is rebuilt lazily
// whenever a mutation touches the top two levels of the document.

var useIndex = flag.Bool("index", true, "use the defName/@Name index fast path")

var (
	indexRoot  *xmlquery.Node
	indexDirty = true
)

// touch marks the index dirty when inserting/removing n under p can change the key set:
// a top-level node itself, a defName child of a top-level def, or text inside such a defName.
func touch(p, n *xmlquery.Node) {
	if indexRoot == nil || p == nil {
		return
	}
	switch {
	case p == indexRoot:
		indexDirty = true
	case p.Parent == indexRoot && n != nil && n.Type == xmlquery.ElementNode && n.Data == "defName":
		indexDirty = true
	case p.Data == "defName" && p.Parent != nil && p.Parent.Parent == indexRoot:
		indexDirty = true
	}
}

// touchAttr: only the Name attribute of a top-level def is indexed.
func touchAttr(n *xmlquery.Node, attr string) {
	if n.Parent == indexRoot && attr == "Name" {
		indexDirty = true
	}
}

type defIndex struct {
	byDefName map[string][]*xmlquery.Node
	byName    map[string][]*xmlquery.Node
	rebuilds  int
	hits      int
}

func (ix *defIndex) ensure() {
	if !indexDirty && ix.byDefName != nil {
		return
	}
	ix.rebuilds++
	ix.byDefName = make(map[string][]*xmlquery.Node, 40000)
	ix.byName = make(map[string][]*xmlquery.Node, 8000)
	for d := indexRoot.FirstChild; d != nil; d = d.NextSibling {
		if d.Type != xmlquery.ElementNode {
			continue
		}
		for _, a := range d.Attr {
			if a.Name.Local == "Name" && a.Name.Space == "" {
				ix.byName[a.Value] = append(ix.byName[a.Value], d)
			}
		}
		for c := d.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xmlquery.ElementNode && c.Data == "defName" {
				ix.byDefName[c.InnerText()] = append(ix.byDefName[c.InnerText()], d)
			}
		}
	}
	indexDirty = false
}

type fastPlan struct {
	elem  string // element name or "*"
	attr  bool   // @Name instead of defName
	value string
	rest  *xpath.Expr // nil = the candidate itself
}

var fastRe = regexp.MustCompile(`^/?Defs/([A-Za-z_][\w.\-]*|\*)\[\s*(defName|@Name)\s*=\s*(?:"([^"]*)"|'([^']*)')\s*\](.*)$`)

func (s *Sim) plan(expr string) *fastPlan {
	if p, ok := s.plans[expr]; ok {
		return p
	}
	var p *fastPlan
	if m := fastRe.FindStringSubmatch(strings.TrimSpace(expr)); m != nil {
		rest := strings.TrimSpace(m[5])
		// Absolute references or unions in the remainder need the whole document.
		if !strings.Contains(rest, "/Defs") && !strings.Contains(rest, "..") && !strings.Contains(rest, "ancestor") &&
			!strings.Contains(rest, "preceding") && !strings.Contains(rest, "following") && !strings.Contains(rest, "|") && !strings.HasPrefix(rest, "//") {
			p = &fastPlan{elem: m[1], attr: m[2] == "@Name", value: m[3] + m[4]}
			if rest != "" {
				var src string
				switch rest[0] {
				case '/':
					src = "." + rest
				case '[':
					src = "self::node()" + rest
				}
				if src != "" {
					if e, err := xpath.Compile(src); err == nil {
						p.rest = e
					} else {
						p = nil
					}
				} else {
					p = nil
				}
			}
		}
	}
	s.plans[expr] = p
	return p
}

func (s *Sim) fastSelect(p *fastPlan, one bool) []*xmlquery.Node {
	s.ix.ensure()
	s.ix.hits++
	cands := s.ix.byDefName[p.value]
	if p.attr {
		cands = s.ix.byName[p.value]
	}
	var out []*xmlquery.Node
	for _, c := range cands {
		if p.elem != "*" && c.Data != p.elem {
			continue
		}
		if p.rest == nil {
			out = append(out, c)
		} else {
			out = append(out, xmlquery.QuerySelectorAll(c, p.rest)...)
		}
		if one && len(out) > 0 {
			return out[:1]
		}
	}
	return out
}
