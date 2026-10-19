package dotnet

import (
	"fmt"
	"strings"
)

// TypeCount is the number of TypeDef rows.
func (a *Assembly) TypeCount() int { return len(a.typeDefs) }

// TypeName returns the full name of TypeDef row (1-based); nested types use '+'.
func (a *Assembly) TypeName(row uint32) string { return a.typeDefName(row) }

// TypeNames lists every TypeDef except <Module> and compiler-generated types.
func (a *Assembly) TypeNames() []string {
	var out []string
	for i := range a.typeDefs {
		n := a.typeDefName(uint32(i + 1))
		if n == "<Module>" || strings.Contains(n, "<") {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Method is one MethodDef.
type Method struct {
	Row        uint32
	Name       string
	ReturnBool bool
	Params     int
}

// Methods returns the MethodDefs of TypeDef row (1-based).
func (a *Assembly) Methods(row uint32) []Method {
	if row == 0 || int(row) > len(a.typeDefs) {
		return nil
	}
	start := a.typeDefs[row-1].MethodList
	end := uint32(len(a.methods) + 1)
	if int(row) < len(a.typeDefs) {
		end = a.typeDefs[row].MethodList
	}
	var out []Method
	for m := start; m < end && m >= 1 && int(m) <= len(a.methods); m++ {
		md := a.methods[m-1]
		sig := a.blobAt(md.Signature)
		mt := Method{Row: m, Name: a.str(md.Name)}
		if len(sig) >= 3 {
			r := &reader{b: sig}
			cc := r.u8()
			if cc&0x10 != 0 { // generic: parameter count first
				r.compressed()
			}
			mt.Params = int(r.compressed())
			mt.ReturnBool = r.u8() == 0x02
		}
		out = append(out, mt)
	}
	return out
}

// Attribute parents.
const (
	ParentType   = 1
	ParentMethod = 2
	ParentOther  = 0
)

// Attr2 is an attribute with its parent row identified.
type Attr2 struct {
	Attr
	ParentKind int
	ParentRow  uint32
}

// AttributesOf decodes every custom attribute whose type passes keep, with parent rows.
func (a *Assembly) AttributesOf(keep func(string) bool) []Attr2 {
	var out []Attr2
	all := a.Attributes(keep)
	k := 0
	for _, ca := range a.attrs {
		typ, _ := a.ctorInfo(ca.Type)
		if !keep(typ) {
			continue
		}
		x := Attr2{Attr: all[k]}
		k++
		idx := ca.Parent >> 5
		switch ca.Parent & 31 {
		case 0:
			x.ParentKind, x.ParentRow = ParentMethod, idx
		case 3:
			x.ParentKind, x.ParentRow = ParentType, idx
		}
		out = append(out, x)
	}
	return out
}

// MethodOwner returns the TypeDef row owning MethodDef row m.
func (a *Assembly) MethodOwner(m uint32) uint32 { return a.methodOwner(m) }

// EnclosingType returns the enclosing TypeDef row of a nested type (0 if none).
func (a *Assembly) EnclosingType(row uint32) uint32 { return a.nestedIn[row] }

// TypeRefs lists referenced type names (for "references Harmony").
func (a *Assembly) References(owner string) bool {
	for i := range a.typeRefs {
		if strings.HasPrefix(a.typeRefName(uint32(i+1)), owner) {
			return true
		}
	}
	return false
}

// CLRName strips the assembly qualification from a serialized type name:
// "Verse.Pawn, Assembly-CSharp, Version=…" → "Verse.Pawn".
func CLRName(s string) string {
	name, rest := splitTypeName(s)
	if rest == "" {
		return name
	}
	var args []string
	for _, g := range splitGenericArgs(rest) {
		args = append(args, CLRName(g))
	}
	return name + "[" + strings.Join(args, ",") + "]"
}

// CSName renders a serialized type name in C# syntax with short keywords (int, string …).
func CSName(s string) string { return clrToCS(s) }

func splitTypeName(s string) (string, string) {
	s = strings.TrimSpace(s)
	var name strings.Builder
	depth := 0
	start, end := -1, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '[' && depth == 0 && i+1 < len(s) && s[i+1] == '[':
			start = i + 1
			depth++
		case c == '[' && depth > 0:
			depth++
		case c == ']' && depth > 0:
			depth--
			if depth == 0 {
				end = i
			}
		case c == ',' && depth == 0:
			return name.String(), sliceOr(s, start, end)
		default:
			if depth == 0 {
				name.WriteByte(c)
			}
		}
	}
	return name.String(), sliceOr(s, start, end)
}

func sliceOr(s string, a, b int) string {
	if a < 0 || b < 0 {
		return ""
	}
	return s[a:b]
}

func splitGenericArgs(s string) []string {
	var out []string
	depth := 0
	cur := strings.Builder{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '[':
			if depth > 0 {
				cur.WriteByte(c)
			}
			depth++
		case c == ']':
			depth--
			if depth > 0 {
				cur.WriteByte(c)
			} else {
				out = append(out, cur.String())
				cur.Reset()
			}
		case c == ',' && depth == 0:
		default:
			cur.WriteByte(c)
		}
	}
	return out
}

var keywords = map[string]string{
	"System.Boolean": "bool", "System.Byte": "byte", "System.SByte": "sbyte", "System.Char": "char",
	"System.Int16": "short", "System.UInt16": "ushort", "System.Int32": "int", "System.UInt32": "uint",
	"System.Int64": "long", "System.UInt64": "ulong", "System.Single": "float", "System.Double": "double",
	"System.String": "string", "System.Object": "object", "System.Decimal": "decimal", "System.Void": "void",
}

func clrToCS(s string) string {
	name, rest := splitTypeName(s)
	arr := ""
	for strings.HasSuffix(name, "[]") {
		arr += "[]"
		name = strings.TrimSuffix(name, "[]")
	}
	byRef := strings.HasSuffix(name, "&")
	name = strings.TrimSuffix(name, "&")
	name = strings.ReplaceAll(name, "+", ".")
	if kw, ok := keywords[name]; ok {
		name = kw
	}
	if i := strings.IndexByte(name, '`'); i >= 0 {
		n := 0
		fmt.Sscan(name[i+1:], &n)
		base := name[:i]
		if rest == "" {
			return base + "<" + strings.Repeat(",", max(0, n-1)) + ">" + arr
		}
		var args []string
		for _, g := range splitGenericArgs(rest) {
			args = append(args, clrToCS(g))
		}
		name = base + "<" + strings.Join(args, ", ") + ">"
	}
	if byRef {
		name += "&"
	}
	return name + arr
}
