// Package csharp finds the members of a type in decompiled C# (ilspycmd output) without a
// compiler: enough to print a member index and to cut one member out of a long type
// (05 §5.4). It tokenises (comments, strings and directives are skipped) and follows braces;
// it does not resolve anything, and on text it cannot follow it returns what it has.
package csharp

import (
	"strconv"
	"strings"
)

// Member is one member of a type, or a nested type.
type Member struct {
	Name  string // "ctor", "cctor", "Concerns", "this[]", "operator ==", a nested type's name
	Kind  string // ctor, cctor, method, finalizer, operator, property, indexer, event, field, delegate, type
	Start int    // first line, attributes included (1-based)
	Sig   int    // line of the declaration itself, after the attributes
	End   int    // last line
}

// Type is a type declaration found in the text.
type Type struct {
	Name    string
	Kind    string // class, struct, interface, enum, record
	Start   int
	End     int
	Members []Member
}

// Parse lists the types declared in src, outermost first, nested ones after their outer
// type. Members are the direct members of each type (an enum has none).
func Parse(src string) []*Type {
	p := &parser{toks: tokenize(src)}
	p.body(nil, 0)
	return p.types
}

// Find picks the type to show for a requested name (Namespace.Outer+Inner, a simple name,
// a generic with its arity): the shallowest declaration with that simple name, else the
// first type in the text.
func Find(types []*Type, name string) *Type {
	if i := strings.LastIndexAny(name, ".+"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexByte(name, '`'); i >= 0 {
		name = name[:i]
	}
	for _, t := range types {
		if t.Name == name {
			return t
		}
	}
	if len(types) > 0 {
		return types[0]
	}
	return nil
}

// Match returns the members a request names. Tiers, the first that finds any wins: the
// exact name, the name in any case, the name after an interface prefix (IFoo.Bar), and a
// unique substring. "ctor", ".ctor" and the type's own name select the constructors.
func (t *Type) Match(want string) []Member {
	want = strings.TrimSpace(want)
	switch lw := strings.ToLower(want); lw {
	case ".ctor", "ctor", strings.ToLower(t.Name):
		want = "ctor"
	case ".cctor":
		want = "cctor"
	}
	tiers := []func(string) bool{
		func(n string) bool { return n == want },
		func(n string) bool { return strings.EqualFold(n, want) },
		func(n string) bool { return strings.HasSuffix(strings.ToLower(n), "."+strings.ToLower(want)) },
	}
	for _, ok := range tiers {
		var out []Member
		for _, m := range t.Members {
			if ok(m.Name) {
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	var out []Member
	for _, m := range t.Members {
		if strings.Contains(strings.ToLower(m.Name), strings.ToLower(want)) {
			out = append(out, m)
		}
	}
	if len(out) == 1 {
		return out
	}
	return nil
}

// Index is the one-line member index: "ctor 51, Concerns 61, … Name 141" (methods,
// properties, events, indexers, operators and nested types; fields are counted). Members
// that share a name list their lines together; at most limit entries are named.
func (t *Type) Index(limit int) string {
	type entry struct {
		name  string
		lines []string
	}
	var order []*entry
	byName := map[string]*entry{}
	fields := 0
	for _, m := range t.Members {
		if m.Kind == "field" {
			fields++
			continue
		}
		e := byName[m.Name]
		if e == nil {
			e = &entry{name: m.Name}
			byName[m.Name] = e
			order = append(order, e)
		}
		e.lines = append(e.lines, strconv.Itoa(m.Start))
	}
	var parts []string
	for i, e := range order {
		if limit > 0 && i == limit {
			parts = append(parts, "… "+strconv.Itoa(len(order)-limit)+" more")
			break
		}
		parts = append(parts, e.name+" "+strings.Join(e.lines, "/"))
	}
	s := strings.Join(parts, ", ")
	if fields > 0 {
		if s != "" {
			s += "; "
		}
		s += strconv.Itoa(fields) + " fields"
	}
	return s
}

// ---- tokens ----

type token struct {
	text string // a word, a punctuation mark, or "\"" for a string or character literal
	line int
}

func (t token) word() bool {
	c := t.text[0]
	return c == '_' || c == '@' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

func isWordByte(c byte) bool {
	return c == '_' || c == '@' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

func tokenize(src string) []token {
	var toks []token
	line := 1
	i := 0
	n := len(src)
	// skipString moves past a string or character literal that starts at i (src[i] is the
	// opening quote after any @ / $ prefix) and returns the new index; interpolation holes
	// are walked as code so quotes inside them do not end the string early.
	var skipString func(i int, verbatim, interp bool) int
	skipCode := func(i int, until byte) int { // code up to the unmatched until, for a hole
		depth := 0
		for i < n {
			c := src[i]
			switch {
			case c == '\n':
				line++
			case c == '"':
				i = skipString(i, false, false)
				continue
			case c == '@' && i+1 < n && src[i+1] == '"':
				i = skipString(i+1, true, false)
				continue
			case c == '$' && i+1 < n && (src[i+1] == '"' || src[i+1] == '@' && i+2 < n && src[i+2] == '"'):
				j := i + 1
				verb := false
				if src[j] == '@' {
					verb, j = true, j+1
				}
				i = skipString(j, verb, true)
				continue
			case c == '\'':
				i = skipChar(src, i)
				continue
			case c == '{' || c == '(' || c == '[':
				depth++
			case c == '}' || c == ')' || c == ']':
				if depth == 0 && c == until {
					return i
				}
				depth--
			}
			i++
		}
		return i
	}
	skipString = func(i int, verbatim, interp bool) int {
		// raw string literal: three or more quotes
		if !verbatim && i+2 < n && src[i+1] == '"' && src[i+2] == '"' {
			q := 0
			for i+q < n && src[i+q] == '"' {
				q++
			}
			end := strings.Repeat("\"", q)
			j := strings.Index(src[i+q:], end)
			if j < 0 {
				line += strings.Count(src[i:], "\n")
				return n
			}
			line += strings.Count(src[i:i+q+j], "\n")
			return i + q + j + q
		}
		i++ // the opening quote
		for i < n {
			c := src[i]
			switch {
			case c == '\n':
				line++
			case c == '\\' && !verbatim:
				i += 2
				continue
			case c == '"':
				if verbatim && i+1 < n && src[i+1] == '"' {
					i += 2
					continue
				}
				return i + 1
			case c == '{' && interp:
				if i+1 < n && src[i+1] == '{' {
					i += 2
					continue
				}
				i = skipCode(i+1, '}')
			}
			i++
		}
		return i
	}
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case c == '#' && onlyBlankBefore(src, i):
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				line += strings.Count(src[i:], "\n")
				i = n
				break
			}
			line += strings.Count(src[i:i+2+j], "\n")
			i += 2 + j + 2
		case c == '"':
			at := line
			i = skipString(i, false, false)
			toks = append(toks, token{"\"", at})
		case c == '@' && i+1 < n && src[i+1] == '"':
			at := line
			i = skipString(i+1, true, false)
			toks = append(toks, token{"\"", at})
		case c == '$' && i+1 < n && (src[i+1] == '"' || src[i+1] == '@' && i+2 < n && src[i+2] == '"'):
			at := line
			j, verb := i+1, false
			if src[j] == '@' {
				verb, j = true, j+1
			}
			i = skipString(j, verb, true)
			toks = append(toks, token{"\"", at})
		case c == '@' && i+2 < n && src[i+1] == '$' && src[i+2] == '"':
			at := line
			i = skipString(i+2, true, true)
			toks = append(toks, token{"\"", at})
		case c == '\'':
			at := line
			i = skipChar(src, i)
			toks = append(toks, token{"\"", at})
		case isWordByte(c):
			j := i + 1
			for j < n && isWordByte(src[j]) {
				j++
			}
			toks = append(toks, token{src[i:j], line})
			i = j
		default:
			if i+1 < n {
				switch two := src[i : i+2]; two {
				case "=>", "==", "!=", "<=", ">=":
					toks = append(toks, token{two, line})
					i += 2
					continue
				}
			}
			toks = append(toks, token{string(c), line})
			i++
		}
	}
	return toks
}

func onlyBlankBefore(src string, i int) bool {
	for i--; i >= 0 && src[i] != '\n'; i-- {
		if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			return false
		}
	}
	return true
}

// skipChar moves past a character literal; a stray quote (not a literal) advances by one.
func skipChar(src string, i int) int {
	j := i + 1
	if j < len(src) && src[j] == '\\' {
		j += 2
	} else {
		j++
	}
	for j < len(src) && src[j] != '\'' && src[j] != '\n' && j < i+12 {
		j++ // A, \x41 and \U00000041 escapes are longer than one character
	}
	if j < len(src) && src[j] == '\'' {
		return j + 1
	}
	return i + 1
}

// ---- declarations ----

type parser struct {
	toks  []token
	types []*Type
}

var modifiers = map[string]bool{"public": true, "private": true, "protected": true, "internal": true, "static": true,
	"extern": true, "unsafe": true, "abstract": true, "virtual": true, "override": true, "sealed": true, "new": true,
	"readonly": true, "volatile": true, "async": true, "partial": true, "const": true, "fixed": true, "required": true, "file": true}

// body reads the declarations of a type, namespace or file from index i until the closing
// brace that ends it (not consumed) or the end; owner is the type whose members they are
// (nil in a namespace or at file level). It returns the index where it stopped.
func (p *parser) body(owner *Type, i int) int {
	toks := p.toks
	for i < len(toks) {
		if toks[i].text == "}" {
			return i
		}
		if toks[i].text == ";" {
			i++
			continue
		}
		// attributes
		start := toks[i].line
		for i < len(toks) && toks[i].text == "[" {
			i = skipGroup(toks, i)
		}
		if i >= len(toks) || toks[i].text == "}" {
			return i
		}
		sig := toks[i].line
		// header: up to { ; = => at the top level of parentheses and brackets
		var h []token
		depth := 0
	header:
		for ; i < len(toks); i++ {
			t := toks[i]
			switch t.text {
			case "(", "[":
				depth++
			case ")", "]":
				depth--
			case "{", ";", "=", "=>", "}":
				if depth <= 0 {
					break header
				}
			}
			h = append(h, t)
		}
		if i >= len(toks) {
			return i
		}
		end := toks[i]
		switch end.text {
		case "}":
			return i
		case ";":
			p.member(owner, h, start, sig, end.line, ";")
			i++
		case "=", "=>":
			// an initializer or an expression body: up to the ; that ends the member
			i = skipTo(toks, i+1, ";")
			endLine := toks[min(i, len(toks)-1)].line
			p.member(owner, h, start, sig, endLine, end.text)
			if i < len(toks) && toks[i].text == ";" {
				i++
			}
		case "{":
			kind, name := typeHeader(h)
			switch {
			case kind == "namespace":
				i = p.body(owner, i+1)
				if i < len(toks) {
					i++
				}
			case kind != "" && kind != "enum":
				t := &Type{Name: name, Kind: kind, Start: start}
				p.types = append(p.types, t)
				if owner != nil {
					owner.Members = append(owner.Members, Member{Name: name, Kind: "type", Start: start, Sig: sig})
				}
				i = p.body(t, i+1)
				if i < len(toks) {
					t.End = toks[i].line
					i++
				} else if len(toks) > 0 {
					t.End = toks[len(toks)-1].line
				}
				if owner != nil {
					owner.Members[len(owner.Members)-1].End = t.End
				}
			case kind == "enum":
				t := &Type{Name: name, Kind: kind, Start: start}
				p.types = append(p.types, t)
				j := skipGroup(toks, i)
				t.End = toks[j-1].line
				i = j
				if owner != nil {
					owner.Members = append(owner.Members, Member{Name: name, Kind: "type", Start: start, Sig: sig, End: t.End})
				}
			default:
				j := skipGroup(toks, i)
				endLine := toks[j-1].line
				// { get; set; } = value;  and  new T { ... }  keep going to the ;
				if j < len(toks) && (toks[j].text == "=" || toks[j].text == ";") {
					j = skipTo(toks, j, ";")
					endLine = toks[min(j, len(toks)-1)].line
					if j < len(toks) {
						j++
					}
				}
				p.member(owner, h, start, sig, endLine, "{")
				i = j
			}
		}
	}
	return i
}

// skipGroup returns the index after the group that opens at toks[i] ([ ( or {).
func skipGroup(toks []token, i int) int {
	depth := 0
	for ; i < len(toks); i++ {
		switch toks[i].text {
		case "[", "(", "{":
			depth++
		case "]", ")", "}":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// skipTo returns the index of the first stop token at nesting depth 0 from i, or of the
// closing brace that ends the enclosing body (an enum's last value has no ;).
func skipTo(toks []token, i int, stop string) int {
	depth := 0
	for ; i < len(toks); i++ {
		switch t := toks[i].text; t {
		case "[", "(", "{":
			depth++
		case "]", ")":
			depth--
		case "}":
			if depth == 0 {
				return i
			}
			depth--
		default:
			if t == stop && depth == 0 {
				return i
			}
		}
	}
	return i
}

// typeHeader says whether a header declares a namespace or a type, and its name.
func typeHeader(h []token) (kind, name string) {
	for i, t := range h {
		switch t.text {
		case "(", "=":
			return "", ""
		case "namespace":
			return "namespace", ""
		case "class", "struct", "interface", "enum":
			if i+1 < len(h) && h[i+1].word() {
				return t.text, strings.TrimPrefix(h[i+1].text, "@")
			}
		case "record":
			j := i + 1
			if j < len(h) && (h[j].text == "struct" || h[j].text == "class") {
				j++
			}
			if j < len(h) && h[j].word() {
				return "record", h[j].text
			}
		}
	}
	return "", ""
}

// member records a member of owner from its header tokens.
func (p *parser) member(owner *Type, h []token, start, sig, end int, term string) {
	if owner == nil || len(h) == 0 {
		return
	}
	m := Member{Start: start, Sig: sig, End: end}
	// the position of the first parenthesis (a method-like header) and of "operator"
	paren := -1
	for i, t := range h {
		if t.text == "(" {
			paren = i
			break
		}
	}
	switch {
	case hasWord(h, "delegate") && paren > 0:
		m.Kind, m.Name = "delegate", dotted(h, paren-1)
	case paren > 0:
		opAt := -1
		for i := 0; i < paren; i++ {
			if h[i].text == "operator" {
				opAt = i
			}
		}
		switch {
		case opAt >= 0:
			var sb strings.Builder
			for _, t := range h[opAt+1 : paren] {
				sb.WriteString(t.text)
				sb.WriteByte(' ')
			}
			m.Kind, m.Name = "operator", "operator "+strings.TrimSpace(sb.String())
		default:
			name := dotted(h, genericStart(h, paren-1))
			prefix := h[:identStart(h, genericStart(h, paren-1))]
			switch {
			case len(prefix) > 0 && prefix[len(prefix)-1].text == "~":
				m.Kind, m.Name = "finalizer", "~"+name
			case name == owner.Name && onlyModifiers(prefix):
				m.Kind, m.Name = "ctor", "ctor"
				if hasWord(prefix, "static") {
					m.Kind, m.Name = "cctor", "cctor"
				}
			default:
				m.Kind, m.Name = "method", name
			}
		}
	case hasWord(h, "this") && indexOf(h, "[") >= 0:
		m.Kind, m.Name = "indexer", "this[]"
	case hasWord(h, "event"):
		m.Kind, m.Name = "event", dotted(h, len(h)-1)
	default:
		m.Kind, m.Name = "property", dotted(h, len(h)-1)
		if term == ";" || term == "=" {
			m.Kind = "field"
		}
	}
	if m.Name == "" || strings.ContainsAny(m.Name, "<>") && m.Kind == "type" {
		return
	}
	owner.Members = append(owner.Members, m)
}

func hasWord(h []token, w string) bool {
	for _, t := range h {
		if t.text == w {
			return true
		}
	}
	return false
}

func indexOf(h []token, w string) int {
	for i, t := range h {
		if t.text == w {
			return i
		}
	}
	return -1
}

func onlyModifiers(h []token) bool {
	for _, t := range h {
		if !modifiers[t.text] {
			return false
		}
	}
	return true
}

// genericStart: i indexes the last token of a name; when that is the ">" closing a
// generic argument list (Foo<T>), it returns the index of the name before the "<".
func genericStart(h []token, i int) int {
	if i >= 0 && i < len(h) && h[i].text == ">" {
		depth := 0
		for ; i >= 0; i-- {
			switch h[i].text {
			case ">":
				depth++
			case "<":
				depth--
				if depth == 0 {
					return i - 1
				}
			}
		}
	}
	return i
}

// identStart is the index where the dotted name ending at i begins.
func identStart(h []token, i int) int {
	for i >= 2 && h[i-1].text == "." && h[i-2].word() {
		i -= 2
	}
	if i < 0 {
		return 0
	}
	return i
}

// dotted is the name that ends at h[i], with an interface prefix (IFoo.Bar) and without
// generic arguments.
func dotted(h []token, i int) string {
	i = genericStart(h, i)
	if i < 0 || i >= len(h) {
		return ""
	}
	start := identStart(h, i)
	var parts []string
	for j := start; j <= i; j++ {
		if h[j].text == "." || !h[j].word() {
			if h[j].text == "." {
				parts = append(parts, ".")
			}
			continue
		}
		parts = append(parts, strings.TrimPrefix(h[j].text, "@"))
	}
	return strings.Join(parts, "")
}
