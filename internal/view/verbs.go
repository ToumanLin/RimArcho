package view

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Verbs (06 §13.6): each acts at once on the object (the selection, else the cursor's
// row) and is one undo step. The interfaces decide which key, click or drop runs them.

// Result is what a verb did, for the status line; Placed are the mods it put into the list
// (the next "m changes it" applies to them).
type Result struct {
	Text   string   `json:"text"`
	Placed []string `json:"placed,omitempty"`
}

// Positions: the 1-based load positions of the mods that are in the list.
func Positions(st *app.LoadoutState, ids []string) []int {
	var ps []int
	for _, id := range ids {
		if p := st.Position(id); p > 0 {
			ps = append(ps, p)
		}
	}
	sort.Ints(ps)
	return ps
}

// PlacedText: "placed at position 352 · m changes it" for the mods now at positions ps.
func PlacedText(ps []int) string {
	if len(ps) == 0 {
		return ""
	}
	sort.Ints(ps)
	where := fmt.Sprint(ps[0])
	if len(ps) > 1 {
		where = fmt.Sprintf("%d–%d", ps[0], ps[len(ps)-1])
	}
	return Tf("placed at position %s · m changes it", where)
}

// NowAt: "Alpha is now number 3" / "3 mods are now at 3–5".
func NowAt(st *app.LoadoutState, what string, ids []string) string {
	ps := Positions(st, ids)
	switch len(ps) {
	case 0:
		return ""
	case 1:
		return Tf("%s is now number %d", what, ps[0])
	}
	return Tf("%s are now at %d–%d", what, ps[0], ps[len(ps)-1])
}

// Named: the mod's name for one, "3 mods" for several.
func Named(st *app.LoadoutState, ids []string) string {
	if len(ids) == 1 && st != nil {
		if mod := st.Ix.ByID[ids[0]]; mod != nil {
			return mod.DisplayName()
		}
	}
	return Plural(len(ids), "mod")
}

// Plural: "1 mod" / "3 mods" (English), "3 个Mod" (Chinese, T(noun)).
func Plural(n int, noun string) string {
	if lang == "zh" {
		return fmt.Sprintf("%d 个%s", n, T(noun))
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Object is what a verb acts on: the instances, their CLI references (a missing list entry
// has a pkg: reference and no instance) and its name for the status line.
type Object struct {
	IDs  []string `json:"ids"`
	Refs []string `json:"refs"`
	What string   `json:"what"`
}

// ObjectOf makes the object of instances.
func ObjectOf(st *app.LoadoutState, ids []string) Object {
	o := Object{IDs: ids, What: Named(st, ids)}
	for _, id := range ids {
		o.Refs = append(o.Refs, st.Ix.Ref(id))
	}
	return o
}

// Toggle is space: enable where the load order wants it (from Disabled), disable (from
// Enabled), or put back where it was (from Staging).
func Toggle(a *app.App, from Column, o Object) (*Result, error) {
	ids, refs := o.IDs, o.Refs
	switch from {
	case Inactive:
		at, err := a.SuggestAt(ids)
		if err == nil {
			_, err = a.Enable(refs, app.Placement{At: at})
		}
		if err != nil {
			return nil, err
		}
		return placed(a, ids)
	case Staging:
		if _, err := a.Restore(refs, app.Placement{}); err != nil {
			return nil, err
		}
		return placed(a, ids)
	}
	if _, err := a.Disable(refs); err != nil {
		return nil, err
	}
	return &Result{Text: Tf("disabled %s", o.What)}, nil
}

func placed(a *app.App, ids []string) (*Result, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	return &Result{Text: PlacedText(Positions(st, ids)), Placed: ids}, nil
}

// Stage is t: stage the mods; from Staging put them back where they were.
func Stage(a *app.App, from Column, o Object) (*Result, error) {
	if from == Staging {
		if _, err := a.Restore(o.Refs, app.Placement{}); err != nil {
			return nil, err
		}
		return placed(a, o.IDs)
	}
	if _, err := a.Stage(o.Refs); err != nil {
		return nil, err
	}
	return &Result{Text: Tf("staged %s", o.What)}, nil
}

// Unstage takes mods out of staging without enabling them (a drop on Disabled).
func Unstage(a *app.App, o Object) (*Result, error) {
	if _, err := a.Unstage(o.Refs); err != nil {
		return nil, err
	}
	return &Result{Text: Tf("took %s out of staging", o.What)}, nil
}

// MoveBy is J / K: move enabled mods d places as one block (a selection that is not
// contiguous first gathers at its first mod's place).
func MoveBy(a *app.App, o Object, d int) (*Result, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ps := Positions(st, o.IDs)
	if len(ps) == 0 {
		return nil, fmt.Errorf("%s", T("only enabled mods can be moved"))
	}
	rest := len(st.List) - len(o.IDs)
	return Place(a, Active, o, max(1, min(ps[0]+d, rest+1)))
}

// Place puts mods at a 1-based place in the list without them: enabled mods move as one
// block, the others are enabled there (and leave staging). One undo step.
func Place(a *app.App, from Column, o Object, at int) (*Result, error) {
	var err error
	if from == Active {
		_, err = a.MoveBlock(o.IDs, at)
	} else {
		_, err = a.Enable(o.Refs, app.Placement{At: at})
		if err == nil && from == Staging {
			_, err = a.Unstage(o.Refs)
		}
	}
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	return &Result{Text: NowAt(st, o.What, o.IDs)}, nil
}

// ---- the position input (06 §13.7) ----

// PosOpt is one place the position input offers.
type PosOpt struct {
	Label string `json:"label"`          // "after  Harmony"
	Note  string `json:"note,omitempty"` // "(position 1)"
	At    int    `json:"at"`             // 1-based, in the list without the moving mods
	Edge  bool   `json:"edge,omitempty"` // first / last
}

// PosOptions: a number is "position N"; a name lists the enabled mods it matches ("after
// X", or "before X"); then the suggested place, then first and last.
func PosOptions(a *app.App, st *app.LoadoutState, ids []string, text string, before bool) []PosOpt {
	moving := map[string]bool{}
	for _, id := range ids {
		moving[id] = true
	}
	type ent struct {
		name string
		pos  int // shown: its number now
		at   int // its place in the list without the moving mods, 1-based
	}
	var rest []ent
	for i, e := range st.List {
		if moving[e.Instance] && e.Instance != "" {
			continue
		}
		name := e.Missing
		if mod := st.Ix.ByID[e.Instance]; mod != nil {
			name = mod.DisplayName()
		}
		rest = append(rest, ent{name, i + 1, len(rest) + 1})
	}
	n := len(rest)
	var opts []PosOpt
	q := strings.ToLower(strings.TrimSpace(text))
	if v, err := strconv.Atoi(q); err == nil && v > 0 {
		v = min(v, n+1)
		opts = append(opts, PosOpt{Label: Tf("position %d", v), At: v})
	} else if q != "" {
		type hit struct {
			e     ent
			score int
		}
		var hits []hit
		for _, e := range rest {
			l := strings.ToLower(e.name)
			s := 0
			switch {
			case strings.HasPrefix(l, q):
				s = 3
			case strings.Contains(l, q):
				s = 2
			case Subsequence(q, l):
				s = 1
			}
			if s > 0 {
				hits = append(hits, hit{e, s})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		for i, h := range hits {
			if i == 5 {
				break
			}
			o := PosOpt{Label: T("after") + "  " + h.e.name, Note: Tf("(position %d)", h.e.pos), At: h.e.at + 1}
			if before {
				o.Label, o.At = T("before")+"  "+h.e.name, h.e.at
			}
			opts = append(opts, o)
		}
	}
	if at, err := a.SuggestAt(ids); err == nil {
		opts = append(opts, PosOpt{Label: Tf("position %d", at), Note: T("(suggested)"), At: at})
	}
	return append(opts, PosOpt{Label: T("first"), At: 1, Edge: true}, PosOpt{Label: T("last"), At: n + 1, Edge: true})
}

// KeepOfficial is T: the enabled list keeps only Core and the DLCs, in one undo step.
func KeepOfficial(a *app.App) (*Result, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, e := range st.List {
		if e.Instance == "" {
			refs = append(refs, "pkg:"+e.Missing)
			continue
		}
		if m := st.Ix.ByID[e.Instance]; m != nil && m.Kind != source.Official {
			refs = append(refs, st.Ix.Ref(m.ID))
		}
	}
	if len(refs) == 0 {
		return &Result{Text: T("only Core and the DLCs are enabled already")}, nil
	}
	if _, err := a.Disable(refs); err != nil {
		return nil, err
	}
	return &Result{Text: Tf("only Core and the DLCs stay enabled (%d mods disabled)", len(refs))}, nil
}
