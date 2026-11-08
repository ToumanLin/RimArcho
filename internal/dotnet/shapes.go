package dotnet

// Value shapes on the IL evaluation stack, for call-site tables and null traces (05 §5.4,
// 06 §2): a small abstract interpretation that only says where a value came from (null
// literal, this, a parameter, a local, a field, a constant, a call). Like the manual-patch
// scan (internal/harmony/manual.go) it has no real control flow: stacks merge at branch
// targets and differing values become ShOther, so a desynchronised stack yields ShOther,
// never a wrong null.

import (
	"fmt"
	"strings"
)

// ShapeKind is what pushed a value.
type ShapeKind uint8

const (
	ShOther  ShapeKind = iota
	ShNull             // ldnull, or a local whose only store is ldnull
	ShThis             // ldarg.0 of an instance method
	ShParam            // N: parameter index, this not counted
	ShLocal            // N: local index
	ShField            // an instance field (Member), of Base
	ShStatic           // a static field (Member)
	ShConst            // a number (Text)
	ShString           // ldstr (Text)
	ShNew              // newobj / newarr (Member: the type)
	ShCall             // a call's result (Member)
)

// Shape is where a stack value came from.
type Shape struct {
	Kind   ShapeKind
	N      int
	Member string // Owner::Name for fields and calls, the type for ShNew
	Text   string // constant text
	Base   *Shape // the object of an instance field
	Off    int    // the instruction that pushed it
	Op     uint16
}

// Label is the shape's class: null, this, param, local, field, const or other.
func (s Shape) Label() string {
	switch s.Kind {
	case ShNull:
		return "null"
	case ShThis:
		return "this"
	case ShParam:
		return "param"
	case ShLocal:
		return "local"
	case ShField, ShStatic:
		return "field"
	case ShConst, ShString:
		return "const"
	}
	return "other"
}

// Describe is the shape with its detail: "param pawn", "field Verse.Pawn::genes", "null".
// names are the method's parameter names (may be short or nil).
func (s Shape) Describe(names []string) string {
	switch s.Kind {
	case ShNull:
		if s.Text == "local" {
			return fmt.Sprintf("null (local %d)", s.N)
		}
		return "null"
	case ShParam:
		if s.N < len(names) && names[s.N] != "" {
			return "param " + names[s.N]
		}
		return fmt.Sprintf("param %d", s.N+1)
	case ShLocal:
		return fmt.Sprintf("local %d", s.N)
	case ShField, ShStatic:
		return "field " + s.Member
	case ShConst:
		return "const " + s.Text
	case ShString:
		return "const \"" + clip(s.Text, 30) + "\""
	case ShNew:
		return "other (new " + shortName(s.Member) + ")"
	case ShCall:
		return "other (" + shortMember(s.Member) + "())"
	}
	return s.Label()
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func shortName(t string) string {
	if i := strings.LastIndexAny(t, ".+"); i >= 0 {
		return t[i+1:]
	}
	return t
}

func shortMember(m string) string {
	owner, name, ok := strings.Cut(m, "::")
	if !ok {
		return m
	}
	return shortName(owner) + "." + name
}

// evalHook sees each instruction with the stack before it executes (top last).
type evalHook func(in Instr, st []Shape)

// eval runs the shape evaluation over MethodDef row's body; false when it has none.
func (a *Assembly) eval(row uint32, hook evalHook) bool {
	code := a.Body(row)
	if len(code) == 0 {
		return false
	}
	self, _ := a.Member(0x06<<24 | row)
	ins := DecodeIL(code)
	targets := map[int]bool{}
	for _, in := range ins {
		if IsBranch(in.Op) {
			targets[int(in.Arg)] = true
		}
		for _, t := range in.Targets {
			targets[t] = true
		}
	}
	// A local counts as null only when its one store is ldnull and its address is never taken.
	stores, nullStore, addr := map[int64]int{}, map[int64]bool{}, map[int64]bool{}
	for i, in := range ins {
		if n, ok := stlocIndex(in); ok {
			stores[n]++
			nullStore[n] = i > 0 && ins[i-1].Op == 0x14 && !targets[in.Off]
		}
		if in.Op == 0x12 || in.Op == 0xfe0d {
			addr[in.Arg] = true
		}
	}
	nullLocal := func(n int64) bool { return stores[n] == 1 && nullStore[n] && !addr[n] }

	var st []Shape
	snap := map[int][]Shape{}
	dead := false
	record := func(t int) {
		cp := append([]Shape(nil), st...)
		if old, ok := snap[t]; ok {
			cp = mergeShapes(old, cp)
		}
		snap[t] = cp
	}
	pop := func() Shape {
		if len(st) == 0 {
			return Shape{}
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	popN := func(n int) {
		for i := 0; i < n; i++ {
			pop()
		}
	}
	for _, in := range ins {
		if targets[in.Off] {
			sn, ok := snap[in.Off]
			switch {
			case dead && ok:
				st = append(st[:0], sn...)
			case dead:
				st = st[:0]
			case ok:
				st = mergeShapes(st, sn)
			}
		} else if dead {
			st = st[:0]
		}
		dead = false
		if hook != nil {
			hook(in, st)
		}
		op := in.Op
		push := func(s Shape) {
			s.Off, s.Op = in.Off, op
			st = append(st, s)
		}
		ldarg := func(n int64) {
			if self.HasThis {
				if n == 0 {
					push(Shape{Kind: ShThis})
					return
				}
				n--
			}
			push(Shape{Kind: ShParam, N: int(n)})
		}
		ldloc := func(n int64) {
			if nullLocal(n) {
				push(Shape{Kind: ShNull, N: int(n), Text: "local"})
				return
			}
			push(Shape{Kind: ShLocal, N: int(n)})
		}
		unary := func() { pop(); push(Shape{}) }
		switch {
		case op == 0x00 || op == 0x01:
		case op >= 0x02 && op <= 0x05:
			ldarg(int64(op - 0x02))
		case op >= 0x06 && op <= 0x09:
			ldloc(int64(op - 0x06))
		case op >= 0x0a && op <= 0x0d, op == 0x13, op == 0xfe0e, op == 0x10, op == 0xfe0b, op == 0x26:
			pop() // stloc, starg, pop
		case op == 0x0e || op == 0xfe09:
			ldarg(in.Arg)
		case op == 0x11 || op == 0xfe0c:
			ldloc(in.Arg)
		case op == 0x0f || op == 0xfe0a || op == 0x12 || op == 0xfe0d: // ldarga, ldloca
			push(Shape{})
		case op == 0x14:
			push(Shape{Kind: ShNull})
		case op >= 0x15 && op <= 0x1e:
			push(Shape{Kind: ShConst, Text: fmt.Sprint(int64(op) - 0x16)})
		case op == 0x1f || op == 0x20 || op == 0x21:
			push(Shape{Kind: ShConst, Text: fmt.Sprint(in.Arg)})
		case op == 0x22 || op == 0x23:
			push(Shape{Kind: ShConst, Text: "float"})
		case op == 0x25:
			v := pop()
			st = append(st, v, v)
		case op == 0x27 || op == 0x29 || op == 0x2a || op == 0x7a || op == 0xdc || op == 0xfe1a || op == 0xfe11:
			// jmp, calli, ret, throw, endfinally, rethrow, endfilter
			st = st[:0]
			dead = op != 0x29
		case op == 0xdd || op == 0xde: // leave
			st = st[:0]
			record(int(in.Arg))
			dead = true
		case op == 0x2b || op == 0x38:
			record(int(in.Arg))
			dead = true
		case op == 0x2c || op == 0x2d || op == 0x39 || op == 0x3a:
			pop()
			record(int(in.Arg))
		case op >= 0x2e && op <= 0x37 || op >= 0x3b && op <= 0x44:
			popN(2)
			record(int(in.Arg))
		case op == 0x45:
			pop()
			for _, t := range in.Targets {
				record(t)
			}
		case op >= 0x46 && op <= 0x50:
			unary() // ldind
		case op >= 0x51 && op <= 0x57 || op == 0xdf || op == 0x70 || op == 0x81:
			popN(2) // stind, cpobj, stobj
		case op >= 0x58 && op <= 0x64 || op >= 0xd6 && op <= 0xdb || op >= 0xfe01 && op <= 0xfe05:
			popN(2)
			push(Shape{})
		case op == 0x65 || op == 0x66 || op >= 0x67 && op <= 0x6e || op == 0x76 || op >= 0x82 && op <= 0x8b ||
			op >= 0xb3 && op <= 0xba || op == 0xc2 || op == 0xc3 || op == 0xc6 || op >= 0xd1 && op <= 0xd5 ||
			op == 0xe0 || op == 0x8e || op == 0xfe0f || op == 0xfe1d || op == 0x71 || op == 0x79 || op == 0xa5 || op == 0x75 || op == 0xfe07:
			unary() // conversions, ldlen, ldobj, unbox, isinst (may be null), ldvirtftn …
		case op == 0x74 || op == 0x8c:
			// castclass, box: the same value
		case op == 0x72:
			push(Shape{Kind: ShString, Text: a.UserString(uint32(in.Arg))})
		case op == 0x7b || op == 0x7c:
			obj := pop()
			m, _ := a.Member(uint32(in.Arg))
			push(Shape{Kind: ShField, Member: m.Owner + "::" + m.Name, Base: &obj})
		case op == 0x7d:
			popN(2)
		case op == 0x7e || op == 0x7f:
			m, _ := a.Member(uint32(in.Arg))
			push(Shape{Kind: ShStatic, Member: m.Owner + "::" + m.Name})
		case op == 0x80:
			pop()
		case op == 0x8d:
			pop()
			push(Shape{Kind: ShNew, Member: a.TypeToken(uint32(in.Arg)) + "[]"})
		case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3:
			popN(2)
			push(Shape{})
		case op >= 0x9b && op <= 0xa2 || op == 0xa4 || op == 0xfe17 || op == 0xfe18:
			popN(3)
		case op == 0xd0 || op == 0xfe06 || op == 0xfe1c || op == 0xfe00:
			push(Shape{}) // ldtoken, ldftn, sizeof, arglist
		case op == 0xfe15:
			pop() // initobj
		case op == 0x28 || op == 0x6f || op == 0x73:
			m, ok := a.Member(uint32(in.Arg))
			if !ok {
				st = st[:0]
				continue
			}
			n := m.Params
			if m.HasThis && op != 0x73 {
				n++
			}
			popN(n)
			switch {
			case op == 0x73:
				push(Shape{Kind: ShNew, Member: m.Owner})
			case !m.RetVoid:
				push(Shape{Kind: ShCall, Member: m.Owner + "::" + m.Name})
			}
		default:
			// prefixes (unaligned, volatile, tail, constrained, readonly, no.): no stack effect
		}
	}
	return true
}

func stlocIndex(in Instr) (int64, bool) {
	switch {
	case in.Op >= 0x0a && in.Op <= 0x0d:
		return int64(in.Op - 0x0a), true
	case in.Op == 0x13 || in.Op == 0xfe0e:
		return in.Arg, true
	}
	return 0, false
}

func sameShape(a, b Shape) bool {
	if a.Kind != b.Kind || a.N != b.N || a.Member != b.Member || a.Text != b.Text {
		return false
	}
	if (a.Base == nil) != (b.Base == nil) {
		return false
	}
	return a.Base == nil || sameShape(*a.Base, *b.Base)
}

func mergeShapes(a, b []Shape) []Shape {
	if len(a) != len(b) {
		return a
	}
	out := make([]Shape, len(a))
	for i := range a {
		out[i] = a[i]
		if !sameShape(a[i], b[i]) {
			out[i] = Shape{}
		}
	}
	return out
}
