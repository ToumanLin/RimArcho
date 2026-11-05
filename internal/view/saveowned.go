package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// ownedConsequence says what loading a save without a Mod loses (10 §4.6), from what the
// Mod owns in it; without the full read (nil) it is lost, the generic sentence.
func ownedConsequence(o *app.SaveOwned, lost string) string {
	if o == nil {
		return lost
	}
	var b strings.Builder
	if o.Empty() {
		b.WriteString(T("nothing in the save is known to need it; what else it stored is removed when the save loads"))
	} else {
		b.WriteString(Tf("loading without it loses %s", strings.Join(ownedParts(o), T(", "))))
		if o.Inferred {
			b.WriteString(T("; inferred: nothing installed provides them"))
			if o.Among > 0 {
				b.WriteString(Tf("; they may belong to the other %s without facts instead", Plural(o.Among, "Mod")))
			}
		}
	}
	if o.DefsUnchecked {
		b.WriteString(T("; Defs, pawns and factions not checked: the list is not analysed for this game version"))
	}
	if o.ClassesUnchecked {
		b.WriteString(T("; classes not checked: the assemblies of the list or the game are not indexed"))
	}
	return b.String()
}

// ownedParts lists what it owns, most important first.
func ownedParts(o *app.SaveOwned) []string {
	var out []string
	for _, r := range o.Colonists {
		p := Tf("%s of race %s", Plural(r.Alive+r.Dead, "colonist"), r.Race)
		if r.Dead > 0 {
			p += Tf(" (%d dead)", r.Dead)
		}
		out = append(out, p)
	}
	if n := len(o.Factions); n > 0 {
		out = append(out, Plural(n, "faction")+" ("+strings.Join(headNames(o.Factions, 3), T(", "))+")")
	}
	if n := len(o.Classes); n > 0 {
		out = append(out, Plural(n, "class")+" ("+strings.Join(headNames(o.Classes, 3), T(", "))+")")
	}
	rest := 0
	for i, d := range o.Defs {
		switch {
		case i >= 3:
			rest += d.Uses
		case d.Type == "":
			out = append(out, Tf("%d other Def uses", d.Uses))
		default:
			out = append(out, Tf("%d uses of %s", d.Uses, d.Type))
		}
	}
	if rest > 0 {
		out = append(out, Tf("%d more Def uses", rest))
	}
	return out
}

func headNames(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(append([]string{}, s[:n]...), fmt.Sprintf(T("%d more"), len(s)-n))
}
