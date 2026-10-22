// Package dotnet reads .NET metadata (TypeDef, MethodDef, MemberRef, CustomAttribute, heaps)
// with saferwall/pe plus a custom-attribute blob decoder (M0 spike spikes/dotnetmeta, 07 §6.1 item 1).
package dotnet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/saferwall/pe"
)

// Assembly is a thin view over saferwall/pe's CLR tables plus the heaps.
type Assembly struct {
	strs     []byte
	blob     []byte
	us       []byte
	typeRefs []pe.TypeRefTableRow
	typeDefs []pe.TypeDefTableRow
	methods  []pe.MethodDefTableRow
	members  []pe.MemberRefTableRow
	attrs    []pe.CustomAttributeTableRow
	nestedIn map[uint32]uint32 // TypeDef row → enclosing TypeDef row (1-based)
	// For IL reading (il.go): the file is read again on demand.
	path      string
	sections  []section
	fields    []pe.FieldTableRow
	typeSpecs []pe.TypeSpecTableRow
	methSpecs []pe.MethodSpecTableRow
	il        *ilState
}

type section struct{ va, vsize, raw, rawSize uint32 }

func table[T any](f *pe.File, id int) []T {
	t, ok := f.CLR.MetadataTables[id]
	if !ok || t == nil || t.Content == nil {
		return nil
	}
	rows, _ := t.Content.([]T)
	return rows
}

func Open(path string) (*Assembly, error) {
	f, err := pe.New(path, &pe.Options{
		OmitExportDirectory: true, OmitImportDirectory: true, OmitExceptionDirectory: true,
		OmitResourceDirectory: true, OmitSecurityDirectory: true, OmitRelocDirectory: true,
		OmitDebugDirectory: true, OmitArchitectureDirectory: true, OmitGlobalPtrDirectory: true,
		OmitTLSDirectory: true, OmitLoadConfigDirectory: true, OmitBoundImportDirectory: true,
		OmitIATDirectory: true, OmitDelayImportDirectory: true,
	})
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.Parse(); err != nil {
		return nil, err
	}
	if f.CLR.MetadataStreams == nil {
		return nil, errors.New("not a .NET assembly")
	}
	a := &Assembly{
		strs:      f.CLR.MetadataStreams["#Strings"],
		blob:      f.CLR.MetadataStreams["#Blob"],
		us:        f.CLR.MetadataStreams["#US"],
		typeRefs:  table[pe.TypeRefTableRow](f, pe.TypeRef),
		typeDefs:  table[pe.TypeDefTableRow](f, pe.TypeDef),
		methods:   table[pe.MethodDefTableRow](f, pe.MethodDef),
		members:   table[pe.MemberRefTableRow](f, pe.MemberRef),
		attrs:     table[pe.CustomAttributeTableRow](f, pe.CustomAttribute),
		nestedIn:  map[uint32]uint32{},
		path:      path,
		fields:    table[pe.FieldTableRow](f, pe.Field),
		typeSpecs: table[pe.TypeSpecTableRow](f, pe.TypeSpec),
		methSpecs: table[pe.MethodSpecTableRow](f, pe.MethodSpec),
	}
	for _, sc := range f.Sections {
		h := sc.Header
		a.sections = append(a.sections, section{h.VirtualAddress, h.VirtualSize, h.PointerToRawData, h.SizeOfRawData})
	}
	for _, n := range table[pe.NestedClassTableRow](f, pe.NestedClass) {
		a.nestedIn[n.NestedClass] = n.EnclosingClass
	}
	// Heaps are sub-slices of the mapped file; copy before Close unmaps it.
	a.strs = append([]byte(nil), a.strs...)
	a.blob = append([]byte(nil), a.blob...)
	a.us = append([]byte(nil), a.us...)
	return a, nil
}

func (a *Assembly) str(i uint32) string {
	if int(i) >= len(a.strs) {
		return ""
	}
	end := i
	for int(end) < len(a.strs) && a.strs[end] != 0 {
		end++
	}
	return string(a.strs[i:end])
}

func (a *Assembly) blobAt(i uint32) []byte {
	if int(i) >= len(a.blob) {
		return nil
	}
	n, sz := compressed(a.blob[i:])
	start := int(i) + sz
	if sz == 0 || start+int(n) > len(a.blob) {
		return nil
	}
	return a.blob[start : start+int(n)]
}

func compressed(b []byte) (uint32, int) {
	if len(b) == 0 {
		return 0, 0
	}
	switch {
	case b[0]&0x80 == 0:
		return uint32(b[0]), 1
	case b[0]&0xC0 == 0x80 && len(b) >= 2:
		return uint32(b[0]&0x3F)<<8 | uint32(b[1]), 2
	case b[0]&0xE0 == 0xC0 && len(b) >= 4:
		return uint32(b[0]&0x1F)<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), 4
	}
	return 0, 0
}

// typeDefName returns the full name of TypeDef row (1-based), nested types joined with '+'.
func (a *Assembly) typeDefName(row uint32) string {
	if row == 0 || int(row) > len(a.typeDefs) {
		return "?"
	}
	t := a.typeDefs[row-1]
	name := a.str(t.TypeName)
	if enc, ok := a.nestedIn[row]; ok {
		return a.typeDefName(enc) + "+" + name
	}
	if ns := a.str(t.TypeNamespace); ns != "" {
		return ns + "." + name
	}
	return name
}

func (a *Assembly) typeRefName(row uint32) string {
	if row == 0 || int(row) > len(a.typeRefs) {
		return "?"
	}
	t := a.typeRefs[row-1]
	name := a.str(t.TypeName)
	// ResolutionScope tag 3 = TypeRef → nested type reference.
	if t.ResolutionScope&3 == 3 {
		return a.typeRefName(t.ResolutionScope>>2) + "+" + name
	}
	if ns := a.str(t.TypeNamespace); ns != "" {
		return ns + "." + name
	}
	return name
}

// typeDefOrRef decodes a TypeDefOrRef coded index (2 tag bits).
func (a *Assembly) typeDefOrRef(c uint32) string {
	switch c & 3 {
	case 0:
		return a.typeDefName(c >> 2)
	case 1:
		return a.typeRefName(c >> 2)
	}
	return "typespec"
}

// methodOwner returns the TypeDef row owning MethodDef row m (1-based):
// the last TypeDef whose MethodList start is <= m.
func (a *Assembly) methodOwner(m uint32) uint32 {
	var owner uint32
	for i, t := range a.typeDefs {
		if t.MethodList != 0 && t.MethodList <= m {
			owner = uint32(i + 1)
		} else if t.MethodList > m {
			break
		}
	}
	return owner
}

// Attr is one decoded custom attribute instance.
type Attr struct {
	Type   string // e.g. HarmonyLib.HarmonyPatch
	Owner  string // Type full name, or Type::Method
	Fixed  []Value
	Named  []NamedValue
	Errors string
}

type Value struct {
	Kind string // type, string, int, enum:<T>, bool, array, null, ...
	S    string
	Arr  []Value
}

type NamedValue struct {
	Name string
	V    Value
}

// ctorInfo returns attribute type name and constructor signature blob.
func (a *Assembly) ctorInfo(c uint32) (string, []byte) {
	idx := c >> 3
	switch c & 7 {
	case 2: // MethodDef
		if idx == 0 || int(idx) > len(a.methods) {
			return "?", nil
		}
		return a.typeDefName(a.methodOwner(idx)), a.blobAt(a.methods[idx-1].Signature)
	case 3: // MemberRef
		if idx == 0 || int(idx) > len(a.members) {
			return "?", nil
		}
		m := a.members[idx-1]
		var owner string
		switch m.Class & 7 {
		case 0:
			owner = a.typeDefName(m.Class >> 3)
		case 1:
			owner = a.typeRefName(m.Class >> 3)
		default:
			owner = "?"
		}
		return owner, a.blobAt(m.Signature)
	}
	return "?", nil
}

func (a *Assembly) parentName(c uint32) string {
	idx := c >> 5
	switch c & 31 {
	case 0:
		if idx == 0 || int(idx) > len(a.methods) {
			return "?"
		}
		return a.typeDefName(a.methodOwner(idx)) + "::" + a.str(a.methods[idx-1].Name)
	case 3:
		return a.typeDefName(idx)
	case 14:
		return "<assembly>"
	}
	return fmt.Sprintf("<parent tag %d>", c&31)
}

// Attributes decodes every custom attribute whose type name passes keep.
func (a *Assembly) Attributes(keep func(string) bool) []Attr {
	var out []Attr
	for _, ca := range a.attrs {
		typ, sig := a.ctorInfo(ca.Type)
		if !keep(typ) {
			continue
		}
		at := Attr{Type: typ, Owner: a.parentName(ca.Parent)}
		params, err := a.parseMethodSig(sig)
		if err != nil {
			at.Errors = "sig: " + err.Error()
			out = append(out, at)
			continue
		}
		r := &reader{b: a.blobAt(ca.Value)}
		if len(r.b) < 2 || r.u16() != 1 {
			at.Errors = "bad prolog"
			out = append(out, at)
			continue
		}
		for _, p := range params {
			v, err := r.value(p)
			if err != nil {
				at.Errors = err.Error()
				break
			}
			at.Fixed = append(at.Fixed, v)
		}
		if at.Errors == "" && r.left() >= 2 {
			n := int(r.u16())
			for i := 0; i < n && r.err == nil; i++ {
				r.u8() // FIELD / PROPERTY
				t := r.fieldOrPropType()
				name := r.serString()
				v, err := r.value(t)
				if err != nil {
					at.Errors = err.Error()
					break
				}
				at.Named = append(at.Named, NamedValue{Name: name.S, V: v})
			}
		}
		out = append(out, at)
	}
	return out
}

// sigType is a parameter type in a ctor signature, reduced to what attribute blobs need.
type sigType struct {
	elem byte // ECMA element type; 0x50 = System.Type; 0x55 = enum (int32 assumed)
	name string
	arr  *sigType
}

func (a *Assembly) parseMethodSig(sig []byte) ([]sigType, error) {
	if len(sig) < 3 {
		return nil, errors.New("short signature")
	}
	r := &reader{b: sig}
	r.u8() // calling convention
	n := r.compressed()
	a.sigParam(r) // return type (void)
	var ps []sigType
	for i := 0; i < int(n); i++ {
		ps = append(ps, a.sigParam(r))
	}
	return ps, r.err
}

func (a *Assembly) sigParam(r *reader) sigType {
	e := r.u8()
	switch e {
	case 0x12, 0x11: // CLASS / VALUETYPE
		name := a.typeDefOrRef(r.compressed())
		if name == "System.Type" {
			return sigType{elem: 0x50, name: name}
		}
		if e == 0x11 {
			return sigType{elem: 0x55, name: name}
		}
		return sigType{elem: 0x12, name: name}
	case 0x1d:
		el := a.sigParam(r)
		return sigType{elem: 0x1d, arr: &el}
	}
	return sigType{elem: e}
}

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) left() int { return len(r.b) - r.p }
func (r *reader) need(n int) bool {
	if r.err != nil || r.p+n > len(r.b) {
		if r.err == nil {
			r.err = errors.New("blob truncated")
		}
		return false
	}
	return true
}
func (r *reader) u8() byte {
	if !r.need(1) {
		return 0
	}
	r.p++
	return r.b[r.p-1]
}
func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	r.p += 2
	return binary.LittleEndian.Uint16(r.b[r.p-2:])
}
func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	r.p += 4
	return binary.LittleEndian.Uint32(r.b[r.p-4:])
}
func (r *reader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	r.p += 8
	return binary.LittleEndian.Uint64(r.b[r.p-8:])
}
func (r *reader) compressed() uint32 {
	if r.err != nil || r.p >= len(r.b) {
		r.err = errors.New("blob truncated")
		return 0
	}
	v, n := compressed(r.b[r.p:])
	if n == 0 {
		r.err = errors.New("bad compressed int")
	}
	r.p += n
	return v
}
func (r *reader) serString() Value {
	if r.need(1) && r.b[r.p] == 0xFF {
		r.p++
		return Value{Kind: "null"}
	}
	n := int(r.compressed())
	if !r.need(n) {
		return Value{}
	}
	s := r.b[r.p : r.p+n]
	r.p += n
	if !utf8.Valid(s) {
		r.err = errors.New("invalid utf8")
	}
	return Value{Kind: "string", S: string(s)}
}

// fieldOrPropType reads a FieldOrPropType (used by named args and boxed values).
func (r *reader) fieldOrPropType() sigType {
	e := r.u8()
	switch e {
	case 0x1d:
		el := r.fieldOrPropType()
		return sigType{elem: 0x1d, arr: &el}
	case 0x55:
		return sigType{elem: 0x55, name: r.serString().S}
	}
	return sigType{elem: e}
}

func (r *reader) value(t sigType) (Value, error) {
	var v Value
	switch t.elem {
	case 0x02:
		v = Value{Kind: "bool", S: fmt.Sprint(r.u8() != 0)}
	case 0x03:
		v = Value{Kind: "char", S: string(rune(r.u16()))}
	case 0x04:
		v = Value{Kind: "int", S: fmt.Sprint(int8(r.u8()))}
	case 0x05:
		v = Value{Kind: "int", S: fmt.Sprint(r.u8())}
	case 0x06:
		v = Value{Kind: "int", S: fmt.Sprint(int16(r.u16()))}
	case 0x07:
		v = Value{Kind: "int", S: fmt.Sprint(r.u16())}
	case 0x08:
		v = Value{Kind: "int", S: fmt.Sprint(int32(r.u32()))}
	case 0x09:
		v = Value{Kind: "int", S: fmt.Sprint(r.u32())}
	case 0x0a:
		v = Value{Kind: "int", S: fmt.Sprint(int64(r.u64()))}
	case 0x0b:
		v = Value{Kind: "int", S: fmt.Sprint(r.u64())}
	case 0x0c:
		v = Value{Kind: "float", S: fmt.Sprint(math.Float32frombits(r.u32()))}
	case 0x0d:
		v = Value{Kind: "float", S: fmt.Sprint(math.Float64frombits(r.u64()))}
	case 0x0e:
		v = r.serString()
	case 0x50:
		v = r.serString()
		if v.Kind == "string" {
			v.Kind = "type"
		}
	case 0x55:
		// Enums: Harmony's MethodType/ArgumentType/Priority are int32-backed.
		v = Value{Kind: "enum:" + t.name, S: fmt.Sprint(int32(r.u32()))}
	case 0x51, 0x1c: // boxed object
		return r.value(r.fieldOrPropType())
	case 0x1d:
		n := r.u32()
		if n == 0xFFFFFFFF {
			return Value{Kind: "null"}, r.err
		}
		v = Value{Kind: "array"}
		for i := uint32(0); i < n && r.err == nil; i++ {
			e, err := r.value(*t.arr)
			if err != nil {
				return v, err
			}
			v.Arr = append(v.Arr, e)
		}
	case 0x12:
		return v, fmt.Errorf("unsupported class param %s", t.name)
	default:
		return v, fmt.Errorf("unsupported elem 0x%x", t.elem)
	}
	return v, r.err
}

// UserStrings returns all #US heap strings (UTF-16).
func (a *Assembly) UserStrings() []string {
	var out []string
	b := a.us
	for i := 1; i < len(b); {
		n, sz := compressed(b[i:])
		if sz == 0 {
			break
		}
		i += sz
		if n == 0 {
			continue
		}
		end := i + int(n) - 1 // last byte is the "has special chars" flag
		if end > len(b) {
			break
		}
		var sb strings.Builder
		for j := i; j+1 < end+1 && j+1 < len(b) && j < end; j += 2 {
			sb.WriteRune(rune(binary.LittleEndian.Uint16(b[j:])))
		}
		out = append(out, sb.String())
		i += int(n)
	}
	return out
}

// HasMemberRef reports a MemberRef to owner::name (e.g. HarmonyLib.Harmony::Patch).
func (a *Assembly) HasMemberRef(owner, name string) bool {
	for _, m := range a.members {
		if m.Class&7 == 1 && a.str(m.Name) == name && a.typeRefName(m.Class>>3) == owner {
			return true
		}
	}
	return false
}
