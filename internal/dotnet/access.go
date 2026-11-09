package dotnet

// Runtime member access (05 §5.6): whether the game's Mono will let an assembly use the
// non-public members it references. A Mod built against a publicized Assembly-CSharp compiles
// calls to private and protected game members; Mono then throws FieldAccessException or
// MethodAccessException unless the assembly skips verification. csc marks an assembly that way
// when it compiles with /unsafe (AllowUnsafeBlocks), which is how Krafs.Publicizer's default
// build works; IgnoresAccessChecksTo alone is not enough in the game's Mono (MCP eval h8).

import (
	"bytes"
	"sort"
)

// SkipsVerification reports an assembly-level security permission with SkipVerification.
func (a *Assembly) SkipsVerification() bool {
	for _, d := range a.ext.declSec {
		// HasDeclSecurity coded index: tag 2 is the Assembly table
		if d.Parent&3 == 2 && bytes.Contains(a.blobAt(d.PermissionSet), []byte("SkipVerification")) {
			return true
		}
	}
	return false
}

// IgnoresAccessChecksTo lists the assemblies an IgnoresAccessChecksTo attribute names.
func (a *Assembly) IgnoresAccessChecksTo() []string {
	_, ignores := a.attrTexts()
	return ignores
}

// derives reports whether TypeDef row has owner as a base, directly or through other types
// of this assembly, or is nested in a type that does (a nested type, such as a lambda's
// closure class, sees what its enclosing type sees).
func (a *Assembly) derives(row uint32, owner string) bool {
	for n := 0; row != 0 && n < 8; n++ {
		for t, d := row, 0; t != 0 && d < 16; d++ {
			ext := a.typeDefs[t-1].Extends
			if ext == 0 {
				break
			}
			if a.typeDefOrRef(ext) == owner {
				return true
			}
			if ext&3 != 0 { // not a TypeDef of this assembly: the chain leaves it
				break
			}
			t = ext >> 2
		}
		row = a.nestedIn[row]
	}
	return false
}

// refUsers maps a MemberRef row to the TypeDef rows of the methods whose bodies use it.
func (a *Assembly) refUsers() map[uint32]map[uint32]bool {
	out := map[uint32]map[uint32]bool{}
	for row := uint32(1); int(row) <= len(a.methods); row++ {
		var owner uint32
		for _, in := range DecodeIL(a.Body(row)) {
			if !tokenOperand(in.Op) || uint32(in.Arg)>>24 != 0x0A {
				continue
			}
			if owner == 0 {
				owner = a.methodOwner(row)
			}
			r := uint32(in.Arg) & 0xFFFFFF
			if out[r] == nil {
				out[r] = map[uint32]bool{}
			}
			out[r][owner] = true
		}
	}
	return out
}

// InaccessibleRefs lists the member references the runtime refuses unless the assembly skips
// verification: a private, internal or other non-public member, or one of a non-public type,
// of an assembly open returns. A protected member is left out when every method that uses it
// belongs to a type deriving from its owner (an override calling its base); a Harmony patch
// class reading a protected field is not one. References into assemblies open cannot provide
// are skipped.
func (a *Assembly) InaccessibleRefs(open func(name string) *Assembly) []RefDiff {
	var out []RefDiff
	var users map[uint32]map[uint32]bool
	for _, d := range a.memberRefs() {
		if !resolveAccess(&d, open) || !d.NonPublic() {
			continue
		}
		if d.Access == AccessProtected && (d.TypeAccess == "" || d.TypeAccess == AccessPublic) {
			if users == nil {
				users = a.refUsers()
			}
			inSub := len(users[d.row]) > 0
			for t := range users[d.row] {
				if !a.derives(t, d.Owner) {
					inSub = false
					break
				}
			}
			if inSub {
				continue
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	return out
}

// resolveAccess fills d's accessibility from its scope assembly; false when that assembly is
// not available or has no such member.
func resolveAccess(d *RefDiff, open func(name string) *Assembly) bool {
	if d.Scope == "" || open == nil {
		return false
	}
	t := open(d.Scope)
	if t == nil {
		return false
	}
	acc, tacc, ok := t.MemberAccess(d.Owner, d.Name, d.Field, d.Params)
	d.Access, d.TypeAccess, d.Missing = acc, tacc, !ok
	return ok
}
