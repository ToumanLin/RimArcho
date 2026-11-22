package dotnet

// Comp-list scan (06 §2 "List modifiers"): methods that change a thing's comp list
// (ThingWithComps.AllComps or its comps field), and the lists a method enumerates with
// foreach. A "Collection was modified" error names the loop that threw, never the code that
// changed the list: that code ran inside the loop body and is not on the stack.
//
// The scan follows values on a small symbolic stack: a slot remembers which member produced
// it (AllComps, a field, a property). Locals keep the last value stored in them; there is no
// real control flow (the stack is cleared after an unconditional branch). It is a heuristic,
// not a data-flow analysis: a list reached through a mod's own field or by reflection is missed.

import (
	"fmt"
	"sort"
	"strings"
)

// CompEdit is one method that changes a comp list.
type CompEdit struct {
	Type   string   `json:"t"` // declaring type ('+' for nested types)
	Method string   `json:"m"`
	Ops    []string `json:"o"` // "AllComps.Remove ×2", "GenList.Swap", in first-seen order
	// Tick is the ThingComp tick method this method overrides (CompTick, CompTickRare,
	// CompTickLong, CompTickInterval); "" when it is not one.
	Tick string `json:"k,omitempty"`
}

// CompTickMethods are the ThingComp methods ThingWithComps.Tick and its variants call for
// every comp in the list.
var CompTickMethods = map[string]bool{"CompTick": true, "CompTickRare": true, "CompTickLong": true, "CompTickInterval": true}

// listMutators are List<T> methods that change the list.
var listMutators = map[string]bool{"Add": true, "AddRange": true, "Insert": true, "InsertRange": true, "Remove": true,
	"RemoveAt": true, "RemoveAll": true, "RemoveRange": true, "Clear": true, "Sort": true, "Reverse": true, "set_Item": true}

// genListMutators are Verse.GenList extension methods that reorder or change a list in place.
var genListMutators = map[string]bool{"Swap": true, "Shuffle": true, "SortBy": true, "SortByDescending": true,
	"InsertionSort": true, "RemoveDuplicates": true, "RemoveLast": true}

// UsesCompList reports whether the assembly references ThingWithComps.AllComps or its comps
// field at all (a cheap filter before reading method bodies).
func (a *Assembly) UsesCompList() bool {
	for _, mr := range a.members {
		switch a.str(mr.Name) {
		case "get_AllComps", "comps":
			return true
		}
	}
	return false
}

// CompEdits lists the methods that change a comp list, tick overrides first.
func (a *Assembly) CompEdits() []CompEdit {
	if !a.UsesCompList() {
		return nil
	}
	var out []CompEdit
	var stack []slot
	for row := uint32(1); int(row) <= len(a.methods); row++ {
		r := a.scanBody(row, &stack)
		if len(r.ops) == 0 {
			continue
		}
		e := CompEdit{Type: a.MethodDeclType(row), Method: a.MethodName(row)}
		for _, op := range r.order {
			if n := r.ops[op]; n > 1 {
				e.Ops = append(e.Ops, fmt.Sprintf("%s ×%d", op, n))
			} else {
				e.Ops = append(e.Ops, op)
			}
		}
		if a.overridesCompTick(row) {
			e.Tick = e.Method
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return (out[i].Tick != "") && (out[j].Tick == "") })
	return out
}

// Enumerated lists the members whose value a method enumerates with foreach (GetEnumerator),
// as "Verse.ThingWithComps.AllComps", for the method and the compiler-generated types an
// iterator or lambda of it lives in. typ uses '+' or '.' for nesting.
func (a *Assembly) Enumerated(typ, method string) []string {
	row := a.TypeRow(typ)
	for i := len(typ) - 1; row == 0 && i > 0; i-- {
		if typ[i] == '.' {
			typ = typ[:i] + "+" + typ[i+1:]
			row = a.TypeRow(typ)
		}
	}
	if row == 0 {
		return nil
	}
	var rows []uint32
	for _, m := range a.Methods(row) {
		if m.Name == method {
			rows = append(rows, m.Row)
		}
	}
	// Iterator state machines and closures: Outer+<Method>d__3.MoveNext, Outer+<>c.<Method>b__2_0.
	for t := uint32(1); int(t) <= len(a.typeDefs); t++ {
		if a.nestedIn[t] != row {
			continue
		}
		n := a.str(a.typeDefs[t-1].TypeName)
		for _, m := range a.Methods(t) {
			if strings.HasPrefix(n, "<"+method+">") && m.Name == "MoveNext" || strings.HasPrefix(m.Name, "<"+method+">") {
				rows = append(rows, m.Row)
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	var stack []slot
	for _, r := range rows {
		for _, s := range a.scanBody(r, &stack).enums {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// IsCompList reports whether an Enumerated member is a thing's comp list.
func IsCompList(member string) bool {
	return member == "Verse.ThingWithComps.AllComps" || member == "Verse.ThingWithComps.comps"
}

// overridesCompTick: a virtual instance method named like a ThingComp tick method in a type
// whose base chain reaches Verse.ThingComp, or leaves the assembly at a type other than
// System.Object (a framework's comp base class).
func (a *Assembly) overridesCompTick(row uint32) bool {
	md := a.methods[row-1]
	if !CompTickMethods[a.str(md.Name)] || md.Flags&0x0040 == 0 || md.Flags&0x0010 != 0 { // virtual, not static
		return false
	}
	t := a.ilData().methOwner[row-1]
	for depth := 0; t != 0 && depth < 16; depth++ {
		ext := a.typeDefs[t-1].Extends
		switch ext & 3 {
		case 0:
			t = ext >> 2
			continue
		case 1:
			base := a.typeRefName(ext >> 2)
			return base == "Verse.ThingComp" || !strings.HasPrefix(base, "System.")
		case 2:
			base := a.typeSpecName(ext >> 2)
			return base != "" && !strings.HasPrefix(base, "System.")
		}
		return false
	}
	return false
}

type slot struct{ src string } // the member that produced the value ("" unknown)

type bodyScan struct {
	ops   map[string]int // comp-list operations → count
	order []string
	enums []string // members enumerated with GetEnumerator
}

// compSource names a comp list produced by a member, "" if it is not one.
func compSource(m Member) string {
	switch {
	case !m.Field && m.Name == "get_AllComps" && !strings.HasPrefix(m.Owner, "RimWorld.Planet.") && !strings.Contains(m.Owner, "WorldObject"):
		// WorldObject.AllComps is the world object's own list, not a thing's
		return "AllComps"
	case m.Field && m.Name == "comps" && (m.Owner == "Verse.ThingWithComps" || m.Owner == ""):
		return "comps"
	}
	return ""
}

func memberSrc(m Member) string {
	name := strings.TrimPrefix(m.Name, "get_")
	if cs := compSource(m); cs != "" {
		return "Verse.ThingWithComps." + cs
	}
	if m.Owner == "" {
		return name
	}
	return strings.ReplaceAll(m.Owner, "+", ".") + "." + name
}

// compScan is the per-method state of scanBody except the stack. The stack and the result
// are a separate slotStack so that these maps can stay off the heap.
type compScan struct {
	a      *Assembly
	snap   map[int][]slot // the first stack recorded at each branch target
	locals map[int64]slot
	dead   bool // after an unconditional branch, ret or throw
}

// slotStack is scanBody's stack and its result.
type slotStack struct {
	v []slot
	r bodyScan
}

// scanBody scans MethodDef row's body for comp-list changes and enumerated members. stack is
// a buffer for the evaluation stack, reused from call to call.
func (a *Assembly) scanBody(row uint32, stack *[]slot) bodyScan {
	r := bodyScan{ops: map[string]int{}}
	code := a.Body(row)
	if len(code) == 0 {
		return r
	}
	ins := DecodeIL(code)
	targets := map[int]bool{}
	addBranchTargets(targets, ins)
	c := compScan{a: a, snap: map[int][]slot{}, locals: map[int64]slot{}}
	s := slotStack{v: (*stack)[:0], r: r}
	for _, in := range ins {
		if targets[in.Off] && c.dead {
			s.v = append(s.v[:0], c.snap[in.Off]...)
		} else if c.dead {
			s.v = s.v[:0]
		}
		c.dead = false
		c.step(in, &s)
	}
	clear(s.v[:cap(s.v)])
	*stack = s.v[:0]
	return s.r
}

// pop takes the top of the stack; an empty stack gives an unknown slot.
func (s *slotStack) pop() slot {
	if len(s.v) == 0 {
		return slot{}
	}
	v := s.v[len(s.v)-1]
	s.v = s.v[:len(s.v)-1]
	return v
}

// popInto fills out with the top len(out) slots, deepest first.
func (s *slotStack) popInto(out []slot) {
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = s.pop()
	}
}

// drop discards the top n slots.
func (s *slotStack) drop(n int) {
	for i := 0; i < n; i++ {
		s.pop()
	}
}

// push puts v on the stack.
func (s *slotStack) push(v slot) { s.v = append(s.v, v) }

// op1 counts one comp-list operation.
func (s *slotStack) op1(label string) {
	if s.r.ops[label] == 0 {
		s.r.order = append(s.r.order, label)
	}
	s.r.ops[label]++
}

// record saves the stack for branch target t unless one is recorded there already.
func (c *compScan) record(t int, s *slotStack) {
	if _, ok := c.snap[t]; !ok {
		c.snap[t] = append([]slot(nil), s.v...)
	}
}

// step applies one instruction. The handlers keep the opcode cases in their original order;
// anything else has no stack effect.
func (c *compScan) step(in Instr, s *slotStack) {
	_ = c.stepLoad(in, s) || c.stepControl(in, s) || c.stepArith(in, s) || c.stepObject(in, s) || c.stepCall(in, s)
}

// stepLoad handles arguments, locals, constants, dup and pop.
func (c *compScan) stepLoad(in Instr, s *slotStack) bool {
	op := in.Op
	switch {
	case op >= 0x02 && op <= 0x05, op == 0x0e, op == 0x0f, op == 0xfe09, op == 0xfe0a: // ldarg(a)
		s.push(slot{})
	case op >= 0x06 && op <= 0x09:
		s.push(c.locals[int64(op-0x06)])
	case op == 0x11 || op == 0x12 || op == 0xfe0c || op == 0xfe0d:
		s.push(c.locals[in.Arg])
	case op >= 0x0a && op <= 0x0d:
		c.locals[int64(op-0x0a)] = s.pop()
	case op == 0x13 || op == 0xfe0e:
		c.locals[in.Arg] = s.pop()
	case op == 0x10 || op == 0xfe0b, op == 0x26: // starg, pop
		s.pop()
	case op == 0x14, op >= 0x15 && op <= 0x23, op == 0x72, op == 0xd0, op == 0xfe06, op == 0xfe1c, op == 0xfe00:
		s.push(slot{}) // constants, ldstr, ldtoken, ldftn, sizeof, arglist
	case op == 0x25: // dup
		v := s.pop()
		s.push(v)
		s.push(v)
	default:
		return false
	}
	return true
}

// stepControl handles returns, throws, leaves, branches and switch.
func (c *compScan) stepControl(in Instr, s *slotStack) bool {
	op := in.Op
	switch {
	case op == 0x27 || op == 0x29 || op == 0x2a || op == 0xdc || op == 0xdd || op == 0xde || op == 0x7a || op == 0xfe1a || op == 0xfe11:
		s.v = s.v[:0]
		if op == 0xdd || op == 0xde {
			c.record(int(in.Arg), s)
		}
		c.dead = true
	case op == 0x2b || op == 0x38:
		c.record(int(in.Arg), s)
		c.dead = true
	case op == 0x2c || op == 0x2d || op == 0x39 || op == 0x3a:
		s.pop()
		c.record(int(in.Arg), s)
	case op >= 0x2e && op <= 0x37 || op >= 0x3b && op <= 0x44:
		s.drop(2)
		c.record(int(in.Arg), s)
	case op == 0x45:
		s.pop()
		for _, t := range in.Targets {
			c.record(t, s)
		}
	default:
		return false
	}
	return true
}

// stepArith handles indirect loads and stores, arithmetic, conversions and same-value casts.
func (c *compScan) stepArith(in Instr, s *slotStack) bool {
	op := in.Op
	switch {
	case op >= 0x46 && op <= 0x50: // ldind
		s.pop()
		s.push(slot{})
	case op >= 0x51 && op <= 0x57 || op == 0xdf: // stind
		s.drop(2)
	case op >= 0x58 && op <= 0x64 || op >= 0xd6 && op <= 0xdb || op >= 0xfe01 && op <= 0xfe05:
		s.drop(2)
		s.push(slot{})
	case op == 0x65 || op == 0x66 || op >= 0x67 && op <= 0x6e || op == 0x76 || op >= 0x82 && op <= 0x8b ||
		op >= 0xb3 && op <= 0xba || op == 0xc3 || op >= 0xd1 && op <= 0xd5 || op == 0xe0 || op == 0x8e || op == 0xfe0f ||
		op == 0x8d || op == 0xc2 || op == 0xc6 || op == 0xfe1d || op == 0xfe07:
		s.pop()
		s.push(slot{})
	case op == 0x74 || op == 0x75 || op == 0x8c || op == 0x79 || op == 0xa5 || op == 0x71:
		// castclass, isinst, box, unbox(.any), ldobj: the same value
	default:
		return false
	}
	return true
}

// stepObject handles cpobj, stobj, fields, arrays and the block opcodes.
func (c *compScan) stepObject(in Instr, s *slotStack) bool {
	op := in.Op
	switch {
	case op == 0x70 || op == 0x81:
		s.drop(2)
	case op == 0x7b || op == 0x7c: // ldfld(a)
		s.pop()
		m, _ := c.a.Member(uint32(in.Arg))
		s.push(slot{memberSrc(m)})
	case op == 0x7e || op == 0x7f: // ldsfld(a)
		m, _ := c.a.Member(uint32(in.Arg))
		s.push(slot{memberSrc(m)})
	case op == 0x7d:
		s.drop(2)
	case op == 0x80:
		s.pop()
	case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3:
		s.drop(2)
		s.push(slot{})
	case op >= 0x9b && op <= 0xa2 || op == 0xa4:
		s.drop(3)
	case op == 0xfe15:
		s.pop()
	case op == 0xfe17 || op == 0xfe18:
		s.drop(3)
	default:
		return false
	}
	return true
}

// stepCall handles call, callvirt and newobj: enumerated members, comp-list changes, and the
// member a getter's result comes from.
func (c *compScan) stepCall(in Instr, s *slotStack) bool {
	op := in.Op
	if op != 0x28 && op != 0x6f && op != 0x73 {
		return false
	}
	m, ok := c.a.Member(uint32(in.Arg))
	if !ok {
		s.v = s.v[:0]
		return true
	}
	n := m.Params
	if m.HasThis && op != 0x73 {
		n++
	}
	args := make([]slot, n)
	s.popInto(args)
	var res slot
	if op != 0x73 {
		res = c.callEffect(m, args, s)
	}
	if op == 0x73 || !m.RetVoid {
		s.push(res)
	}
	return true
}

// callEffect records what a call (not newobj) does to comp lists and enumerations, and returns
// the slot of its result.
func (c *compScan) callEffect(m Member, args []slot, s *slotStack) slot {
	switch {
	case m.HasThis && len(args) > 0 && m.Name == "GetEnumerator":
		if args[0].src != "" {
			s.r.enums = append(s.r.enums, args[0].src)
		}
	case m.HasThis && len(args) > 0 && listMutators[m.Name] && strings.HasPrefix(m.Owner, "System.Collections.Generic.List`1"):
		if IsCompList(args[0].src) {
			label := args[0].src[strings.LastIndexByte(args[0].src, '.')+1:]
			if m.Name == "set_Item" {
				s.op1(label + "[i] =")
			} else {
				s.op1(label + "." + m.Name)
			}
		}
	case !m.HasThis && len(args) > 0 && m.Owner == "Verse.GenList" && genListMutators[m.Name]:
		if IsCompList(args[0].src) {
			s.op1("GenList." + m.Name)
		}
	case !m.Field && (strings.HasPrefix(m.Name, "get_") && m.Params == 0 || compSource(m) != ""):
		return slot{memberSrc(m)}
	}
	return slot{}
}

// CompClassesSet maps CompProperties subclasses of the assembly to the comp class their
// constructor assigns (compClass = typeof(T)): XML that attaches the comp may name only the
// properties class.
func (a *Assembly) CompClassesSet() map[string]string {
	out := map[string]string{}
	for row := uint32(1); int(row) <= len(a.methods); row++ {
		if a.str(a.methods[row-1].Name) != ".ctor" {
			continue
		}
		last := ""
		for _, in := range DecodeIL(a.Body(row)) {
			switch in.Op {
			case 0xd0: // ldtoken
				last = a.TypeToken(uint32(in.Arg))
			case 0x7d: // stfld
				if m, ok := a.Member(uint32(in.Arg)); ok && m.Name == "compClass" && last != "" {
					out[a.MethodDeclType(row)] = last
				}
				last = ""
			}
		}
	}
	return out
}
