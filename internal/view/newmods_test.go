package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// New Mods (16 §2.5, §2.9): what a later scan finds is new for a week, first in Not
// enabled and found by added:new; the first scan's Mods and a new copy of an old Mod are not.
func TestNewMods(t *testing.T) {
	e := libEnv(t)
	newRows := func() []string {
		b, err := view.BuildBoard(e.App)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, col := range b.Cols {
			for _, it := range col {
				if it.New {
					out = append(out, it.Name)
				}
			}
		}
		return out
	}
	if got := newRows(); len(got) != 0 {
		t.Fatalf("the first scan's Mods are new: %v", got)
	}
	e.Advance(time.Hour)
	e.AddWorkshop("5700", testkit.Sample("tester.fresh", "Zeta Fresh"))
	e.MustRun("pin", "ws:5200") // a locked version of an old Mod: a new copy, not a new Mod
	e.MustRun("scan")
	if got := newRows(); len(got) != 1 || got[0] != "Zeta Fresh" {
		t.Fatalf("new rows = %v, want only Zeta Fresh", got)
	}
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if first := b.Cols[view.Inactive][0]; first.Name != "Zeta Fresh" {
		t.Fatalf("Not enabled starts with %q, not the new Mod", first.Name)
	}
	f := view.FilterBoard(b, "", []string{"added:new"}, view.MatchFacts{}, true, "")
	if rows := f.Rows[view.Inactive]; len(rows) != 1 || b.Cols[view.Inactive][rows[0]].Name != "Zeta Fresh" {
		t.Fatalf("added:new in the editor = %v", rows)
	}
	if f := view.FilterBoard(b, "added:new", nil, view.MatchFacts{}, false, ""); len(f.Rows[view.Inactive]) != 1 {
		t.Fatalf("typed added:new = %v", f.Rows[view.Inactive])
	}
	p := libPage(t, e, "", "added:new")
	if len(p.Rows) != 1 || !p.Rows[0].New || p.Rows[0].Added == "" {
		t.Fatalf("library added:new = %+v", p.Rows)
	}
	if out := e.MustRun("library", "--include", "added:new"); !strings.Contains(out, "Zeta Fresh") || !strings.Contains(out, "new") {
		t.Fatalf("archo library --include added:new:\n%s", out)
	}
	e.Advance(8 * 24 * time.Hour)
	if got := newRows(); len(got) != 0 {
		t.Fatalf("still new after a week: %v", got)
	}
	if p := libPage(t, e, "", "added:month"); len(p.Rows) == 0 {
		t.Fatal("added:month lost the Mod found nine days ago")
	}
}
