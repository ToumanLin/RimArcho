package modsearch

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strings"
)

type region struct {
	start, end   int
	within, role string
}

// regionScan holds the decoder state that contexts carries from token to token; the element stack stays local to contexts.
type regionScan struct {
	b                                   []byte
	kind                                Kind
	def                                 bool
	pattern                             string
	spans, roles                        []region
	depth, current                      int
	name, defName, xpath                strings.Builder
	captureDef, captureXPath, haveXPath bool
}

// Context uses byte offsets so two defs or operations on one line stay distinguishable.
// Non-strict decoding also tolerates undeclared entities and common malformed mod XML.
func contexts(ctx context.Context, b []byte, kind Kind, o Options, hits []Match, offsets []int) DefSummary {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	s := regionScan{b: b, kind: kind, def: o.Def, pattern: o.Pattern, current: -1}
	var stack []string
	for ctx.Err() == nil {
		start := int(dec.InputOffset())
		token, err := dec.Token()
		if err != nil {
			s.finish(len(b))
			break
		}
		end := int(dec.InputOffset())
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			s.startElement(&t, start, end)
		case xml.CharData:
			s.charData(t, stack, start, end)
		case xml.EndElement:
			s.endElement(&t, end)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return summarize(s.spans, s.roles, kind, &o, hits, offsets)
}

// finish closes the open def or operation span at end and records what it is within.
func (s *regionScan) finish(end int) {
	if s.current < 0 {
		return
	}
	span := &s.spans[s.current]
	span.end = end
	if s.kind == Defs {
		if value := strings.TrimSpace(s.defName.String()); value != "" {
			span.within += "/" + value
		} else if s.name.Len() > 0 {
			span.within += "@" + s.name.String()
		}
	} else {
		span.within = snippet(s.xpath.String(), 120)
	}
	s.current = -1
}

// startElement opens a span at a top-level def or operation and starts capturing its name.
func (s *regionScan) startElement(t *xml.StartElement, start, end int) {
	s.depth++
	if s.depth == 2 && (s.kind == Defs || t.Name.Local == "Operation") {
		s.openSpan(t, start)
	}
	if s.current >= 0 {
		if s.kind == Defs && s.depth == 3 && t.Name.Local == "defName" {
			s.captureDef = true
		}
		if s.kind == Patches && t.Name.Local == "xpath" && !s.haveXPath {
			s.captureXPath, s.haveXPath = true, true
		}
	}
	if s.kind == Defs && s.def {
		s.roles = append(s.roles, attributeRoles(s.b[start:end], start, s.pattern)...)
	}
}

// openSpan starts a new span at start and resets the captured name, defName and xpath.
func (s *regionScan) openSpan(t *xml.StartElement, start int) {
	s.current = len(s.spans)
	s.spans = append(s.spans, region{start: start, end: len(s.b), within: t.Name.Local})
	s.name.Reset()
	s.defName.Reset()
	s.xpath.Reset()
	s.captureDef, s.captureXPath, s.haveXPath = false, false, false
	for _, a := range t.Attr {
		if a.Name.Local == "Name" {
			s.name.WriteString(a.Value)
		}
	}
}

// charData captures defName and xpath text and marks a defName equal to the pattern as a def.
func (s *regionScan) charData(t xml.CharData, stack []string, start, end int) {
	if s.captureDef {
		s.defName.Write(t)
	}
	if s.captureXPath {
		s.xpath.Write(t)
	}
	if s.kind == Defs && s.def && len(stack) > 0 && stack[len(stack)-1] == "defName" && strings.TrimSpace(string(t)) == s.pattern {
		s.roles = append(s.roles, region{start: start, end: end, role: "def"})
	}
}

// endElement stops captures and closes the span when a top-level element ends.
func (s *regionScan) endElement(t *xml.EndElement, end int) {
	if t.Name.Local == "defName" {
		s.captureDef = false
	}
	if t.Name.Local == "xpath" {
		s.captureXPath = false
	}
	if s.depth == 2 {
		s.finish(end)
	}
	s.depth--
}

// defTally remembers which spans already put a patch, def, abstract or child in the summary.
type defTally struct {
	operations, defined, abstracts, children map[int]bool
}

// summarize assigns each hit its span and role and builds the def summary.
func summarize(spans, roles []region, kind Kind, o *Options, hits []Match, offsets []int) DefSummary {
	summary := DefSummary{}
	if o.Def {
		summary.Roles = map[string]int{}
	}
	tally := defTally{operations: map[int]bool{}, defined: map[int]bool{}, abstracts: map[int]bool{}, children: map[int]bool{}}
	si, ri := 0, 0
	for i, offset := range offsets {
		for si < len(spans) && spans[si].end <= offset {
			si++
		}
		inSpan := si < len(spans) && spans[si].start <= offset
		if inSpan {
			hits[i].Within = spans[si].within
		}
		if !o.Def {
			continue
		}
		hits[i].Role = "ref"
		if kind == Patches {
			tally.patchRole(&hits[i], &summary, inSpan, si, o.Pattern)
			continue
		}
		for ri < len(roles) && roles[ri].end <= offset {
			ri++
		}
		if ri < len(roles) && roles[ri].start <= offset {
			hits[i].Role = roles[ri].role
		}
		summary.Roles[hits[i].Role]++
		if inSpan {
			tally.listDef(&hits[i], &summary, si)
		}
	}
	summary.PatchOperations = len(tally.operations)
	return summary
}

// patchRole marks a Patches hit as patch when its operation's xpath names the def.
func (t *defTally) patchRole(hit *Match, summary *DefSummary, inSpan bool, si int, pattern string) {
	// Only an operation whose xpath names the def patches it; elsewhere it is a value.
	hit.Role = "patch-value"
	if inSpan && containsWord(hit.Within, pattern) {
		hit.Role = "patch"
		t.operations[si] = true
	}
	summary.Roles[hit.Role]++
}

// listDef adds the hit's span to Defined, Abstracts or Children once per span.
func (t *defTally) listDef(hit *Match, summary *DefSummary, si int) {
	switch hit.Role {
	case "def":
		if !t.defined[si] {
			summary.Defined = append(summary.Defined, hit.Within)
			t.defined[si] = true
		}
	case "abstract":
		if !t.abstracts[si] {
			summary.Abstracts = append(summary.Abstracts, hit.Within)
			t.abstracts[si] = true
		}
	case "child":
		if !t.children[si] {
			summary.Children = append(summary.Children, hit.Within)
			t.children[si] = true
		}
	}
}

// Locate attribute values in the original token, preserving columns across newlines and quotes.
func attributeRoles(tag []byte, base int, pattern string) []region {
	var out []region
	for i := 0; i < len(tag); {
		if tag[i] == '"' || tag[i] == '\'' {
			quote := tag[i]
			i++
			for i < len(tag) && tag[i] != quote {
				i++
			}
			if i < len(tag) {
				i++
			}
			continue
		}
		if !ident(tag[i]) {
			i++
			continue
		}
		start := i
		for i < len(tag) && ident(tag[i]) {
			i++
		}
		name := string(tag[start:i])
		for i < len(tag) && space(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			continue
		}
		i++
		for i < len(tag) && space(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '"' && tag[i] != '\'' {
			continue
		}
		quote := tag[i]
		i++
		value := i
		for i < len(tag) && tag[i] != quote {
			i++
		}
		if string(tag[value:i]) == pattern {
			role := ""
			if name == "Name" {
				role = "abstract"
			}
			if name == "ParentName" {
				role = "child"
			}
			if role != "" {
				out = append(out, region{start: base + value, end: base + i, role: role})
			}
		}
		if i < len(tag) {
			i++
		}
	}
	return out
}
func space(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// containsWord reports whether w occurs in s between identifier boundaries.
func containsWord(s, w string) bool {
	for at := 0; w != ""; {
		n := strings.Index(s[at:], w)
		if n < 0 {
			return false
		}
		if boundary([]byte(s), at+n, at+n+len(w)) {
			return true
		}
		at += n + 1
	}
	return false
}
