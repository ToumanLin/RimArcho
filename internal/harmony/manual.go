package harmony

// Manual patching: targets of Harmony.Patch(original, prefix, postfix, transpiler, finalizer)
// call sites, found by a small abstract interpretation of each method's IL.
//
// Values are symbolic: string and int constants, types (ldtoken / typeof / AccessTools.TypeByName),
// methods (ldtoken, AccessTools.Method, Type.GetMethod, property getters/setters, constructors,
// delegates), HarmonyMethods, arrays and lists of those, and the method's own parameters. Locals,
// fields and array elements follow the last store (no real control flow: the stack is cleared at
// branch targets). A method that calls Harmony.Patch with values built from its parameters is a
// helper; its calls are re-evaluated at each call site in the assembly (a few rounds, so helpers of
// helpers work too). A foreach over a known array or list patches every element.
//
// What stays unknown gets a reason (computed name, collection, parameter, field, GetType(), call …)
// so the remaining gap can be explained.

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/dotnet"
)

type vk uint8

const (
	vUnk    vk = iota
	vNull      // null / default argument
	vStr       // string constant
	vInt       // integer constant
	vType      // a System.Type
	vMethod    // a MethodBase
	vHM        // a HarmonyMethod
	vArr       // an array or List<T> with known elements
	vParam     // the method's own parameter n (0 = this for instance methods)
	vProp      // a PropertyInfo
	vAlt       // one of several values (foreach over a known collection)
	vEnum      // an enumerator over a collection
	vThis      // "this" of an instance method nobody in the assembly calls: an instance of type s
	vTypeOf    // a.GetType() of a parameter or vThis
	vSearch    // the methods of type a, filtered by name b when a lambda names it (LINQ)
)

type val struct {
	k       vk
	s       string // vStr text, vType name, vUnk reason, vMethod kind ("", get, set, ctor)
	n       int64  // vInt, vParam
	a, b, c *val   // vMethod: type, name, args; vHM: method; vProp: type, name; vEnum: collection
	el      []*val // vArr, vAlt
}

func unk(reason string) *val { return &val{k: vUnk, s: reason} }
func vstr(s string) *val     { return &val{k: vStr, s: s} }
func vtyp(s string) *val     { return &val{k: vType, s: s} }

var null = &val{k: vNull}

func (v *val) key() string {
	var sb strings.Builder
	v.write(&sb, 0)
	return sb.String()
}

func (v *val) write(sb *strings.Builder, d int) {
	if v == nil {
		sb.WriteString("nil")
		return
	}
	if d > 12 {
		sb.WriteString("…")
		return
	}
	sb.WriteByte(byte('a' + v.k))
	sb.WriteString(v.s)
	if v.k == vInt || v.k == vParam {
		sb.WriteString(":" + itoa(v.n))
	}
	for _, x := range []*val{v.a, v.b, v.c} {
		if x != nil {
			sb.WriteByte('(')
			x.write(sb, d+1)
			sb.WriteByte(')')
		}
	}
	if len(v.el) > 0 {
		sb.WriteByte('[')
		for _, x := range v.el {
			x.write(sb, d+1)
			sb.WriteByte(',')
		}
		sb.WriteByte(']')
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func hasParam(v *val, d int) bool {
	if v == nil || d > 12 {
		return false
	}
	if v.k == vParam {
		return true
	}
	if hasParam(v.a, d+1) || hasParam(v.b, d+1) || hasParam(v.c, d+1) {
		return true
	}
	for _, x := range v.el {
		if hasParam(x, d+1) {
			return true
		}
	}
	return false
}

// subst replaces parameter references with call-site arguments.
func subst(v *val, args []*val, d int) *val {
	if v == nil || !hasParam(v, 0) {
		return v
	}
	if d > 12 {
		return unk("parameter")
	}
	if v.k == vParam {
		if int(v.n) < len(args) && args[v.n] != nil {
			return args[v.n]
		}
		return unk("parameter")
	}
	r := *v
	r.a, r.b, r.c = subst(v.a, args, d+1), subst(v.b, args, d+1), subst(v.c, args, d+1)
	if len(v.el) > 0 {
		r.el = make([]*val, len(v.el))
		for i, x := range v.el {
			r.el[i] = subst(x, args, d+1)
		}
	}
	if r.k == vTypeOf && r.a != nil {
		switch r.a.k {
		case vThis:
			return vtyp(r.a.s)
		case vUnk:
			return unk("GetType()")
		}
	}
	return &r
}

// pcall is one Harmony.Patch call: the original and the HarmonyMethods by kind position.
type pcall struct {
	orig *val
	hms  []*val
	kind []string
}

func (p pcall) subst(args []*val) pcall {
	q := pcall{orig: subst(p.orig, args, 0), kind: p.kind}
	for _, h := range p.hms {
		q.hms = append(q.hms, subst(h, args, 0))
	}
	return q
}

func (p pcall) hasParam() bool {
	if hasParam(p.orig, 0) {
		return true
	}
	for _, h := range p.hms {
		if hasParam(h, 0) {
			return true
		}
	}
	return false
}

type summary struct {
	ret   *val
	calls []pcall
}

type scanner struct {
	a      *dotnet.Assembly
	sums   map[uint32]*summary // previous round
	next   map[uint32]*summary // this round
	fields map[string]*val     // field values of the previous round ("" reason = several stores)
	fnext  map[string][]*val
	called map[uint32]bool // helpers called from this assembly (this round)
	final  bool
	out    *Assembly
	seen   map[string]bool
	stack  []*val // run's stack buffer, reused from method to method
}

var patchKinds5 = []string{Prefix, Postfix, Transpiler, Finalizer, ILManipulator}

const rounds = 5

// scanManual fills out.Patches / ManualCalls / Unresolved from the assembly's IL.
func scanManual(a *dotnet.Assembly, out *Assembly) {
	s := &scanner{a: a, sums: map[uint32]*summary{}, fields: map[string]*val{}, out: out, seen: map[string]bool{}}
	for _, p := range out.Patches {
		s.seen[p.Kind+"|"+p.PatchType+"."+p.PatchMethod+"|"+p.Target()] = true
	}
	n := uint32(a.MethodCount())
	for r := 0; r < rounds; r++ {
		s.final = r == rounds-1
		s.next = map[uint32]*summary{}
		s.fnext = map[string][]*val{}
		s.called = map[uint32]bool{}
		for m := uint32(1); m <= n; m++ {
			s.run(m)
		}
		s.sums = s.next
		s.fields = map[string]*val{}
		for k, vs := range s.fnext {
			v := vs[0]
			for _, x := range vs[1:] {
				if x.key() != v.key() {
					v = unk("field")
					break
				}
			}
			s.fields[k] = v
		}
	}
	// Helpers nobody in this assembly calls: their parameters stay unknown.
	rows := make([]uint32, 0, len(s.sums))
	for m := range s.sums {
		rows = append(rows, m)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i] < rows[j] })
	for _, m := range rows {
		sm := s.sums[m]
		if s.called[m] {
			continue
		}
		var args []*val
		if mb, ok := a.Member(0x06<<24 | m); ok && mb.HasThis {
			args = []*val{{k: vThis, s: mb.Owner}}
		}
		for _, pc := range sm.calls {
			s.emit(pc.subst(args))
		}
	}
}

func (s *scanner) sum(m uint32) *summary {
	x := s.next[m]
	if x == nil {
		x = &summary{}
		s.next[m] = x
	}
	return x
}

func (s *scanner) setField(k string, v *val) {
	if hasParam(v, 0) {
		v = unk("field")
	}
	s.fnext[k] = append(s.fnext[k], v)
}

// frame is the evaluation state of one method body in run, except the stack. The stack is a
// separate valStack so that these maps can stay off the heap.
type frame struct {
	s      *scanner
	row    uint32
	snap   map[int][]*val // stacks recorded at branch targets
	dead   bool           // after an unconditional branch, ret or throw
	stores map[int64]int  // stores per local
	locals map[int64]*val
	args   map[int64]*val
	localF map[string]*val // fields stored in this method
}

// valStack is run's evaluation stack.
type valStack struct{ v []*val }

// record saves the stack for branch target t, merged with what was recorded there before.
func (f *frame) record(t int, s *valStack) {
	cp := append([]*val(nil), s.v...)
	if old, ok := f.snap[t]; ok {
		cp = mergeStacks(old, cp)
	}
	f.snap[t] = cp
}

// pop takes the top of the stack; an empty stack gives an unknown value.
func (s *valStack) pop() *val {
	if len(s.v) == 0 {
		return unk("stack")
	}
	v := s.v[len(s.v)-1]
	s.v = s.v[:len(s.v)-1]
	return v
}

// push puts v on the stack.
func (s *valStack) push(v *val) { s.v = append(s.v, v) }

// popN takes the top n values, deepest first.
func (s *valStack) popN(n int) []*val {
	r := make([]*val, n)
	for i := n - 1; i >= 0; i-- {
		r[i] = s.pop()
	}
	return r
}

// drop discards the top n values.
func (s *valStack) drop(n int) {
	for i := 0; i < n; i++ {
		s.pop()
	}
}

// ldarg is argument i: its last store, else the parameter itself.
func (f *frame) ldarg(i int64) *val {
	if v := f.args[i]; v != nil {
		return v
	}
	return &val{k: vParam, n: i}
}

// ldloc is local i's last store; an int in a local stored more than once is no fixed index.
func (f *frame) ldloc(i int64) *val {
	v := f.locals[i]
	switch {
	case v == nil:
		return unk("local")
	case v.k == vInt && f.stores[i] > 1:
		return unk("index")
	}
	return v
}

// field is a field token's value: stored in this method, else the previous round's.
func (f *frame) field(tok uint32) *val {
	m, ok := f.s.a.Member(tok)
	if !ok {
		return unk("field")
	}
	k := m.Owner + "::" + m.Name
	if m.Owner == "System.Type" && m.Name == "EmptyTypes" {
		return &val{k: vArr}
	}
	if v := f.localF[k]; v != nil {
		return v
	}
	if v := f.s.fields[k]; v != nil {
		return v
	}
	return unk("field")
}

// storeField records v as the value of the field token's field.
func (f *frame) storeField(tok uint32, v *val) {
	if m, ok := f.s.a.Member(tok); ok {
		k := m.Owner + "::" + m.Name
		f.localF[k] = v
		f.s.setField(k, v)
	}
}

// addBranchTargets adds the offsets some branch or switch jumps to. The caller makes the map,
// so that a small one can stay off the heap.
func addBranchTargets(targets map[int]bool, ins []dotnet.Instr) {
	for _, in := range ins {
		if dotnet.IsBranch(in.Op) {
			targets[int(in.Arg)] = true
		}
		for _, t := range in.Targets {
			targets[t] = true
		}
	}
}

// addLocalStores counts the stores to each local. Locals stored more than once (loop counters):
// their int value is not a fixed index.
func addLocalStores(stores map[int64]int, ins []dotnet.Instr) {
	for _, in := range ins {
		switch op := in.Op; {
		case op >= 0x0a && op <= 0x0d:
			stores[int64(op-0x0a)]++
		case op == 0x13 || op == 0xfe0e:
			stores[in.Arg]++
		}
	}
}

// run evaluates MethodDef row's IL once, filling its summary and the field stores.
func (s *scanner) run(row uint32) {
	code := s.a.Body(row)
	if len(code) == 0 {
		return
	}
	ins := dotnet.DecodeIL(code)
	targets, stores := map[int]bool{}, map[int64]int{}
	addBranchTargets(targets, ins)
	addLocalStores(stores, ins)
	f := frame{s: s, row: row, snap: map[int][]*val{}, stores: stores,
		locals: map[int64]*val{}, args: map[int64]*val{}, localF: map[string]*val{}}
	st := valStack{v: s.stack[:0]}
	for _, in := range ins {
		f.join(in.Off, targets[in.Off], &st)
		f.dead = false
		f.step(in, &st)
	}
	clear(st.v[:cap(st.v)])
	s.stack = st.v[:0]
}

// join sets the stack at off: the fall-through stack merged with the stacks branched here.
func (f *frame) join(off int, target bool, s *valStack) {
	if target {
		sn, ok := f.snap[off]
		switch {
		case f.dead && ok:
			s.v = append(s.v[:0], sn...)
		case f.dead:
			s.v = s.v[:0]
		case ok:
			s.v = mergeStacks(s.v, sn)
		}
	} else if f.dead {
		s.v = s.v[:0]
	}
}

// step applies one instruction. The handlers keep the opcode cases in their original order;
// prefixes (unaligned, volatile, tail, constrained, readonly, no.) and anything else have no
// stack effect.
func (f *frame) step(in dotnet.Instr, s *valStack) {
	_ = f.stepLoad(in, s) || f.stepControl(in, s) || f.stepArith(in, s) || f.stepObject(in, s) || f.stepMisc(in, s) || f.stepCall(in, s)
}

// stepLoad handles nop, arguments, locals, constants, dup and pop.
func (f *frame) stepLoad(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	switch {
	case op == 0x00: // nop
	case op >= 0x02 && op <= 0x05: // ldarg.0-3
		s.push(f.ldarg(int64(op - 0x02)))
	case op >= 0x06 && op <= 0x09: // ldloc.0-3
		s.push(f.ldloc(int64(op - 0x06)))
	case op >= 0x0a && op <= 0x0d: // stloc.0-3
		f.locals[int64(op-0x0a)] = s.pop()
	case op == 0x0e || op == 0x0f || op == 0xfe09 || op == 0xfe0a: // ldarg(a)
		s.push(f.ldarg(in.Arg))
	case op == 0x10 || op == 0xfe0b: // starg
		f.args[in.Arg] = s.pop()
	case op == 0x11 || op == 0x12 || op == 0xfe0c || op == 0xfe0d: // ldloc(a)
		s.push(f.ldloc(in.Arg))
	case op == 0x13 || op == 0xfe0e: // stloc
		f.locals[in.Arg] = s.pop()
	case op == 0x14:
		s.push(null)
	case op >= 0x15 && op <= 0x1e: // ldc.i4.m1 … ldc.i4.8
		s.push(&val{k: vInt, n: int64(op) - 0x16})
	case op == 0x1f || op == 0x20 || op == 0x21:
		s.push(&val{k: vInt, n: in.Arg})
	case op == 0x22 || op == 0x23:
		s.push(unk("float"))
	case op == 0x25:
		v := s.pop()
		s.push(v)
		s.push(v)
	case op == 0x26:
		s.pop()
	default:
		return false
	}
	return true
}

// stepControl handles returns, throws, leaves, branches and switch.
func (f *frame) stepControl(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	switch {
	case op == 0x27 || op == 0x29 || op == 0x2a || op == 0xdc || op == 0xdd || op == 0xde || op == 0x7a || op == 0xfe1a || op == 0xfe11:
		// jmp, calli, ret, endfinally, leave, throw, rethrow, endfilter
		if op == 0x2a && len(s.v) > 0 {
			f.recordReturn(s.v[len(s.v)-1])
		}
		s.v = s.v[:0]
		if op == 0xdd || op == 0xde {
			f.record(int(in.Arg), s)
		}
		f.dead = true
	case op == 0x2b || op == 0x38: // br
		f.record(int(in.Arg), s)
		f.dead = true
	case op == 0x2c || op == 0x2d || op == 0x39 || op == 0x3a: // brfalse / brtrue
		s.pop()
		f.record(int(in.Arg), s)
	case op >= 0x2e && op <= 0x37 || op >= 0x3b && op <= 0x44:
		s.drop(2)
		f.record(int(in.Arg), s)
	case op == 0x45:
		s.pop()
		for _, t := range in.Targets {
			f.record(t, s)
		}
	default:
		return false
	}
	return true
}

// recordReturn adds a returned value to the method's summary.
func (f *frame) recordReturn(v *val) {
	sm := f.s.sum(f.row)
	if sm.ret == nil {
		sm.ret = v
	} else if sm.ret.key() != v.key() {
		sm.ret = alt(sm.ret, v)
	}
}

// stepArith handles indirect loads and stores, arithmetic, conversions and same-value casts.
func (f *frame) stepArith(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	switch {
	case op >= 0x46 && op <= 0x50: // ldind.*
		s.pop()
		s.push(unk("pointer"))
	case op == 0x51 || op >= 0x52 && op <= 0x57 || op == 0xdf: // stind.*
		s.drop(2)
	case op >= 0x58 && op <= 0x64 || op >= 0xd6 && op <= 0xdb || op >= 0xfe01 && op <= 0xfe05: // binary
		s.drop(2)
		s.push(unk("arithmetic"))
	case op == 0x65 || op == 0x66 || op >= 0x67 && op <= 0x6e || op == 0x76 || op >= 0x82 && op <= 0x8b ||
		op >= 0xb3 && op <= 0xba || op == 0xc3 || op >= 0xd1 && op <= 0xd5 || op == 0xe0 || op == 0x8e || op == 0xfe0f:
		s.pop()
		s.push(unk("arithmetic"))
	case op == 0x74 || op == 0x75 || op == 0x8c || op == 0x79 || op == 0xa5 || op == 0x71:
		// castclass, isinst, box, unbox, unbox.any, ldobj: same value
	default:
		return false
	}
	return true
}

// stepObject handles cpobj, stobj, ldstr and the field loads and stores.
func (f *frame) stepObject(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	switch {
	case op == 0x70 || op == 0x81: // cpobj, stobj
		s.drop(2)
	case op == 0x72:
		s.push(vstr(f.s.a.UserString(uint32(in.Arg))))
	case op == 0x7b || op == 0x7c: // ldfld(a)
		s.pop()
		s.push(f.field(uint32(in.Arg)))
	case op == 0x7d: // stfld
		v := s.pop()
		s.pop()
		f.storeField(uint32(in.Arg), v)
	case op == 0x7e || op == 0x7f:
		s.push(f.field(uint32(in.Arg)))
	case op == 0x80:
		f.storeField(uint32(in.Arg), s.pop())
	default:
		return false
	}
	return true
}

// stepMisc handles arrays, typed references, tokens, method pointers and the block opcodes.
func (f *frame) stepMisc(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	switch {
	case op == 0x8d: // newarr
		s.push(newArr(s.pop()))
	case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3: // ldelem*
		idx := s.pop()
		arr := s.pop()
		s.push(elem(arr, idx))
	case op >= 0x9b && op <= 0xa2 || op == 0xa4: // stelem*
		v := s.pop()
		idx := s.pop()
		arr := s.pop()
		if arr.k == vArr && idx.k == vInt && idx.n >= 0 && int(idx.n) < len(arr.el) {
			arr.el[idx.n] = v
		}
	case op == 0xc2 || op == 0xc6 || op == 0xfe1d:
		s.pop()
		s.push(unk("typedref"))
	case op == 0xd0: // ldtoken
		s.push(f.s.token(uint32(in.Arg)))
	case op == 0xfe06: // ldftn
		s.push(f.s.methodToken(uint32(in.Arg)))
	case op == 0xfe07: // ldvirtftn
		s.pop()
		s.push(f.s.methodToken(uint32(in.Arg)))
	case op == 0xfe15: // initobj
		s.pop()
	case op == 0xfe17 || op == 0xfe18: // cpblk, initblk
		s.drop(3)
	case op == 0xfe1c:
		s.push(unk("sizeof"))
	case op == 0xfe00:
		s.push(unk("arglist"))
	default:
		return false
	}
	return true
}

// newArr is the array newarr makes for length n: known null elements for a small constant.
func newArr(n *val) *val {
	arr := &val{k: vArr}
	if n.k == vInt && n.n > 0 && n.n < 1000 {
		arr.el = make([]*val, n.n)
		for i := range arr.el {
			arr.el[i] = null
		}
	} else if n.k != vInt {
		arr = unk("collection")
	}
	return arr
}

// stepCall handles call, callvirt and newobj through the API models in call.
func (f *frame) stepCall(in dotnet.Instr, s *valStack) bool {
	op := in.Op
	if op != 0x28 && op != 0x6f && op != 0x73 {
		return false
	}
	m, ok := f.s.a.Member(uint32(in.Arg))
	if !ok {
		s.v = s.v[:0]
		return true
	}
	n := m.Params
	if m.HasThis && op != 0x73 {
		n++
	}
	cargs := s.popN(n)
	if op == 0x73 {
		cargs = append([]*val{unk("this")}, cargs...)
	}
	r := f.s.call(f.row, m, op, cargs)
	if op == 0x73 || !m.RetVoid {
		if r == nil {
			r = unk("call " + shortOwner(m.Owner) + "." + m.Name)
		}
		s.push(r)
	}
	return true
}

func mergeStacks(a, b []*val) []*val {
	if len(a) != len(b) {
		return a
	}
	out := make([]*val, len(a))
	for i := range a {
		out[i] = a[i]
		if a[i] != b[i] && a[i].key() != b[i].key() {
			out[i] = alt(a[i], b[i])
		}
	}
	return out
}

func alt(a, b *val) *val {
	var el []*val
	for _, x := range []*val{a, b} {
		if x.k == vAlt {
			el = append(el, x.el...)
		} else {
			el = append(el, x)
		}
	}
	if len(el) > 64 {
		return unk("collection")
	}
	return &val{k: vAlt, el: el}
}

func elem(arr, idx *val) *val {
	switch arr.k {
	case vArr:
		if idx.k == vInt && idx.n >= 0 && int(idx.n) < len(arr.el) {
			return arr.el[idx.n]
		}
		if len(arr.el) == 0 {
			return unk("collection")
		}
		if len(arr.el) == 1 {
			return arr.el[0]
		}
		return &val{k: vAlt, el: arr.el}
	case vParam:
		return unk("parameter")
	}
	return unk("collection")
}

func shortOwner(o string) string {
	if i := strings.IndexByte(o, '`'); i >= 0 {
		o = o[:i]
	}
	if i := strings.LastIndexAny(o, ".+"); i >= 0 {
		return o[i+1:]
	}
	return o
}

func (s *scanner) token(tok uint32) *val {
	switch tok >> 24 {
	case 0x01, 0x02, 0x1B:
		if n := s.a.TypeToken(tok); n != "" {
			return vtyp(n)
		}
		return unk("generic type")
	case 0x06, 0x0A, 0x2B:
		return s.methodToken(tok)
	}
	return unk("field")
}

func (s *scanner) methodToken(tok uint32) *val {
	m, ok := s.a.Member(tok)
	if !ok || m.Owner == "" {
		return unk("method token")
	}
	mk := ""
	if m.Name == ".ctor" {
		mk = "ctor"
	}
	return &val{k: vMethod, s: mk, a: vtyp(m.Owner), b: vstr(m.Name), n: int64(m.Def)}
}

// lambdaName: the string a predicate lambda compares a member's Name with
// (x => x.Name == "X", x.Name.Contains("X"), StartsWith, Equals), "" if it is not that simple.
func (s *scanner) lambdaName(fn *val) string {
	if fn == nil || fn.k != vMethod || fn.n == 0 {
		return ""
	}
	name, strs, cmp := false, []string{}, false
	for _, in := range dotnet.DecodeIL(s.a.Body(uint32(fn.n))) {
		switch in.Op {
		case 0x72:
			strs = append(strs, s.a.UserString(uint32(in.Arg)))
		case 0x28, 0x6f:
			m, _ := s.a.Member(uint32(in.Arg))
			switch {
			case m.Name == "get_Name":
				name = true
			case m.Owner == "System.String" && (m.Name == "Contains" || m.Name == "op_Equality" || m.Name == "Equals" || m.Name == "StartsWith"):
				cmp = true
			}
		}
	}
	if name && cmp && len(strs) == 1 {
		return strs[0]
	}
	return ""
}

func harmonyOwner(o, name string) bool {
	return o == "HarmonyLib."+name || o == "Harmony."+name
}

// callOutcome says how a group of modelled APIs handled a call.
type callOutcome uint8

const (
	callNext   callOutcome = iota // no API of the group matched: try the next group
	callFall                      // an API matched without a result: try wrappers and summaries
	callReturn                    // the result is final (nil = unknown)
)

// argAt is args[i], or null when the call has fewer arguments.
func argAt(args []*val, i int) *val {
	if i < len(args) {
		return args[i]
	}
	return null
}

// firstArr is the first array or null argument from index from on, nil if none.
func firstArr(args []*val, from int) *val {
	for i := from; i < len(args); i++ {
		if args[i].k == vArr || args[i].k == vNull {
			return args[i]
		}
	}
	return nil
}

// call models the reflection and Harmony APIs used to name patch targets; nil = unknown result.
// The API groups are tried in order; the first that matches decides.
func (s *scanner) call(row uint32, m dotnet.Member, op uint16, args []*val) *val {
	r, how := s.callHarmony(row, m, args)
	if how == callNext {
		r, how = callReflection(m, args)
	}
	if how == callNext {
		r, how = callCollection(m, op, args)
	}
	if how == callReturn {
		return r
	}
	return s.callAssembly(row, m, args)
}

// callHarmony models Harmony.Patch, HarmonyMethod, GenTypes, LINQ method searches and AccessTools.
func (s *scanner) callHarmony(row uint32, m dotnet.Member, args []*val) (*val, callOutcome) {
	o, n := m.Owner, m.Name
	switch {
	case (harmonyOwner(o, "Harmony") || harmonyOwner(o, "HarmonyInstance")) && n == "Patch" && m.HasThis:
		pc := pcall{orig: argAt(args, 1)}
		for i := 2; i < len(args) && i-2 < len(patchKinds5); i++ {
			pc.hms = append(pc.hms, args[i])
			pc.kind = append(pc.kind, patchKinds5[i-2])
		}
		s.patchCall(row, pc)
		return nil, callReturn
	case harmonyOwner(o, "HarmonyMethod") && (n == ".ctor" || n == "op_Implicit"):
		return harmonyMethod(n, args), callReturn
	case o == "Verse.GenTypes" && n == "GetTypeInAnyAssembly":
		if argAt(args, 0).k == vStr {
			return vtyp(argAt(args, 0).s), callReturn
		}
		return reason(argAt(args, 0), "computed name"), callReturn
	case o == "System.Linq.Enumerable" && argAt(args, 0).k == vSearch:
		return s.linqSearch(n, args), callReturn
	case harmonyOwner(o, "AccessTools") || strings.HasSuffix(o, ".AccessToolsExtensions"):
		if r, ok := s.accessTools(n, args); ok {
			return r, callReturn
		}
		return nil, callFall
	}
	return nil, callNext
}

// harmonyMethod is new HarmonyMethod(…) or an implicit conversion to one.
func harmonyMethod(n string, args []*val) *val {
	a := args
	if n == ".ctor" {
		a = args[1:]
	}
	if len(a) == 0 {
		return unk("empty HarmonyMethod")
	}
	if a[0].k == vType && len(a) > 1 {
		return &val{k: vHM, a: &val{k: vMethod, a: a[0], b: a[1], c: argOr(a, 2)}}
	}
	return &val{k: vHM, a: a[0]}
}

// linqSearch is a LINQ call (Where, First …) on a method search, narrowed by a lambda's name.
func (s *scanner) linqSearch(n string, args []*val) *val {
	src := argAt(args, 0)
	if len(args) > 1 {
		nm := s.lambdaName(argAt(args, 1))
		if nm == "" {
			return unk("method search")
		}
		src = &val{k: vSearch, a: src.a, b: vstr(nm)}
	}
	switch n {
	case "Where":
		return src
	case "First", "FirstOrDefault", "Single", "SingleOrDefault", "Last", "LastOrDefault":
		if src.b != nil {
			return &val{k: vMethod, a: src.a, b: src.b}
		}
	}
	return unk("method search")
}

// accessTools models the AccessTools method n; false when it is not one of those modelled.
func (s *scanner) accessTools(n string, args []*val) (*val, bool) {
	arg0, arg1 := argAt(args, 0), argAt(args, 1)
	switch n {
	case "Method", "DeclaredMethod":
		return atMethod(args, ""), true
	case "PropertyGetter", "DeclaredPropertyGetter":
		return atMethod(args, "get"), true
	case "PropertySetter", "DeclaredPropertySetter":
		return atMethod(args, "set"), true
	case "Constructor", "DeclaredConstructor":
		return &val{k: vMethod, s: "ctor", a: arg0, b: vstr(".ctor"), c: argOr(args, 1)}, true
	case "Property", "DeclaredProperty":
		if arg0.k == vStr {
			t, nm := splitColon(arg0.s)
			return &val{k: vProp, a: t, b: nm}, true
		}
		return &val{k: vProp, a: arg0, b: arg1}, true
	case "TypeByName":
		if arg0.k == vStr {
			return vtyp(arg0.s), true
		}
		return reason(arg0, "computed name"), true
	case "Inner":
		if arg0.k == vType && arg1.k == vStr {
			return vtyp(arg0.s + "+" + arg1.s), true
		}
		return reason(arg0, "computed name"), true
	case "EnumeratorMoveNext", "AsyncMoveNext":
		return unk("enumerator"), true
	case "GetDeclaredMethods":
		return &val{k: vSearch, a: arg0}, true
	case "FirstMethod":
		if nm := s.lambdaName(arg1); nm != "" {
			return &val{k: vMethod, a: arg0, b: vstr(nm)}, true
		}
		return unk("method search"), true
	case "FirstConstructor", "GetDeclaredConstructors", "FirstInner", "GetMethodNames", "AllTypes", "GetTypesFromAssembly":
		return unk("method search"), true
	}
	return nil, false
}

// callReflection models System.Type, GetType(), MethodBase, PropertyInfo, MethodInfo, delegates,
// SymbolExtensions and LINQ expressions.
func callReflection(m dotnet.Member, args []*val) (*val, callOutcome) {
	o, n := m.Owner, m.Name
	switch {
	case o == "System.Type" || o == "System.Reflection.TypeInfo" || o == "System.Reflection.IReflect":
		if r, ok := typeAPI(m, args); ok {
			return r, callReturn
		}
		return nil, callFall
	case o == "System.Object" && n == "GetType":
		if a := argAt(args, 0); a.k == vParam || a.k == vThis {
			return &val{k: vTypeOf, a: a}, callReturn
		}
		return unk("GetType()"), callReturn
	case o == "System.Reflection.MethodBase" && n == "GetMethodFromHandle":
		return argAt(args, 0), callReturn
	case o == "System.Reflection.PropertyInfo":
		p := argAt(args, 0)
		if p.k != vProp {
			return nil, callReturn
		}
		switch n {
		case "GetGetMethod", "get_GetMethod":
			return &val{k: vMethod, s: "get", a: p.a, b: p.b}, callReturn
		case "GetSetMethod", "get_SetMethod":
			return &val{k: vMethod, s: "set", a: p.a, b: p.b}, callReturn
		}
		return nil, callFall
	case o == "System.Reflection.MethodInfo" && (n == "MakeGenericMethod" || n == "GetBaseDefinition"):
		return argAt(args, 0), callReturn
	case o == "System.Delegate" && n == "get_Method", o == "System.Reflection.RuntimeReflectionExtensions" && n == "GetMethodInfo":
		return argAt(args, 0), callReturn
	case strings.HasSuffix(o, "SymbolExtensions") && n == "GetMethodInfo":
		return firstMethod(args), callReturn
	case o == "System.Linq.Expressions.Expression":
		switch {
		case n == "Call" || n == "Property":
			if v := firstMethod(args); v != nil {
				return v, callReturn
			}
		case strings.HasPrefix(n, "Lambda"):
			return argAt(args, 0), callReturn
		}
		return unk("expression"), callReturn
	}
	return nil, callNext
}

// typeAPI models the System.Type method m; false when it is not one of those modelled.
func typeAPI(m dotnet.Member, args []*val) (*val, bool) {
	arg0, arg1 := argAt(args, 0), argAt(args, 1)
	switch m.Name {
	case "GetTypeFromHandle":
		return arg0, true
	case "GetMethod":
		return &val{k: vMethod, a: arg0, b: arg1, c: firstArr(args, 2)}, true
	case "GetProperty":
		return &val{k: vProp, a: arg0, b: arg1}, true
	case "GetConstructor":
		return &val{k: vMethod, s: "ctor", a: arg0, b: vstr(".ctor"), c: firstArr(args, 1)}, true
	case "GetNestedType":
		if arg0.k == vType && arg1.k == vStr {
			return vtyp(arg0.s + "+" + arg1.s), true
		}
	case "GetType":
		if !m.HasThis && arg0.k == vStr {
			t := arg0.s
			if i := strings.IndexByte(t, ','); i >= 0 {
				t = strings.TrimSpace(t[:i])
			}
			return vtyp(t), true
		}
	case "MakeGenericType", "get_UnderlyingSystemType":
		return arg0, true
	case "GetMethods":
		return &val{k: vSearch, a: arg0}, true
	case "GetConstructors", "GetNestedTypes", "GetProperties":
		return unk("method search"), true
	}
	return nil, false
}

// callCollection models String.Concat, enumerators, List/HashSet/ICollection, LINQ copies and
// delegates made from ldftn.
func callCollection(m dotnet.Member, op uint16, args []*val) (*val, callOutcome) {
	o, n := m.Owner, m.Name
	switch {
	case o == "System.String" && n == "Concat":
		return concat(args), callReturn
	case n == "get_Current":
		if e := argAt(args, 0); e.k == vEnum {
			return elem(e.a, unk("index")), callReturn
		}
		return unk("collection"), callReturn
	case strings.HasPrefix(o, "System.Collections.Generic.List`1") || strings.HasPrefix(o, "System.Collections.Generic.HashSet`1") ||
		strings.HasPrefix(o, "System.Collections.Generic.ICollection`1"):
		if r, ok := listAPI(n, args); ok {
			return r, callReturn
		}
		return nil, callFall
	case n == "GetEnumerator" && (strings.HasPrefix(o, "System.Collections.Generic.IEnumerable`1") || o == "System.Collections.IEnumerable"):
		return &val{k: vEnum, a: argAt(args, 0)}, callReturn
	case o == "System.Linq.Enumerable" && (n == "ToList" || n == "ToArray" || n == "AsEnumerable" || n == "Distinct"):
		return argAt(args, 0), callReturn
	case op == 0x73 && n == ".ctor" && len(args) == 3 && argAt(args, 2).k == vMethod:
		return argAt(args, 2), callReturn // a delegate made from ldftn
	}
	return nil, callNext
}

// concat is String.Concat of constant strings; anything else is a computed name.
func concat(args []*val) *val {
	var sb strings.Builder
	for _, x := range args {
		if x.k != vStr {
			return unk("computed name")
		}
		sb.WriteString(x.s)
	}
	return vstr(sb.String())
}

// listAPI models the List/HashSet/ICollection method n; false when it is not one of those modelled.
func listAPI(n string, args []*val) (*val, bool) {
	arg0, arg1 := argAt(args, 0), argAt(args, 1)
	switch n {
	case ".ctor":
		if len(args) == 1 || arg1.k == vInt {
			return &val{k: vArr}, true
		}
		if arg1.k == vArr {
			return &val{k: vArr, el: append([]*val(nil), arg1.el...)}, true
		}
		return unk("collection"), true
	case "Add":
		if c := arg0; c.k == vArr && len(c.el) < 256 {
			c.el = append(c.el, arg1)
		}
		return nil, true
	case "AddRange":
		if c := arg0; c.k == vArr && arg1.k == vArr {
			c.el = append(c.el, arg1.el...)
		}
		return nil, true
	case "get_Item":
		return elem(arg0, arg1), true
	case "GetEnumerator":
		return &val{k: vEnum, a: arg0}, true
	case "ToArray":
		return arg0, true
	}
	return nil, false
}

// callAssembly handles a call no API model decided: a wrapper of Harmony.Patch, or a method of
// this assembly through its summary.
func (s *scanner) callAssembly(row uint32, m dotnet.Member, args []*val) *val {
	// A wrapper of Harmony.Patch in another assembly (or one without a summary):
	// Patch(MethodBase original, HarmonyMethod prefix, HarmonyMethod postfix, …).
	if (m.Def == 0 || s.sums[m.Def] == nil) && isWrapper(m.Types) {
		off := 0
		if m.HasThis {
			off = 1
		}
		pc := pcall{orig: argAt(args, off)}
		for j := 1; j < len(m.Types) && j-1 < len(patchKinds5) && isHM(m.Types[j]); j++ {
			pc.hms = append(pc.hms, argAt(args, off+j))
			pc.kind = append(pc.kind, patchKinds5[j-1])
		}
		s.patchCall(row, pc)
		return nil
	}
	// A method of this assembly: its summary at this call site.
	if m.Def != 0 && m.Def != row {
		if sm := s.sums[m.Def]; sm != nil {
			for _, pc := range sm.calls {
				s.called[m.Def] = true
				s.patchCall(row, pc.subst(args))
			}
			if sm.ret != nil {
				return subst(sm.ret, args, 0)
			}
		}
	}
	return nil
}

func isHM(t string) bool { return t == "HarmonyLib.HarmonyMethod" || t == "Harmony.HarmonyMethod" }

func isWrapper(ts []string) bool {
	return len(ts) >= 2 && (ts[0] == "System.Reflection.MethodBase" || ts[0] == "System.Reflection.MethodInfo") && isHM(ts[1])
}

func argOr(a []*val, i int) *val {
	if i < len(a) {
		return a[i]
	}
	return nil
}

func firstMethod(args []*val) *val {
	for _, x := range args {
		if x.k == vMethod {
			return x
		}
	}
	return nil
}

func reason(v *val, def string) *val {
	if v.k == vUnk {
		return v
	}
	if v.k == vParam {
		return v
	}
	return unk(def)
}

func splitColon(s string) (*val, *val) {
	t, m, ok := strings.Cut(s, ":")
	if !ok {
		return unk("computed name"), unk("computed name")
	}
	return vtyp(t), vstr(m)
}

// atMethod: AccessTools.Method(Type, string, Type[] …) or AccessTools.Method("Type:Name", Type[] …).
func atMethod(args []*val, mk string) *val {
	if len(args) == 0 {
		return unk("call AccessTools")
	}
	if args[0].k == vStr {
		t, nm := splitColon(args[0].s)
		return &val{k: vMethod, s: mk, a: t, b: nm, c: argOr(args, 1)}
	}
	if args[0].k == vParam && len(args) == 1 {
		return &val{k: vMethod, s: mk, a: unk("parameter"), b: unk("parameter")}
	}
	return &val{k: vMethod, s: mk, a: args[0], b: argOr(args, 1), c: argOr(args, 2)}
}

func (s *scanner) patchCall(row uint32, pc pcall) {
	if pc.hasParam() {
		sm := s.sum(row)
		if len(sm.calls) < 256 {
			sm.calls = append(sm.calls, pc)
		}
		return
	}
	if s.final {
		s.emit(pc)
	}
}

type target struct{ typ, method, args string }

// strs resolves a string value (one or several alternatives).
func strs(v *val) ([]string, string) {
	switch v.k {
	case vStr:
		return []string{v.s}, ""
	case vAlt:
		var out []string
		for _, x := range nonNull(v.el) {
			s, r := strs(x)
			if r != "" {
				return nil, r
			}
			out = append(out, s...)
		}
		return out, ""
	}
	return nil, why(v, "computed name")
}

func types(v *val) ([]string, string) {
	switch v.k {
	case vType:
		return []string{v.s}, ""
	case vTypeOf:
		return nil, "GetType()"
	case vAlt:
		var out []string
		for _, x := range nonNull(v.el) {
			s, r := types(x)
			if r != "" {
				return nil, r
			}
			out = append(out, s...)
		}
		return out, ""
	}
	return nil, why(v, "computed type")
}

// nonNull drops null alternatives (cond ? new HarmonyMethod(m) : null); all null stays null.
func nonNull(el []*val) []*val {
	var out []*val
	for _, x := range el {
		if x.k != vNull {
			out = append(out, x)
		}
	}
	if len(out) == 0 {
		return []*val{null}
	}
	return out
}

func why(v *val, def string) string {
	switch v.k {
	case vUnk:
		return v.s
	case vParam:
		return "parameter"
	case vNull:
		return "null"
	}
	return def
}

func argList(v *val) string {
	if v == nil || v.k != vArr {
		return ""
	}
	var as []string
	for _, x := range v.el {
		if x.k != vType {
			return ""
		}
		as = append(as, dotnet.CSName(x.s))
	}
	return "(" + strings.Join(as, ", ") + ")"
}

// methods resolves a method value to (type, name, args) triples.
func methods(v *val) ([]target, string) {
	switch v.k {
	case vMethod:
		ts, r := types(v.a)
		if r != "" {
			return nil, r
		}
		var ns []string
		if v.s == "ctor" {
			ns = []string{".ctor"}
		} else if ns, r = strs(v.b); r != "" {
			return nil, r
		}
		var out []target
		for _, t := range ts {
			for _, n := range ns {
				switch v.s {
				case "get":
					n = "get_" + n
				case "set":
					n = "set_" + n
				}
				out = append(out, target{t, n, argList(v.c)})
			}
		}
		return out, ""
	case vHM:
		return methods(v.a)
	case vAlt:
		var out []target
		for _, x := range nonNull(v.el) {
			t, r := methods(x)
			if r != "" {
				return nil, r
			}
			out = append(out, t...)
		}
		return out, ""
	}
	return nil, why(v, "computed method")
}

func (s *scanner) emit(pc pcall) {
	s.out.ManualCalls++
	origs, oreason := methods(pc.orig)
	if len(origs) > 64 {
		origs, oreason = nil, "collection"
	}
	any := false
	for i, h := range pc.hms {
		if h == nil || h.k == vNull {
			continue
		}
		pms, r := methods(h)
		if r == "null" {
			continue
		}
		if r != "" {
			s.out.unresolved("patch method: " + r)
			continue
		}
		for _, pm := range pms {
			p := Patch{PatchType: pm.typ, PatchMethod: pm.method, Kind: pc.kind[i], Manual: true}
			p.Skips = p.Kind == Prefix && s.returnsBool(pm.typ, pm.method)
			if oreason != "" {
				p.Dynamic, p.Unresolved = true, oreason
				s.add(p)
				continue
			}
			any = true
			for _, o := range origs {
				q := p
				q.TargetType, q.TargetMethod, q.Args = o.typ, o.method, o.args
				s.add(q)
			}
		}
	}
	switch {
	case oreason != "":
		s.out.unresolved(oreason)
	case any:
		s.out.ManualResolved++
	}
}

func (s *scanner) add(p Patch) {
	k := p.Kind + "|" + p.PatchType + "." + p.PatchMethod + "|" + p.Target()
	if s.seen[k] {
		return
	}
	s.seen[k] = true
	s.out.Patches = append(s.out.Patches, p)
}

func (s *scanner) returnsBool(typ, method string) bool {
	r := s.a.TypeRow(typ)
	if r == 0 {
		return false
	}
	for _, m := range s.a.Methods(r) {
		if m.Name == method && m.ReturnBool {
			return true
		}
	}
	return false
}

func (a *Assembly) unresolved(reason string) {
	if a.Unresolved == nil {
		a.Unresolved = map[string]int{}
	}
	a.Unresolved[reason]++
}
