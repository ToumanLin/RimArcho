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
	for row := uint32(1); int(row) <= len(a.methods); row++ {
		r := a.scanBody(row)
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
	for _, r := range rows {
		for _, s := range a.scanBody(r).enums {
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

func (a *Assembly) scanBody(row uint32) bodyScan {
	r := bodyScan{ops: map[string]int{}}
	code := a.Body(row)
	if len(code) == 0 {
		return r
	}
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
	var st []slot
	snap := map[int][]slot{}
	locals := map[int64]slot{}
	dead := false
	pop := func() slot {
		if len(st) == 0 {
			return slot{}
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	popN := func(n int) []slot {
		out := make([]slot, n)
		for i := n - 1; i >= 0; i-- {
			out[i] = pop()
		}
		return out
	}
	push := func(v slot) { st = append(st, v) }
	record := func(t int) {
		if _, ok := snap[t]; !ok {
			snap[t] = append([]slot(nil), st...)
		}
	}
	op1 := func(label string) {
		if r.ops[label] == 0 {
			r.order = append(r.order, label)
		}
		r.ops[label]++
	}
	for _, in := range ins {
		if targets[in.Off] && dead {
			st = append(st[:0], snap[in.Off]...)
		} else if dead {
			st = st[:0]
		}
		dead = false
		op := in.Op
		switch {
		case op >= 0x02 && op <= 0x05, op == 0x0e, op == 0x0f, op == 0xfe09, op == 0xfe0a: // ldarg(a)
			push(slot{})
		case op >= 0x06 && op <= 0x09:
			push(locals[int64(op-0x06)])
		case op == 0x11 || op == 0x12 || op == 0xfe0c || op == 0xfe0d:
			push(locals[in.Arg])
		case op >= 0x0a && op <= 0x0d:
			locals[int64(op-0x0a)] = pop()
		case op == 0x13 || op == 0xfe0e:
			locals[in.Arg] = pop()
		case op == 0x10 || op == 0xfe0b, op == 0x26: // starg, pop
			pop()
		case op == 0x14, op >= 0x15 && op <= 0x23, op == 0x72, op == 0xd0, op == 0xfe06, op == 0xfe1c, op == 0xfe00:
			push(slot{}) // constants, ldstr, ldtoken, ldftn, sizeof, arglist
		case op == 0x25: // dup
			v := pop()
			push(v)
			push(v)
		case op == 0x27 || op == 0x29 || op == 0x2a || op == 0xdc || op == 0xdd || op == 0xde || op == 0x7a || op == 0xfe1a || op == 0xfe11:
			st = st[:0]
			if op == 0xdd || op == 0xde {
				record(int(in.Arg))
			}
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
		case op >= 0x46 && op <= 0x50: // ldind
			pop()
			push(slot{})
		case op >= 0x51 && op <= 0x57 || op == 0xdf: // stind
			popN(2)
		case op >= 0x58 && op <= 0x64 || op >= 0xd6 && op <= 0xdb || op >= 0xfe01 && op <= 0xfe05:
			popN(2)
			push(slot{})
		case op == 0x65 || op == 0x66 || op >= 0x67 && op <= 0x6e || op == 0x76 || op >= 0x82 && op <= 0x8b ||
			op >= 0xb3 && op <= 0xba || op == 0xc3 || op >= 0xd1 && op <= 0xd5 || op == 0xe0 || op == 0x8e || op == 0xfe0f ||
			op == 0x8d || op == 0xc2 || op == 0xc6 || op == 0xfe1d || op == 0xfe07:
			pop()
			push(slot{})
		case op == 0x74 || op == 0x75 || op == 0x8c || op == 0x79 || op == 0xa5 || op == 0x71:
			// castclass, isinst, box, unbox(.any), ldobj: the same value
		case op == 0x70 || op == 0x81:
			popN(2)
		case op == 0x7b || op == 0x7c: // ldfld(a)
			pop()
			m, _ := a.Member(uint32(in.Arg))
			push(slot{memberSrc(m)})
		case op == 0x7e || op == 0x7f: // ldsfld(a)
			m, _ := a.Member(uint32(in.Arg))
			push(slot{memberSrc(m)})
		case op == 0x7d:
			popN(2)
		case op == 0x80:
			pop()
		case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3:
			popN(2)
			push(slot{})
		case op >= 0x9b && op <= 0xa2 || op == 0xa4:
			popN(3)
		case op == 0xfe15:
			pop()
		case op == 0xfe17 || op == 0xfe18:
			popN(3)
		case op == 0x28 || op == 0x6f || op == 0x73: // call, callvirt, newobj
			m, ok := a.Member(uint32(in.Arg))
			if !ok {
				st = st[:0]
				continue
			}
			n := m.Params
			if m.HasThis && op != 0x73 {
				n++
			}
			args := popN(n)
			var res slot
			switch {
			case op == 0x73:
			case m.HasThis && len(args) > 0 && m.Name == "GetEnumerator":
				if args[0].src != "" {
					r.enums = append(r.enums, args[0].src)
				}
			case m.HasThis && len(args) > 0 && listMutators[m.Name] && strings.HasPrefix(m.Owner, "System.Collections.Generic.List`1"):
				if IsCompList(args[0].src) {
					label := args[0].src[strings.LastIndexByte(args[0].src, '.')+1:]
					if m.Name == "set_Item" {
						op1(label + "[i] =")
					} else {
						op1(label + "." + m.Name)
					}
				}
			case !m.HasThis && len(args) > 0 && m.Owner == "Verse.GenList" && genListMutators[m.Name]:
				if IsCompList(args[0].src) {
					op1("GenList." + m.Name)
				}
			case !m.Field && (strings.HasPrefix(m.Name, "get_") && m.Params == 0 || compSource(m) != ""):
				res = slot{memberSrc(m)}
			}
			if op == 0x73 || !m.RetVoid {
				push(res)
			}
		}
	}
	return r
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
