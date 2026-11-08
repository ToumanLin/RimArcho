package dotnet

// Assembly diff (archo source diff, read_source mode=diff; 05 §5.4): what a rebuild changed
// besides the methods meant to change. Method bodies compare as instruction streams with
// tokens resolved to names, so renumbered tokens do not count as changes.

import (
	"fmt"
	"sort"
	"strings"
)

// RefDiff is one member reference added or removed.
type RefDiff struct {
	Text   string // Owner::Name(Types) or Owner::Name for fields
	Owner  string
	Name   string
	Field  bool
	Params int
	Scope  string // the assembly the owner type resolves in ("" unknown or this one)
	// Access: the target's accessibility when resolved (public, internal, private, protected),
	// TypeAccess the declaring type's; "" when the target assembly was not available.
	Access, TypeAccess string
	Missing            bool // the target assembly was read but has no such member
}

// NonPublic reports a resolved target that is not public, or whose type is not.
func (r RefDiff) NonPublic() bool {
	return r.Access != "" && r.Access != AccessPublic || r.TypeAccess != "" && r.TypeAccess != AccessPublic
}

// DiffReport is the difference between two builds of an assembly.
type DiffReport struct {
	Old, New                 AsmRef
	Changed, Added, Removed  []string // methods by Type::Name(Types)
	TypesAdded, TypesRemoved []string
	RefsAdded, RefsRemoved   []RefDiff
	AsmRefs                  []string // "+ Name 1.0.0.0", "- Name …", "~ Name 1.0 → 2.0"
	AttrsAdded, AttrsRemoved []string
	IgnoresAccessChecksTo    []string // assemblies the new build declares IgnoresAccessChecksTo for
	Unchanged                int      // methods with identical bodies
}

// methodKey is Type::Name(Types) of MethodDef row.
func (a *Assembly) methodKey(row uint32) string {
	m, _ := a.Member(0x06<<24 | row)
	return m.Owner + "::" + m.Name + "(" + strings.Join(m.Types, ", ") + ")"
}

// normBody renders a body as text with tokens resolved and branches as instruction indexes.
func (a *Assembly) normBody(row uint32) string {
	ins := DecodeIL(a.Body(row))
	idx := map[int]int{}
	for i, in := range ins {
		idx[in.Off] = i
	}
	var sb strings.Builder
	for _, in := range ins {
		sb.WriteString(OpName(in.Op))
		switch {
		case tokenOperand(in.Op) || in.Op == 0x72:
			sb.WriteString(" " + a.TokenText(uint32(in.Arg)))
			if m, ok := a.Member(uint32(in.Arg)); ok && !m.Field && (in.Arg>>24 == 0x06 || in.Arg>>24 == 0x0A || in.Arg>>24 == 0x2B) {
				sb.WriteString("(" + strings.Join(m.Types, ",") + ")")
			}
		case IsBranch(in.Op):
			fmt.Fprintf(&sb, " #%d", idx[int(in.Arg)])
		case len(in.Targets) > 0:
			for _, t := range in.Targets {
				fmt.Fprintf(&sb, " #%d", idx[t])
			}
		default:
			fmt.Fprintf(&sb, " %d", in.Arg)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (a *Assembly) memberRefs() map[string]RefDiff {
	out := map[string]RefDiff{}
	for r := uint32(1); int(r) <= len(a.members); r++ {
		m, ok := a.Member(0x0A<<24 | r)
		if !ok {
			continue
		}
		d := RefDiff{Owner: m.Owner, Name: m.Name, Field: m.Field, Params: m.Params}
		if m.Field {
			d.Text = m.Owner + "::" + m.Name
			d.Params = -1
		} else {
			d.Text = m.Owner + "::" + m.Name + "(" + strings.Join(m.Types, ", ") + ")"
		}
		if c := a.members[r-1].Class; c&7 == 1 {
			d.Scope = a.typeRefScope(c >> 3)
		}
		out[d.Text] = d
	}
	return out
}

func (a *Assembly) attrTexts() (map[string]bool, []string) {
	out := map[string]bool{}
	var ignores []string
	for _, at := range a.Attributes(func(string) bool { return true }) {
		var args []string
		for _, v := range at.Fixed {
			args = append(args, v.S)
		}
		for _, n := range at.Named {
			args = append(args, n.Name+"="+n.V.S)
		}
		t := at.Type + " on " + at.Owner
		if at.Errors != "" {
			t += " (arguments not decoded)"
		} else if len(args) > 0 {
			t += " (" + strings.Join(args, ", ") + ")"
		}
		out[t] = true
		if strings.HasSuffix(at.Type, ".IgnoresAccessChecksToAttribute") && len(at.Fixed) > 0 {
			ignores = append(ignores, at.Fixed[0].S)
		}
	}
	return out, ignores
}

func diffSets(old, cur map[string]bool) (added, removed []string) {
	for k := range cur {
		if !old[k] {
			added = append(added, k)
		}
	}
	for k := range old {
		if !cur[k] {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return
}

// Diff compares two builds. open, if not nil, returns a referenced assembly by name (nil if
// unavailable); added member references to it get their target's accessibility.
func Diff(old, cur *Assembly, open func(name string) *Assembly) *DiffReport {
	r := &DiffReport{Old: old.Identity(), New: cur.Identity()}
	bodies := func(a *Assembly) map[string]string {
		out := map[string]string{}
		for row := uint32(1); int(row) <= len(a.methods); row++ {
			out[a.methodKey(row)] = a.normBody(row)
		}
		return out
	}
	ob, nb := bodies(old), bodies(cur)
	for k, b := range nb {
		o, ok := ob[k]
		switch {
		case !ok:
			r.Added = append(r.Added, k)
		case o != b:
			r.Changed = append(r.Changed, k)
		default:
			r.Unchanged++
		}
	}
	for k := range ob {
		if _, ok := nb[k]; !ok {
			r.Removed = append(r.Removed, k)
		}
	}
	sort.Strings(r.Added)
	sort.Strings(r.Changed)
	sort.Strings(r.Removed)
	types := func(a *Assembly) map[string]bool {
		out := map[string]bool{}
		for i := range a.typeDefs {
			out[a.typeDefName(uint32(i+1))] = true
		}
		return out
	}
	r.TypesAdded, r.TypesRemoved = diffSets(types(old), types(cur))

	or, nr := old.memberRefs(), cur.memberRefs()
	for k, d := range nr {
		if _, ok := or[k]; !ok {
			r.RefsAdded = append(r.RefsAdded, d)
		}
	}
	for k, d := range or {
		if _, ok := nr[k]; !ok {
			r.RefsRemoved = append(r.RefsRemoved, d)
		}
	}
	sort.Slice(r.RefsAdded, func(i, j int) bool { return r.RefsAdded[i].Text < r.RefsAdded[j].Text })
	sort.Slice(r.RefsRemoved, func(i, j int) bool { return r.RefsRemoved[i].Text < r.RefsRemoved[j].Text })
	if open != nil {
		for i := range r.RefsAdded {
			d := &r.RefsAdded[i]
			if d.Scope == "" {
				continue
			}
			t := open(d.Scope)
			if t == nil {
				continue
			}
			acc, tacc, ok := t.MemberAccess(d.Owner, d.Name, d.Field, d.Params)
			d.Access, d.TypeAccess, d.Missing = acc, tacc, !ok
		}
	}

	oa, na := map[string]AsmRef{}, map[string]AsmRef{}
	for _, x := range old.AssemblyRefs() {
		oa[x.Name] = x
	}
	for _, x := range cur.AssemblyRefs() {
		na[x.Name] = x
	}
	for n, x := range na {
		if o, ok := oa[n]; !ok {
			r.AsmRefs = append(r.AsmRefs, "+ "+n+" "+x.Version)
		} else if o.Version != x.Version {
			r.AsmRefs = append(r.AsmRefs, "~ "+n+" "+o.Version+" → "+x.Version)
		}
	}
	for n, x := range oa {
		if _, ok := na[n]; !ok {
			r.AsmRefs = append(r.AsmRefs, "- "+n+" "+x.Version)
		}
	}
	sort.Strings(r.AsmRefs)

	oat, _ := old.attrTexts()
	nat, ignores := cur.attrTexts()
	r.AttrsAdded, r.AttrsRemoved = diffSets(oat, nat)
	r.IgnoresAccessChecksTo = ignores
	return r
}
