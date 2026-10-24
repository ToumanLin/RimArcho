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

// Context uses byte offsets so two defs or operations on one line stay distinguishable.
// Non-strict decoding also tolerates undeclared entities and common malformed mod XML.
func contexts(ctx context.Context, b []byte, kind Kind, o Options, hits []Match, offsets []int) DefSummary {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var spans, roles []region
	depth, current := 0, -1
	var stack []string
	var name, defName, xpath strings.Builder
	captureDef, captureXPath, haveXPath := false, false, false
	finish := func(end int) {
		if current < 0 {
			return
		}
		span := &spans[current]
		span.end = end
		if kind == Defs {
			if value := strings.TrimSpace(defName.String()); value != "" {
				span.within += "/" + value
			} else if name.Len() > 0 {
				span.within += "@" + name.String()
			}
		} else {
			span.within = snippet(xpath.String(), 120)
		}
		current = -1
	}
	for ctx.Err() == nil {
		start := int(dec.InputOffset())
		token, err := dec.Token()
		if err != nil {
			finish(len(b))
			break
		}
		end := int(dec.InputOffset())
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			stack = append(stack, t.Name.Local)
			if depth == 2 && (kind == Defs || t.Name.Local == "Operation") {
				current = len(spans)
				spans = append(spans, region{start: start, end: len(b), within: t.Name.Local})
				name.Reset()
				defName.Reset()
				xpath.Reset()
				captureDef, captureXPath, haveXPath = false, false, false
				for _, a := range t.Attr {
					if a.Name.Local == "Name" {
						name.WriteString(a.Value)
					}
				}
			}
			if current >= 0 {
				if kind == Defs && depth == 3 && t.Name.Local == "defName" {
					captureDef = true
				}
				if kind == Patches && t.Name.Local == "xpath" && !haveXPath {
					captureXPath, haveXPath = true, true
				}
			}
			if kind == Defs && o.Def {
				roles = append(roles, attributeRoles(b[start:end], start, o.Pattern)...)
			}
		case xml.CharData:
			if captureDef {
				defName.Write(t)
			}
			if captureXPath {
				xpath.Write(t)
			}
			if kind == Defs && o.Def && len(stack) > 0 && stack[len(stack)-1] == "defName" && strings.TrimSpace(string(t)) == o.Pattern {
				roles = append(roles, region{start: start, end: end, role: "def"})
			}
		case xml.EndElement:
			if t.Name.Local == "defName" {
				captureDef = false
			}
			if t.Name.Local == "xpath" {
				captureXPath = false
			}
			if depth == 2 {
				finish(end)
			}
			depth--
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	summary := DefSummary{}
	if o.Def {
		summary.Roles = map[string]int{}
	}
	operations := map[int]bool{}
	defined, abstracts, children := map[int]bool{}, map[int]bool{}, map[int]bool{}
	si, ri := 0, 0
	for i, offset := range offsets {
		for si < len(spans) && spans[si].end <= offset {
			si++
		}
		if si < len(spans) && spans[si].start <= offset {
			hits[i].Within = spans[si].within
		}
		if !o.Def {
			continue
		}
		hits[i].Role = "ref"
		if kind == Patches {
			// Only an operation whose xpath names the def patches it; elsewhere it is a value.
			hits[i].Role = "patch-value"
			if si < len(spans) && spans[si].start <= offset && containsWord(spans[si].within, o.Pattern) {
				hits[i].Role = "patch"
				operations[si] = true
			}
			summary.Roles[hits[i].Role]++
			continue
		}
		for ri < len(roles) && roles[ri].end <= offset {
			ri++
		}
		if ri < len(roles) && roles[ri].start <= offset {
			hits[i].Role = roles[ri].role
		}
		role := hits[i].Role
		summary.Roles[role]++
		if si < len(spans) && spans[si].start <= offset {
			switch role {
			case "def":
				if !defined[si] {
					summary.Defined = append(summary.Defined, hits[i].Within)
					defined[si] = true
				}
			case "abstract":
				if !abstracts[si] {
					summary.Abstracts = append(summary.Abstracts, hits[i].Within)
					abstracts[si] = true
				}
			case "child":
				if !children[si] {
					summary.Children = append(summary.Children, hits[i].Within)
					children[si] = true
				}
			}
		}
	}
	summary.PatchOperations = len(operations)
	return summary
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
