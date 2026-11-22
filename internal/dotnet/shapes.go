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

// shapeEval is the per-method state of eval except the stack. The stack is a separate
// shapeStack so that these maps can stay off the heap.
type shapeEval struct {
	a         *Assembly
	hasThis   bool            // the method is an instance method: ldarg.0 is this
	snap      map[int][]Shape // stacks recorded at branch targets
	dead      bool            // after an unconditional branch, ret or throw
	stores    map[int64]int   // stores per local
	nullStore map[int64]bool  // the local's last store is ldnull
	addr      map[int64]bool  // the local's address is taken
}

// shapeStack is eval's stack and the instruction being applied.
type shapeStack struct {
	v   []Shape
	off int
	op  uint16
}

// eval runs the shape evaluation over MethodDef row's body; false when it has none.
func (a *Assembly) eval(row uint32, hook evalHook) bool {
	code := a.Body(row)
	if len(code) == 0 {
		return false
	}
	self, _ := a.Member(0x06<<24 | row)
	ins := DecodeIL(code)
	targets := map[int]bool{}
	addBranchTargets(targets, ins)
	e := shapeEval{a: a, hasThis: self.HasThis, snap: map[int][]Shape{},
		stores: map[int64]int{}, nullStore: map[int64]bool{}, addr: map[int64]bool{}}
	e.scanLocals(ins, targets)
	var s shapeStack
	for _, in := range ins {
		e.join(in.Off, targets[in.Off], &s)
		e.dead = false
		if hook != nil {
			hook(in, s.v)
		}
		s.off, s.op = in.Off, in.Op
		e.step(in, &s)
	}
	return true
}

// addBranchTargets adds the offsets some branch or switch jumps to. The caller makes the map,
// so that a small one can stay off the heap.
func addBranchTargets(targets map[int]bool, ins []Instr) {
	for _, in := range ins {
		if IsBranch(in.Op) {
			targets[int(in.Arg)] = true
		}
		for _, t := range in.Targets {
			targets[t] = true
		}
	}
}

// scanLocals counts each local's stores, whether the last is ldnull, and whether its address
// is taken.
func (e *shapeEval) scanLocals(ins []Instr, targets map[int]bool) {
	for i, in := range ins {
		if n, ok := stlocIndex(in); ok {
			e.stores[n]++
			e.nullStore[n] = i > 0 && ins[i-1].Op == 0x14 && !targets[in.Off]
		}
		if in.Op == 0x12 || in.Op == 0xfe0d {
			e.addr[in.Arg] = true
		}
	}
}

// nullLocal: a local counts as null only when its one store is ldnull and its address is never
// taken.
func (e *shapeEval) nullLocal(n int64) bool { return e.stores[n] == 1 && e.nullStore[n] && !e.addr[n] }

// join sets the stack at off: the fall-through stack merged with the stacks branched here.
func (e *shapeEval) join(off int, target bool, s *shapeStack) {
	if target {
		sn, ok := e.snap[off]
		switch {
		case e.dead && ok:
			s.v = append(s.v[:0], sn...)
		case e.dead:
			s.v = s.v[:0]
		case ok:
			s.v = mergeShapes(s.v, sn)
		}
	} else if e.dead {
		s.v = s.v[:0]
	}
}

// record saves the stack for branch target t, merged with what was recorded there before.
func (e *shapeEval) record(t int, s *shapeStack) {
	cp := append([]Shape(nil), s.v...)
	if old, ok := e.snap[t]; ok {
		cp = mergeShapes(old, cp)
	}
	e.snap[t] = cp
}

// pop takes the top of the stack; an empty stack gives ShOther.
func (s *shapeStack) pop() Shape {
	if len(s.v) == 0 {
		return Shape{}
	}
	v := s.v[len(s.v)-1]
	s.v = s.v[:len(s.v)-1]
	return v
}

// popN drops the top n values.
func (s *shapeStack) popN(n int) {
	for i := 0; i < n; i++ {
		s.pop()
	}
}

// push puts sh on the stack, pushed by the current instruction.
func (s *shapeStack) push(sh Shape) {
	sh.Off, sh.Op = s.off, s.op
	s.v = append(s.v, sh)
}

// unary replaces the top value with ShOther.
func (s *shapeStack) unary() { s.pop(); s.push(Shape{}) }

// ldarg pushes argument n: this, or a parameter not counting this.
func (e *shapeEval) ldarg(n int64, s *shapeStack) {
	if e.hasThis {
		if n == 0 {
			s.push(Shape{Kind: ShThis})
			return
		}
		n--
	}
	s.push(Shape{Kind: ShParam, N: int(n)})
}

// ldloc pushes local n, as null when nullLocal says so.
func (e *shapeEval) ldloc(n int64, s *shapeStack) {
	if e.nullLocal(n) {
		s.push(Shape{Kind: ShNull, N: int(n), Text: "local"})
		return
	}
	s.push(Shape{Kind: ShLocal, N: int(n)})
}

// step applies one instruction. The handlers keep the opcode cases in their original order;
// prefixes (unaligned, volatile, tail, constrained, readonly, no.) have no stack effect.
func (e *shapeEval) step(in Instr, s *shapeStack) {
	_ = e.stepLoad(in, s) || e.stepControl(in, s) || stepShapeArith(in, s) || e.stepObject(in, s) || e.stepCall(in, s)
}

// stepLoad handles nop, break, arguments, locals, constants and dup.
func (e *shapeEval) stepLoad(in Instr, s *shapeStack) bool {
	op := in.Op
	switch {
	case op == 0x00 || op == 0x01:
	case op >= 0x02 && op <= 0x05:
		e.ldarg(int64(op-0x02), s)
	case op >= 0x06 && op <= 0x09:
		e.ldloc(int64(op-0x06), s)
	case op >= 0x0a && op <= 0x0d, op == 0x13, op == 0xfe0e, op == 0x10, op == 0xfe0b, op == 0x26:
		s.pop() // stloc, starg, pop
	case op == 0x0e || op == 0xfe09:
		e.ldarg(in.Arg, s)
	case op == 0x11 || op == 0xfe0c:
		e.ldloc(in.Arg, s)
	case op == 0x0f || op == 0xfe0a || op == 0x12 || op == 0xfe0d: // ldarga, ldloca
		s.push(Shape{})
	case op == 0x14:
		s.push(Shape{Kind: ShNull})
	case op >= 0x15 && op <= 0x1e:
		s.push(Shape{Kind: ShConst, Text: fmt.Sprint(int64(op) - 0x16)})
	case op == 0x1f || op == 0x20 || op == 0x21:
		s.push(Shape{Kind: ShConst, Text: fmt.Sprint(in.Arg)})
	case op == 0x22 || op == 0x23:
		s.push(Shape{Kind: ShConst, Text: "float"})
	case op == 0x25:
		v := s.pop()
		s.v = append(s.v, v, v)
	default:
		return false
	}
	return true
}

// stepControl handles returns, throws, leaves, branches and switch.
func (e *shapeEval) stepControl(in Instr, s *shapeStack) bool {
	op := in.Op
	switch {
	case op == 0x27 || op == 0x29 || op == 0x2a || op == 0x7a || op == 0xdc || op == 0xfe1a || op == 0xfe11:
		// jmp, calli, ret, throw, endfinally, rethrow, endfilter
		s.v = s.v[:0]
		e.dead = op != 0x29
	case op == 0xdd || op == 0xde: // leave
		s.v = s.v[:0]
		e.record(int(in.Arg), s)
		e.dead = true
	case op == 0x2b || op == 0x38:
		e.record(int(in.Arg), s)
		e.dead = true
	case op == 0x2c || op == 0x2d || op == 0x39 || op == 0x3a:
		s.pop()
		e.record(int(in.Arg), s)
	case op >= 0x2e && op <= 0x37 || op >= 0x3b && op <= 0x44:
		s.popN(2)
		e.record(int(in.Arg), s)
	case op == 0x45:
		s.pop()
		for _, t := range in.Targets {
			e.record(t, s)
		}
	default:
		return false
	}
	return true
}

// stepShapeArith handles indirect loads and stores, arithmetic, conversions and same-value casts.
func stepShapeArith(in Instr, s *shapeStack) bool {
	op := in.Op
	switch {
	case op >= 0x46 && op <= 0x50:
		s.unary() // ldind
	case op >= 0x51 && op <= 0x57 || op == 0xdf || op == 0x70 || op == 0x81:
		s.popN(2) // stind, cpobj, stobj
	case op >= 0x58 && op <= 0x64 || op >= 0xd6 && op <= 0xdb || op >= 0xfe01 && op <= 0xfe05:
		s.popN(2)
		s.push(Shape{})
	case op == 0x65 || op == 0x66 || op >= 0x67 && op <= 0x6e || op == 0x76 || op >= 0x82 && op <= 0x8b ||
		op >= 0xb3 && op <= 0xba || op == 0xc2 || op == 0xc3 || op == 0xc6 || op >= 0xd1 && op <= 0xd5 ||
		op == 0xe0 || op == 0x8e || op == 0xfe0f || op == 0xfe1d || op == 0x71 || op == 0x79 || op == 0xa5 || op == 0x75 || op == 0xfe07:
		s.unary() // conversions, ldlen, ldobj, unbox, isinst (may be null), ldvirtftn …
	case op == 0x74 || op == 0x8c:
		// castclass, box: the same value
	default:
		return false
	}
	return true
}

// stepObject handles ldstr, fields, arrays, tokens and the block opcodes.
func (e *shapeEval) stepObject(in Instr, s *shapeStack) bool {
	op := in.Op
	switch {
	case op == 0x72:
		s.push(Shape{Kind: ShString, Text: e.a.UserString(uint32(in.Arg))})
	case op == 0x7b || op == 0x7c:
		obj := s.pop()
		m, _ := e.a.Member(uint32(in.Arg))
		s.push(Shape{Kind: ShField, Member: m.Owner + "::" + m.Name, Base: &obj})
	case op == 0x7d:
		s.popN(2)
	case op == 0x7e || op == 0x7f:
		m, _ := e.a.Member(uint32(in.Arg))
		s.push(Shape{Kind: ShStatic, Member: m.Owner + "::" + m.Name})
	case op == 0x80:
		s.pop()
	case op == 0x8d:
		s.pop()
		s.push(Shape{Kind: ShNew, Member: e.a.TypeToken(uint32(in.Arg)) + "[]"})
	case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3:
		s.popN(2)
		s.push(Shape{})
	case op >= 0x9b && op <= 0xa2 || op == 0xa4 || op == 0xfe17 || op == 0xfe18:
		s.popN(3)
	case op == 0xd0 || op == 0xfe06 || op == 0xfe1c || op == 0xfe00:
		s.push(Shape{}) // ldtoken, ldftn, sizeof, arglist
	case op == 0xfe15:
		s.pop() // initobj
	default:
		return false
	}
	return true
}

// stepCall handles call, callvirt and newobj: the arguments go, the result is ShNew or ShCall.
func (e *shapeEval) stepCall(in Instr, s *shapeStack) bool {
	op := in.Op
	if op != 0x28 && op != 0x6f && op != 0x73 {
		return false
	}
	m, ok := e.a.Member(uint32(in.Arg))
	if !ok {
		s.v = s.v[:0]
		return true
	}
	n := m.Params
	if m.HasThis && op != 0x73 {
		n++
	}
	s.popN(n)
	switch {
	case op == 0x73:
		s.push(Shape{Kind: ShNew, Member: m.Owner})
	case !m.RetVoid:
		s.push(Shape{Kind: ShCall, Member: m.Owner + "::" + m.Name})
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
