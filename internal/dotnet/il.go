package dotnet

// IL access for the manual Harmony.Patch scan (internal/harmony/manual.go): method bodies,
// metadata tokens, and an opcode decoder. ECMA-335 §II.25.4 (method headers), §III (opcodes).

import (
	"encoding/binary"
	"os"
)

type ilState struct {
	raw        []byte
	err        error
	methOwner  []uint32 // MethodDef row-1 → owning TypeDef row
	fieldOwner []uint32 // Field row-1 → owning TypeDef row
	typeByName map[string]uint32
}

func (a *Assembly) ilData() *ilState {
	if a.il != nil {
		return a.il
	}
	s := &ilState{}
	a.il = s
	s.raw, s.err = os.ReadFile(a.path)
	s.methOwner = make([]uint32, len(a.methods))
	s.fieldOwner = make([]uint32, len(a.fields))
	s.typeByName = map[string]uint32{}
	for i, t := range a.typeDefs {
		row := uint32(i + 1)
		s.typeByName[a.typeDefName(row)] = row
		mEnd, fEnd := uint32(len(a.methods)+1), uint32(len(a.fields)+1)
		if i+1 < len(a.typeDefs) {
			mEnd, fEnd = a.typeDefs[i+1].MethodList, a.typeDefs[i+1].FieldList
		}
		for m := t.MethodList; m >= 1 && m < mEnd && int(m) <= len(a.methods); m++ {
			s.methOwner[m-1] = row
		}
		for f := t.FieldList; f >= 1 && f < fEnd && int(f) <= len(a.fields); f++ {
			s.fieldOwner[f-1] = row
		}
	}
	return s
}

// MethodCount is the number of MethodDef rows.
func (a *Assembly) MethodCount() int { return len(a.methods) }

// MethodName returns the name of MethodDef row (1-based).
func (a *Assembly) MethodName(row uint32) string {
	if row == 0 || int(row) > len(a.methods) {
		return ""
	}
	return a.str(a.methods[row-1].Name)
}

// MethodDeclType returns the full name of the type declaring MethodDef row.
func (a *Assembly) MethodDeclType(row uint32) string {
	s := a.ilData()
	if row == 0 || int(row) > len(s.methOwner) {
		return ""
	}
	return a.typeDefName(s.methOwner[row-1])
}

// TypeRow finds a TypeDef by full name ('+' for nested types), 0 if not defined here.
func (a *Assembly) TypeRow(name string) uint32 { return a.ilData().typeByName[name] }

func (a *Assembly) rvaOffset(rva uint32) (int, bool) {
	for _, s := range a.sections {
		size := s.vsize
		if s.rawSize > size {
			size = s.rawSize
		}
		if rva >= s.va && rva < s.va+size {
			return int(rva - s.va + s.raw), true
		}
	}
	return 0, false
}

// Body returns the IL code of MethodDef row (1-based); nil for abstract/extern methods or errors.
func (a *Assembly) Body(row uint32) []byte {
	if row == 0 || int(row) > len(a.methods) {
		return nil
	}
	rva := a.methods[row-1].RVA
	s := a.ilData()
	if rva == 0 || s.err != nil {
		return nil
	}
	off, ok := a.rvaOffset(rva)
	b := s.raw
	if !ok || off >= len(b) {
		return nil
	}
	switch b[off] & 3 {
	case 2: // tiny header
		n := int(b[off] >> 2)
		if off+1+n > len(b) {
			return nil
		}
		return b[off+1 : off+1+n]
	case 3: // fat header
		if off+12 > len(b) {
			return nil
		}
		hdr := int(b[off+1]>>4) * 4
		n := int(binary.LittleEndian.Uint32(b[off+4:]))
		if hdr < 12 || n < 0 || off+hdr+n > len(b) {
			return nil
		}
		return b[off+hdr : off+hdr+n]
	}
	return nil
}

// Member is a method or field referenced by a metadata token.
type Member struct {
	Owner   string // declaring type ('+' for nested types); "" if unknown
	Name    string
	Field   bool
	HasThis bool
	Params  int
	RetVoid bool
	Def     uint32   // MethodDef row when the method is defined in this assembly
	Types   []string // parameter types (generic parameters as "!"; "" if not decoded)
}

// Member resolves a MethodDef (0x06), MemberRef (0x0A), MethodSpec (0x2B) or Field (0x04) token.
func (a *Assembly) Member(tok uint32) (Member, bool) {
	row := tok & 0xFFFFFF
	switch tok >> 24 {
	case 0x06:
		if row == 0 || int(row) > len(a.methods) {
			return Member{}, false
		}
		md := a.methods[row-1]
		m := Member{Owner: a.typeDefName(a.ilData().methOwner[row-1]), Name: a.str(md.Name), Def: row}
		m.HasThis, m.Params, m.RetVoid, m.Types = a.methodSig(a.blobAt(md.Signature))
		return m, true
	case 0x0A:
		if row == 0 || int(row) > len(a.members) {
			return Member{}, false
		}
		mr := a.members[row-1]
		m := Member{Name: a.str(mr.Name)}
		idx := mr.Class >> 3
		switch mr.Class & 7 {
		case 0:
			m.Owner = a.typeDefName(idx)
		case 1:
			m.Owner = a.typeRefName(idx)
		case 3: // MethodDef (vararg call site)
			m.Owner = a.MethodDeclType(idx)
		case 4:
			m.Owner = a.typeSpecName(idx)
		}
		sig := a.blobAt(mr.Signature)
		if len(sig) > 0 && sig[0] == 0x06 {
			m.Field = true
			return m, true
		}
		m.HasThis, m.Params, m.RetVoid, m.Types = a.methodSig(sig)
		if t := a.ilData().typeByName[m.Owner]; t != 0 && mr.Class&7 == 0 {
			for _, x := range a.Methods(t) {
				if x.Name == m.Name && x.Params == m.Params {
					m.Def = x.Row
				}
			}
		}
		return m, true
	case 0x2B:
		if row == 0 || int(row) > len(a.methSpecs) {
			return Member{}, false
		}
		c := a.methSpecs[row-1].Method
		if c&1 == 0 {
			return a.Member(0x06<<24 | c>>1)
		}
		return a.Member(0x0A<<24 | c>>1)
	case 0x04:
		if row == 0 || int(row) > len(a.fields) {
			return Member{}, false
		}
		return Member{Owner: a.typeDefName(a.ilData().fieldOwner[row-1]), Name: a.str(a.fields[row-1].Name), Field: true}, true
	}
	return Member{}, false
}

// methodSig reads a MethodDefSig/MethodRefSig: stack effects and parameter types.
func (a *Assembly) methodSig(sig []byte) (hasThis bool, params int, retVoid bool, types []string) {
	if len(sig) < 2 {
		return
	}
	r := &reader{b: sig}
	cc := r.u8()
	hasThis = cc&0x20 != 0 && cc&0x40 == 0
	if cc&0x10 != 0 {
		r.compressed()
	}
	params = int(r.compressed())
	ret := a.sigTypeName(r)
	retVoid = ret == "System.Void"
	for i := 0; i < params && r.err == nil && r.left() > 0; i++ {
		types = append(types, a.sigTypeName(r))
	}
	return
}

var primNames = map[byte]string{0x02: "System.Boolean", 0x03: "System.Char", 0x04: "System.SByte", 0x05: "System.Byte",
	0x06: "System.Int16", 0x07: "System.UInt16", 0x08: "System.Int32", 0x09: "System.UInt32", 0x0a: "System.Int64",
	0x0b: "System.UInt64", 0x0c: "System.Single", 0x0d: "System.Double", 0x0e: "System.String", 0x18: "System.IntPtr",
	0x19: "System.UIntPtr", 0x1c: "System.Object"}

// sigTypeName reads one type in a signature blob; generic instances give the generic type's name.
func (a *Assembly) sigTypeName(r *reader) string {
	e := r.u8()
	if n, ok := primNames[e]; ok {
		return n
	}
	switch e {
	case 0x01:
		return "System.Void"
	case 0x1f, 0x20, 0x45: // custom modifier, pinned
		if e != 0x45 {
			r.compressed()
		}
		return a.sigTypeName(r)
	case 0x13, 0x1e: // VAR, MVAR
		r.compressed()
		return "!"
	case 0x14: // ARRAY
		el := a.sigTypeName(r)
		r.compressed()
		for i, n := 0, int(r.compressed()); i < n && r.err == nil; i++ {
			r.compressed()
		}
		for i, n := 0, int(r.compressed()); i < n && r.err == nil; i++ {
			r.compressed()
		}
		return el + "[,]"
	case 0x11, 0x12:
		return a.typeDefOrRef(r.compressed())
	case 0x15:
		r.u8()
		name := a.typeDefOrRef(r.compressed())
		n := int(r.compressed())
		for i := 0; i < n && r.err == nil; i++ {
			a.sigTypeName(r)
		}
		return name
	case 0x1d:
		if el := a.sigTypeName(r); el != "" {
			return el + "[]"
		}
	case 0x0f, 0x10: // PTR, BYREF
		if el := a.sigTypeName(r); el != "" {
			return el + "&"
		}
	}
	return ""
}

func (a *Assembly) typeSpecName(row uint32) string {
	if row == 0 || int(row) > len(a.typeSpecs) {
		return ""
	}
	return a.sigTypeName(&reader{b: a.blobAt(a.typeSpecs[row-1].Signature)})
}

// TypeToken resolves a TypeDef (0x02), TypeRef (0x01) or TypeSpec (0x1B) token to a type name.
func (a *Assembly) TypeToken(tok uint32) string {
	row := tok & 0xFFFFFF
	switch tok >> 24 {
	case 0x01:
		return a.typeRefName(row)
	case 0x02:
		return a.typeDefName(row)
	case 0x1B:
		return a.typeSpecName(row)
	}
	return ""
}

// UserString returns the #US string of an ldstr token.
func (a *Assembly) UserString(tok uint32) string {
	i := int(tok & 0xFFFFFF)
	if tok>>24 != 0x70 || i >= len(a.us) {
		return ""
	}
	n, sz := compressed(a.us[i:])
	if sz == 0 || n == 0 || i+sz+int(n) > len(a.us) {
		return ""
	}
	b := a.us[i+sz : i+sz+int(n)-1]
	r := make([]rune, 0, len(b)/2)
	for j := 0; j+1 < len(b); j += 2 {
		r = append(r, rune(binary.LittleEndian.Uint16(b[j:])))
	}
	return string(r)
}

// Instr is one decoded IL instruction. Two-byte opcodes are 0xFE00|second byte.
type Instr struct {
	Off     int
	Op      uint16
	Arg     int64 // inline operand: token, int, local/arg index, or branch target offset
	Targets []int // switch targets
}

// operand sizes of one-byte opcodes (-1: switch); unlisted = 0.
var opSize1 = func() [256]int8 {
	var t [256]int8
	for _, o := range []byte{0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x1f, 0xde} {
		t[o] = 1
	}
	for o := 0x2b; o <= 0x37; o++ {
		t[o] = 1
	}
	for _, o := range []byte{0x20, 0x22, 0x27, 0x28, 0x29, 0x6f, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x79, 0x7b, 0x7c, 0x7d,
		0x7e, 0x7f, 0x80, 0x81, 0x8c, 0x8d, 0x8f, 0xa3, 0xa4, 0xa5, 0xc2, 0xc6, 0xd0, 0xdd} {
		t[o] = 4
	}
	for o := 0x38; o <= 0x44; o++ {
		t[o] = 4
	}
	t[0x21], t[0x23] = 8, 8
	t[0x45] = -1
	return t
}()

var opSize2 = map[byte]int{0x06: 4, 0x07: 4, 0x09: 2, 0x0a: 2, 0x0b: 2, 0x0c: 2, 0x0d: 2, 0x0e: 2, 0x12: 1, 0x15: 4, 0x16: 4, 0x19: 1, 0x1c: 4}

// IsBranch reports conditional/unconditional branch opcodes (short and long forms, leave).
func IsBranch(op uint16) bool {
	return op >= 0x2b && op <= 0x44 || op == 0xdd || op == 0xde
}

// DecodeIL decodes a method body; it stops at the first malformed instruction.
func DecodeIL(code []byte) []Instr {
	var out []Instr
	for p := 0; p < len(code); {
		in := Instr{Off: p}
		op := uint16(code[p])
		p++
		size := int(opSize1[op])
		if op == 0xfe {
			if p >= len(code) {
				break
			}
			op = 0xfe00 | uint16(code[p])
			size = opSize2[code[p]]
			p++
		}
		in.Op = op
		if size == -1 { // switch
			if p+4 > len(code) {
				break
			}
			n := int(binary.LittleEndian.Uint32(code[p:]))
			p += 4
			if n < 0 || p+4*n > len(code) {
				break
			}
			end := p + 4*n
			for i := 0; i < n; i++ {
				in.Targets = append(in.Targets, end+int(int32(binary.LittleEndian.Uint32(code[p+4*i:]))))
			}
			p = end
			out = append(out, in)
			continue
		}
		if p+size > len(code) {
			break
		}
		switch size {
		case 1:
			in.Arg = int64(code[p])
			if IsBranch(op) || op == 0x1f { // signed
				in.Arg = int64(int8(code[p]))
			}
		case 2:
			in.Arg = int64(binary.LittleEndian.Uint16(code[p:]))
		case 4:
			in.Arg = int64(binary.LittleEndian.Uint32(code[p:]))
			if IsBranch(op) || op == 0x20 {
				in.Arg = int64(int32(uint32(in.Arg)))
			}
		case 8:
			in.Arg = int64(binary.LittleEndian.Uint64(code[p:]))
		}
		p += size
		if IsBranch(op) {
			in.Arg += int64(p)
		}
		out = append(out, in)
	}
	return out
}
