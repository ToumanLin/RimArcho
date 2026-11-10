package dotnet

// What a NullReferenceException at an IL offset dereferenced (06 §2, null trace).
//
// Mono prints a frame's IL offset ("[0x00076]") at the start of the statement that was
// running, not always at the faulting instruction: the candidates are the dereferences from
// that offset to the end of the statement (the next point where the evaluation stack is empty,
// or a branch). The first one whose value can be null is the one that throws when its value
// is null.

// Deref is one dereference: the instruction and the value it dereferences.
type Deref struct {
	At    Instr
	Value Shape
}

// derefOperand returns the stack depth (0 = top) of the value an instruction dereferences,
// or -1.
func (a *Assembly) derefOperand(in Instr) int {
	switch op := in.Op; {
	case op == 0x7b || op == 0x7c || op == 0x8e: // ldfld, ldflda, ldlen
		return 0
	case op == 0x7d: // stfld
		return 1
	case op == 0x6f: // callvirt: the instance under the arguments
		if m, ok := a.Member(uint32(in.Arg)); ok && m.HasThis {
			return m.Params
		}
	case op == 0x8f || op >= 0x90 && op <= 0x9a || op == 0xa3: // ldelema, ldelem*
		return 1
	case op >= 0x9b && op <= 0xa2 || op == 0xa4: // stelem*
		return 2
	}
	return -1
}

func nullable(s Shape) bool {
	switch s.Kind {
	case ShThis, ShNew, ShString, ShConst:
		return false
	}
	return true
}

// DerefsAt lists the dereferences of possibly-null values in the statement of MethodDef row
// that starts at IL offset off, in order. ok is false when off is not an instruction.
func (a *Assembly) DerefsAt(row uint32, off int) (out []Deref, ok bool) {
	started, done := false, false
	a.eval(row, func(in Instr, st []Shape) {
		if done || in.Off < off {
			return
		}
		if !started {
			if in.Off != off {
				done = true
				return
			}
			started, ok = true, true
		} else if len(st) == 0 {
			done = true // the next statement
			return
		}
		if d := a.derefOperand(in); d >= 0 && d < len(st) {
			if v := st[len(st)-1-d]; nullable(v) {
				out = append(out, Deref{At: in, Value: v})
			}
		}
		if IsBranch(in.Op) || in.Op == 0x2a || in.Op == 0x7a || in.Op == 0x45 {
			done = true
		}
	})
	return out, ok
}

// ValueText says what a shape is, for a trace line: "parameter recipient", "field
// Verse.Pawn::genes of parameter recipient", "local 2", "the result of Verse.Pawn::get_Map()".
func ValueText(s Shape, names []string) string {
	switch s.Kind {
	case ShParam:
		if s.N < len(names) && names[s.N] != "" {
			return "parameter " + names[s.N]
		}
		return "parameter " + itoa(s.N+1)
	case ShField:
		t := "field " + s.Member
		if s.Base != nil && s.Base.Kind != ShOther {
			t += " of " + ValueText(*s.Base, names)
		}
		return t
	case ShStatic:
		return "static field " + s.Member
	case ShLocal:
		return "local " + itoa(s.N)
	case ShCall:
		return "the result of " + s.Member + "()"
	case ShThis:
		return "this"
	case ShNull:
		return "null"
	}
	return "a computed value"
}

func itoa(n int) string {
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

// PushText renders the instruction that pushed a shape ("ldarg.3"), "" when unknown.
func (a *Assembly) PushText(row uint32, s Shape) string {
	if s.Kind == ShOther && s.Off == 0 && s.Op == 0 {
		return ""
	}
	if in, ok := a.instrAt(row, s.Off); ok && in.Op == s.Op {
		return a.InstrText(in)
	}
	return ""
}
