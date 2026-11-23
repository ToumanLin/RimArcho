package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// Rows that only lack the game's version get the quiet version icon, a real warning on the
// same row still wins, and has:problem leaves version-only rows out (16 §5, §3.7).
func TestVersionOnlyRowsAreQuiet(t *testing.T) {
	t.Parallel()
	for _, reverse := range []bool{false, true} {
		its := []Need{
			{Counted: true, Status: StatusWarn, Fold: true, Mods: []string{"a", "b"}, code: "version.unsupported"},
			{Counted: true, Status: StatusWarn, Mods: []string{"b"}},
		}
		if reverse {
			its[0], its[1] = its[1], its[0]
		}
		n := &Needs{Items: its}
		indexNeeds(n)
		if n.Status["a"] != StatusVersion || n.Status["b"] != StatusWarn || n.Count != 2 {
			t.Fatalf("version row status: %+v", n.Status)
		}
		if r := n.Reasons["a"]; len(r) != 1 || r[0].Status != StatusVersion {
			t.Fatalf("version row reason: %+v", r)
		}
		if r := n.Reasons["b"]; len(r) != 2 || r[0].Status != StatusWarn {
			t.Fatalf("warning must come first: %+v", r)
		}
		q := ParseQuery("has:problem")
		if q.Match(&Item{ID: "a"}, MatchFacts{Status: n.Status}) || !q.Match(&Item{ID: "b"}, MatchFacts{Status: n.Status}) {
			t.Fatal("has:problem must skip version-only rows and keep warnings")
		}
	}
}

// Each missing entry's row icon offers removing every entry that is not installed (16 §5).
func TestMissingRowsOfferRemovingAll(t *testing.T) {
	t.Parallel()
	e, st := linkFixture(t)
	ps := &app.ProblemSet{}
	for _, pkg := range []string{"gone.one", "gone.two"} {
		ps.List = append(ps.List, &app.Problem{Finding: &finding.Finding{Code: "entry.missing", Severity: finding.Warning, Key: pkg,
			Subjects: []string{"pkg:" + pkg}}})
	}
	n := BuildNeeds(e.App, st, ps)
	for _, ref := range []string{"pkg:gone.one", "pkg:gone.two"} {
		rs := n.Reasons[ref]
		if len(rs) != 1 || rs[0].Status != StatusErr {
			t.Fatalf("%s reasons: %+v", ref, rs)
		}
		acts := rs[0].Actions
		last := acts[len(acts)-1]
		if last.Kind != ActDisable || len(last.Refs) != 2 {
			t.Fatalf("%s: the last action must remove both entries: %+v", ref, acts)
		}
		if own := acts[len(acts)-2]; own.Kind != ActDisable || len(own.Refs) != 1 {
			t.Fatalf("%s: its own removal must come before: %+v", ref, acts)
		}
	}
	// one missing entry: its own fixes, nothing about a group
	ps.List = ps.List[:1]
	n = BuildNeeds(e.App, st, ps)
	if rs := n.Reasons["pkg:gone.one"]; len(rs) != 1 || len(rs[0].Actions) == 0 || rs[0].Actions[len(rs[0].Actions)-1].Refs[0] == "" {
		t.Fatalf("single missing entry: %+v", rs)
	}
}
