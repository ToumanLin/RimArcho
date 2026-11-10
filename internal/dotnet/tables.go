package dotnet

// Tables the IL tools read besides those of meta.go: parameter names (call-site tables, null
// traces), the assembly and its references, and member accessibility (assembly diff).

import (
	"fmt"

	"github.com/saferwall/pe"
)

type extTables struct {
	params  []pe.ParamTableRow
	asmRefs []pe.AssemblyRefTableRow
	asm     []pe.AssemblyTableRow
	declSec []pe.DeclSecurityTableRow // access.go
}

func (a *Assembly) readExt(f *pe.File) {
	a.ext.params = table[pe.ParamTableRow](f, pe.Param)
	a.ext.asmRefs = table[pe.AssemblyRefTableRow](f, pe.AssemblyRef)
	a.ext.asm = table[pe.AssemblyTableRow](f, pe.Assembly)
	a.ext.declSec = table[pe.DeclSecurityTableRow](f, pe.DeclSecurity)
}

// ParamNames returns the declared parameter names of MethodDef row (1-based), in order,
// without the return value; a parameter without a Param row has the name "".
func (a *Assembly) ParamNames(row uint32) []string {
	if row == 0 || int(row) > len(a.methods) {
		return nil
	}
	m, _ := a.Member(0x06<<24 | row)
	out := make([]string, m.Params)
	start := a.methods[row-1].ParamList
	end := uint32(len(a.ext.params) + 1)
	if int(row) < len(a.methods) {
		end = a.methods[row].ParamList
	}
	for p := start; p >= 1 && p < end && int(p) <= len(a.ext.params); p++ {
		r := a.ext.params[p-1]
		if r.Sequence >= 1 && int(r.Sequence) <= len(out) {
			out[r.Sequence-1] = a.str(r.Name)
		}
	}
	return out
}

// AsmRef is an assembly reference (or the assembly itself) with its version.
type AsmRef struct {
	Name    string
	Version string
}

func version(maj, min, build, rev uint16) string {
	return fmt.Sprintf("%d.%d.%d.%d", maj, min, build, rev)
}

// Identity returns the assembly's own name and version ("" if it has no Assembly row).
func (a *Assembly) Identity() AsmRef {
	if len(a.ext.asm) == 0 {
		return AsmRef{}
	}
	r := a.ext.asm[0]
	return AsmRef{a.str(r.Name), version(r.MajorVersion, r.MinorVersion, r.BuildNumber, r.RevisionNumber)}
}

// AssemblyRefs lists the referenced assemblies in table order.
func (a *Assembly) AssemblyRefs() []AsmRef {
	out := make([]AsmRef, 0, len(a.ext.asmRefs))
	for _, r := range a.ext.asmRefs {
		out = append(out, AsmRef{a.str(r.Name), version(r.MajorVersion, r.MinorVersion, r.BuildNumber, r.RevisionNumber)})
	}
	return out
}

// typeRefScope names the assembly a TypeRef row resolves in ("" for this module or unknown).
func (a *Assembly) typeRefScope(row uint32) string {
	for d := 0; row != 0 && int(row) <= len(a.typeRefs) && d < 8; d++ {
		sc := a.typeRefs[row-1].ResolutionScope
		switch sc & 3 {
		case 2:
			if i := sc >> 2; i >= 1 && int(i) <= len(a.ext.asmRefs) {
				return a.str(a.ext.asmRefs[i-1].Name)
			}
			return ""
		case 3: // nested: the enclosing TypeRef's scope
			row = sc >> 2
			continue
		}
		return ""
	}
	return ""
}

// Access names a member's or type's accessibility.
const (
	AccessPublic    = "public"
	AccessPrivate   = "private"
	AccessInternal  = "internal"
	AccessProtected = "protected"
	AccessOther     = "non-public"
)

func memberAccess(flags uint16) string {
	switch flags & 7 {
	case 6:
		return AccessPublic
	case 1:
		return AccessPrivate
	case 3:
		return AccessInternal
	case 4, 5:
		return AccessProtected
	}
	return AccessOther
}

// typeAccess: public for a public top-level type, or a nested public type in a public type.
func (a *Assembly) typeAccess(row uint32) string {
	for d := 0; row != 0 && int(row) <= len(a.typeDefs) && d < 8; d++ {
		switch a.typeDefs[row-1].Flags & 7 {
		case 1: // public
			return AccessPublic
		case 2: // nested public
			row = a.nestedIn[row]
			continue
		case 0, 5:
			return AccessInternal
		case 3:
			return AccessPrivate
		default:
			return AccessProtected
		}
	}
	return AccessOther
}

// MemberAccess finds a method (name and parameter count; params < 0: any) or field (field
// true) of type typ defined here and returns its accessibility, the declaring type's, and
// whether it was found.
func (a *Assembly) MemberAccess(typ, name string, field bool, params int) (member, typeAcc string, ok bool) {
	t := a.TypeRow(typ)
	if t == 0 {
		return "", "", false
	}
	typeAcc = a.typeAccess(t)
	if field {
		s := a.ilData()
		for i, f := range a.fields {
			if s.fieldOwner[i] == t && a.str(f.Name) == name {
				return memberAccess(f.Flags), typeAcc, true
			}
		}
		return "", typeAcc, false
	}
	for _, m := range a.Methods(t) {
		if m.Name == name && (params < 0 || m.Params == params) {
			return memberAccess(a.methods[m.Row-1].Flags), typeAcc, true
		}
	}
	return "", typeAcc, false
}

// IsStatic reports a static MethodDef.
func (a *Assembly) IsStatic(row uint32) bool {
	return row >= 1 && int(row) <= len(a.methods) && a.methods[row-1].Flags&0x10 != 0
}
