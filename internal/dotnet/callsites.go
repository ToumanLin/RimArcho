package dotnet

// Call sites of a method and the shapes of the values each passes (read_source mode=callers,
// the null trace's caller step and game contract; 05 §5.4).

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// Target names a method: its declaring type ('+' or '.' for nesting), its name (".ctor" for
// constructors) and its parameter count (-1: any overload).
type Target struct {
	Owner, Name string
	Params      int
}

// ParseTarget reads "Ns.Type.Method", "Ns.Type..ctor" or "Ns.Type:Method", with an optional
// parameter count "(4)" or parameter list "(Pawn a, Pawn b)".
func ParseTarget(s string) (Target, bool) {
	s = strings.TrimSpace(s)
	t := Target{Params: -1}
	if i := strings.IndexByte(s, '('); i > 0 && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[i+1 : len(s)-1])
		s = strings.TrimSpace(s[:i])
		switch {
		case inner == "":
			t.Params = 0
		case isDigits(inner):
			t.Params = atoi(inner)
		default:
			t.Params = len(SplitParams(inner))
		}
	}
	if i := strings.Index(s, ".."); i > 0 {
		t.Owner, t.Name = s[:i], s[i+1:]
	} else if i := strings.LastIndexByte(s, ':'); i > 0 {
		t.Owner, t.Name = s[:i], s[i+1:]
	} else if i := strings.LastIndexByte(s, '.'); i > 0 {
		t.Owner, t.Name = s[:i], s[i+1:]
	}
	return t, t.Owner != "" && t.Name != ""
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// SplitParams splits a frame's parameter list ("A a, List`1[T] b") at top-level commas.
func SplitParams(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '<', '(':
			depth++
		case ']', '>', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func normType(t string) string { return strings.ReplaceAll(t, "+", ".") }

func (t Target) matches(m Member) bool {
	return m.Name == t.Name && normType(m.Owner) == normType(t.Owner) && (t.Params < 0 || m.Params == t.Params)
}

// FindMethods returns the MethodDef rows of type typ named name with params parameters
// (-1: any).
func (a *Assembly) FindMethods(typ, name string, params int) []uint32 {
	t := a.TypeRow(typ)
	if t == 0 {
		t = a.TypeRow(strings.ReplaceAll(typ, ".", "+"))
	}
	if t == 0 {
		// Nested types printed with dots: try the last dots as '+'.
		parts := strings.Split(typ, ".")
		for k := len(parts) - 1; k > 0 && t == 0; k-- {
			t = a.TypeRow(strings.Join(parts[:k], ".") + "+" + strings.Join(parts[k:], "+"))
		}
	}
	var out []uint32
	for _, m := range a.Methods(t) {
		if m.Name == name && (params < 0 || m.Params == params) {
			out = append(out, m.Row)
		}
	}
	return out
}

// CallSite is one call, callvirt or newobj of a target.
type CallSite struct {
	Method uint32 // the calling MethodDef row
	Caller string // Type.Method of the caller
	Off    int
	Op     uint16
	Base   bool     // a constructor called with call: a base or this(…) constructor call
	Callee Member   // the target as referenced
	This   *Shape   // the instance for call / callvirt of an instance method
	Args   []Shape  // the parameters' values, this excluded
	Names  []string // the caller's parameter names (for Describe)
}

// MethodLabel is "Ns.Type.Method" of MethodDef row.
func (a *Assembly) MethodLabel(row uint32) string {
	return strings.ReplaceAll(a.MethodDeclType(row), "+", ".") + "." + a.MethodName(row)
}

// targetTokens are the tokens that can name a method matching t in this assembly.
func (a *Assembly) targetTokens(name string, match func(Member) bool) [][4]byte {
	var toks [][4]byte
	add := func(tok uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], tok)
		toks = append(toks, b)
	}
	defs := map[uint32]bool{}
	for r := uint32(1); int(r) <= len(a.methods); r++ {
		if a.str(a.methods[r-1].Name) != name {
			continue
		}
		if m, ok := a.Member(0x06<<24 | r); ok && match(m) {
			add(0x06<<24 | r)
			defs[r] = true
		}
	}
	refs := map[uint32]bool{}
	for r := uint32(1); int(r) <= len(a.members); r++ {
		if a.str(a.members[r-1].Name) != name {
			continue
		}
		if m, ok := a.Member(0x0A<<24 | r); ok && !m.Field && match(m) {
			add(0x0A<<24 | r)
			refs[r] = true
		}
	}
	for r := uint32(1); int(r) <= len(a.methSpecs); r++ {
		c := a.methSpecs[r-1].Method
		if c&1 == 0 && defs[c>>1] || c&1 == 1 && refs[c>>1] {
			add(0x2B<<24 | r)
		}
	}
	return toks
}

// CallSites lists every call of a method matching t in the assembly, in method order.
func (a *Assembly) CallSites(t Target) []CallSite {
	return a.callSites(t.Name, t.matches, 0)
}

// CallSitesOf lists the calls of methods named name that pass match, in MethodDef row only
// (0: every method).
func (a *Assembly) CallSitesOf(row uint32, name string, match func(Member) bool) []CallSite {
	return a.callSites(name, match, row)
}

func (a *Assembly) callSites(name string, match func(Member) bool, only uint32) []CallSite {
	toks := a.targetTokens(name, match)
	if len(toks) == 0 {
		return nil
	}
	var out []CallSite
	first, last := uint32(1), uint32(len(a.methods))
	if only != 0 {
		first, last = only, only
	}
	for r := first; r <= last && int(r) <= len(a.methods); r++ {
		body := a.Body(r)
		hit := false
		for _, t := range toks {
			if bytes.Contains(body, t[:]) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		var names []string
		a.eval(r, func(in Instr, st []Shape) {
			if in.Op != 0x28 && in.Op != 0x6f && in.Op != 0x73 {
				return
			}
			m, ok := a.Member(uint32(in.Arg))
			if !ok || !match(m) {
				return
			}
			if names == nil {
				names = a.ParamNames(r)
			}
			cs := CallSite{Method: r, Caller: a.MethodLabel(r), Off: in.Off, Op: in.Op, Callee: m, Names: names,
				Base: in.Op == 0x28 && m.Name == ".ctor"}
			n := m.Params
			withThis := m.HasThis && in.Op != 0x73
			if withThis {
				n++
			}
			vals := make([]Shape, n)
			for i := 0; i < n; i++ {
				if j := len(st) - n + i; j >= 0 {
					vals[i] = st[j]
				}
			}
			if withThis {
				th := vals[0]
				cs.This, vals = &th, vals[1:]
			}
			cs.Args = vals
			out = append(out, cs)
		})
	}
	return out
}
