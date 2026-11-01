package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The player's load-order rules in the plan editor (06 §2.5, 04 §6.1). Rules are global:
// every list and every game share them; the editor only lists, adds, changes and removes.

// RuleMod is a mod a rule names: its lineage reference for the commands, the copy the
// editor jumps to (the enabled one when there is one) and its name.
type RuleMod struct {
	Ref  string `json:"ref"`
	ID   string `json:"id,omitempty"` // an instance; empty when no copy is installed
	Name string `json:"name"`
	Pos  int    `json:"pos,omitempty"` // its place in the open list, 0 when not enabled
}

// RulePart is a piece of a rule's sentence: text, or the rule's mod 1 (A) or 2 (B).
type RulePart struct {
	Text string `json:"text,omitempty"`
	Mod  int    `json:"mod,omitempty"`
}

// RuleRow is one rule.
type RuleRow struct {
	ID      int64      `json:"id"`
	Kind    string     `json:"kind"`
	A       *RuleMod   `json:"a,omitempty"`
	B       *RuleMod   `json:"b,omitempty"`
	Target  string     `json:"target,omitempty"` // ignore: the constraint as sort prints it
	Parts   []RulePart `json:"parts"`
	Text    string     `json:"text"` // the parts as one line
	Note    string     `json:"note,omitempty"`
	Created string     `json:"created"`
	// InList: one of its mods is enabled in the current list (the editor's only the current list)
	InList bool `json:"inList"`
}

// RuleKind is a kind the editor's form offers; ignore rules come from Can it run and sort.
type RuleKind struct {
	Kind  string `json:"kind"`
	Label string `json:"label"` // with %s for the mods, as the row reads
	Two   bool   `json:"two"`   // names a second mod
}

// Rules is the editor's Load-order rules fold.
type Rules struct {
	Rows  []RuleRow  `json:"rows"`
	Kinds []RuleKind `json:"kinds"`
}

// ruleKinds in the form's order; the label is the row's sentence.
var ruleKinds = []RuleKind{
	{order.RuleAfter, "%s loads after %s", true},
	{order.RuleBefore, "%s loads before %s", true},
	{order.RuleAttach, "%s loads right after %s", true},
	{order.RuleIncompatible, "%s is incompatible with %s", true},
	{order.RuleTop, "%s loads as early as it can", false},
	{order.RuleBottom, "%s loads as late as it can", false},
}

// BuildRules lists the rules, newest first.
func BuildRules(a *app.App) (*Rules, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	rs, err := a.Store.Rules(st.Ix.Env.ID)
	if err != nil {
		return nil, err
	}
	out := &Rules{Rows: []RuleRow{}}
	for _, k := range ruleKinds {
		out.Kinds = append(out.Kinds, RuleKind{Kind: k.Kind, Label: T(k.Label), Two: k.Two})
	}
	for i := len(rs) - 1; i >= 0; i-- {
		out.Rows = append(out.Rows, ruleRow(st, rs[i]))
	}
	return out, nil
}

func ruleRow(st *app.LoadoutState, r *store.UserRule) RuleRow {
	row := RuleRow{ID: r.ID, Kind: r.Kind, Note: r.Note, Created: r.Created.Local().Format("2006-01-02 15:04")}
	if r.Kind == order.RuleIgnore {
		row.Target = st.Ix.DisplayConstraintRef(r.Target, func(l string) string { return ruleModOf(st, l).Ref })
		row.Parts = ignoreParts(st, r.Target, &row)
	} else {
		row.A = ruleModOf(st, r.A)
		if r.B != "" {
			row.B = ruleModOf(st, r.B)
		}
		format := r.Kind
		for _, k := range ruleKinds {
			if k.Kind == r.Kind {
				format = T(k.Label)
			}
		}
		row.Parts = splitParts(format, 1, 2)
	}
	var b strings.Builder
	for _, p := range row.Parts {
		switch p.Mod {
		case 1:
			b.WriteString(row.A.Name)
		case 2:
			b.WriteString(row.B.Name)
		default:
			b.WriteString(p.Text)
		}
	}
	row.Text = b.String()
	row.InList = row.A != nil && row.A.Pos > 0 || row.B != nil && row.B.Pos > 0
	return row
}

// splitParts cuts a format at its %s, numbering them with mods.
func splitParts(format string, mods ...int) []RulePart {
	var out []RulePart
	for i, s := range strings.Split(format, "%s") {
		if i > 0 && i-1 < len(mods) {
			out = append(out, RulePart{Mod: mods[i-1]})
		}
		if s != "" {
			out = append(out, RulePart{Text: s})
		}
	}
	return out
}

// ignoreParts says which constraint an ignore rule drops. The mod in it becomes A.
func ignoreParts(st *app.LoadoutState, target string, row *RuleRow) []RulePart {
	p := strings.Split(target, ":")
	pkgName := func(pkg string) string {
		pkg = strings.ToLower(pkg)
		for _, m := range st.Ix.Mods {
			if strings.ToLower(m.PackageID) == pkg && m.Status == identity.Live {
				return m.DisplayName()
			}
		}
		return pkg
	}
	switch {
	case p[0] == "decl" && len(p) == 4:
		row.A = ruleModOf(st, p[1])
		return splitParts(Tf("ignore %%s's About.xml: %s %s", declField(p[2]), pkgName(p[3])), 1)
	case p[0] == "official" && len(p) == 2:
		row.A = ruleModOf(st, p[1])
		return splitParts(T("ignore that %s loads after the official content"), 1)
	case p[0] == "community" && len(p) == 4 && p[3] == "" && strings.EqualFold(p[2], "loadtop"):
		return []RulePart{{Text: Tf("ignore the community rule: %s loads first", pkgName(p[1]))}}
	case p[0] == "community" && len(p) == 4 && p[3] == "" && strings.EqualFold(p[2], "loadbottom"):
		return []RulePart{{Text: Tf("ignore the community rule: %s loads last", pkgName(p[1]))}}
	case p[0] == "community" && len(p) == 4:
		return []RulePart{{Text: Tf("ignore the community rule: %s %s %s", pkgName(p[1]), declField(p[2]), pkgName(p[3]))}}
	}
	return []RulePart{{Text: T("ignore") + " " + row.Target}}
}

// declField is an About.xml field as its authors write it.
func declField(f string) string {
	switch strings.ToLower(f) {
	case "loadafter":
		return "loadAfter"
	case "loadbefore":
		return "loadBefore"
	case "forceloadafter":
		return "forceLoadAfter"
	case "forceloadbefore":
		return "forceLoadBefore"
	case "dependency":
		return "modDependencies"
	}
	return f
}

// ruleModOf is a lineage as a rule names it: the enabled copy, else the primary one.
func ruleModOf(st *app.LoadoutState, lineage string) *RuleMod {
	m := &RuleMod{Ref: st.Ix.LRef(lineage), Name: T("a mod that is no longer installed")}
	copies := st.Ix.ByLineage[lineage]
	var pick *store.Mod
	for _, c := range copies {
		if pos := st.Position(c.ID); pos > 0 && (m.Pos == 0 || pos < m.Pos) {
			pick, m.Pos = c, pos
		}
	}
	if pick == nil {
		if l := st.Ix.Lineages[lineage]; l != nil {
			pick = st.Ix.ByID[l.PrimaryID]
		}
	}
	if pick == nil && len(copies) > 0 {
		pick = copies[0]
	}
	if pick != nil {
		m.ID, m.Name = pick.ID, pick.DisplayName()
	}
	return m
}

// RuleCandidates are the mods the form's second field offers for text: installed mods,
// one per lineage, best match first, enabled before not enabled; not the rule's own mod.
func RuleCandidates(a *app.App, text, not string) ([]RuleMod, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(text))
	if q == "" {
		return []RuleMod{}, nil
	}
	skip := ""
	if not != "" {
		skip, _ = st.Ix.ResolveLineage(not)
	}
	type hit struct {
		m     *RuleMod
		score int
	}
	var hits []hit
	seen := map[string]bool{}
	for _, mod := range st.Ix.Mods {
		if mod.Status != identity.Live || mod.LineageID == skip || seen[mod.LineageID] {
			continue
		}
		name := strings.ToLower(mod.DisplayName())
		s := 0
		switch {
		case strings.HasPrefix(name, q):
			s = 3
		case strings.Contains(name, q) || strings.Contains(strings.ToLower(mod.PackageID), q):
			s = 2
		case Subsequence(q, name):
			s = 1
		}
		if s == 0 {
			continue
		}
		seen[mod.LineageID] = true
		hits = append(hits, hit{ruleModOf(st, mod.LineageID), s})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		pi, pj := hits[i].m.Pos, hits[j].m.Pos
		return pi > 0 && (pj == 0 || pi < pj)
	})
	out := []RuleMod{}
	for i, h := range hits {
		if i == 6 {
			break
		}
		out = append(out, *h.m)
	}
	return out, nil
}

// SaveRule adds a rule (id 0) or changes rule id, and says what it did. A changed ignore
// rule keeps its constraint: only its note is the player's to change here.
func SaveRule(a *app.App, id int64, spec app.RuleSpec) (string, error) {
	spec.Kind = strings.ToLower(strings.TrimSpace(spec.Kind))
	spec.Note = strings.TrimSpace(spec.Note)
	two := false
	for _, k := range ruleKinds {
		if k.Kind == spec.Kind {
			two = k.Two
		}
	}
	if !two {
		spec.B = ""
	}
	if spec.Kind != order.RuleIgnore && (spec.A == "" || two && spec.B == "") {
		return "", fmt.Errorf("%s", T("choose the other mod"))
	}
	var r *store.UserRule
	var err error
	if id == 0 {
		r, err = a.AddRule(spec)
	} else {
		r, err = a.UpdateRule(id, spec)
	}
	if err != nil {
		return "", err
	}
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	text := ruleRow(st, r).Text
	if id == 0 {
		return Tf("rule added: %s", text), nil
	}
	return Tf("rule changed: %s", text), nil
}

// RemoveRule deletes rule id, says which it was and returns it for RestoreRule.
func RemoveRule(a *app.App, id int64) (string, *store.UserRule, error) {
	st, err := a.Loadout()
	if err != nil {
		return "", nil, err
	}
	rs, err := a.Store.Rules(st.Ix.Env.ID)
	if err != nil {
		return "", nil, err
	}
	for _, r := range rs {
		if r.ID != id {
			continue
		}
		text := ruleRow(st, r).Text
		if _, err := a.Store.RemoveRule(st.Ix.Env.ID, id); err != nil {
			return "", nil, err
		}
		return Tf("rule removed: %s", text), r, nil
	}
	return "", nil, fmt.Errorf("no rule r:%d", id)
}

// RestoreRule puts a rule RemoveRule returned back, with its ID.
func RestoreRule(a *app.App, r *store.UserRule) (string, error) {
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	if err := a.Store.RestoreRule(r); err != nil {
		return "", err
	}
	return Tf("rule restored: %s", ruleRow(st, r).Text), nil
}
