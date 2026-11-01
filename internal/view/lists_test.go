package view_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestListsFilterCopiesAndGroups(t *testing.T) {
	e := libEnv(t)
	e.MustRun("plan", "rename", "default", "Main colony")
	e.MustRun("plan", "new", "Other colony", "--from", "current")
	e.MustRun("plan", "new", "Empty list")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	lock := libRow(t, libPage(t, e, ""), "Lockable")
	var enabled, spare string
	for _, i := range lock.Insts {
		if i.InPlan {
			enabled = i.Ref
		} else {
			spare = i.Ref
		}
	}
	if enabled == "" || spare == "" {
		t.Fatalf("expected enabled and spare copies: %+v", lock.Insts)
	}
	e.MustRun("plan", "use", "Other colony")
	e.MustRun("swap", enabled, spare)
	e.MustRun("stage", "add", spare)
	// Staged copies are not enabled by the list.
	if p := libPage(t, e, `lists:"Other colony"`); p.Shown != 0 {
		t.Fatalf("staged copy matched: %+v", p.Rows)
	}
	e.MustRun("enable", spare)
	e.MustRun("plan", "use", "Main colony")
	lib, err = e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, want string }{
		{`lists:"MAIN COLONY"`, enabled},
		{`lists:"Other colony"`, spare},
		{`-lists:"Main colony" name:Lockable`, spare},
		{`lists:"missing list"`, ""},
		{`lists:"Main colony" lists:"Other colony"`, ""},
		{`lists:"Empty list"`, ""},
		{`lists:"Other colony" -lists:"Main colony"`, spare},
		{`lists:"Main colony" id:` + spare, ""},
		{`lists:"Other colony" id:` + enabled, ""},
	} {
		p, err := view.BuildLibrary(lib, tc.query, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		var refs []string
		for _, row := range p.Rows {
			for _, i := range row.Insts {
				refs = append(refs, i.Ref)
			}
		}
		if strings.Join(refs, "") != tc.want {
			t.Errorf("%s: copies %v, want %s", tc.query, refs, tc.want)
		}
	}
	p, err := view.BuildLibrary(lib, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	g := p.Groups[0]
	if g.Key != "lists" || len(g.Values) != 3 || g.Values[0].Filter != "lists:Main colony" || !g.Values[0].Cur {
		t.Fatalf("lists group: %+v", g)
	}
	for _, v := range g.Values {
		want := 1
		if v.Filter == "lists:Empty list" {
			want = 0
		}
		if v.Count != want {
			t.Errorf("count %+v, want %d", v, want)
		}
	}
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	bar := view.FilterBoard(b, `lists:"Other colony" name:Lockable`, nil, view.MatchFacts{}, true, "1.6")
	if len(bar.Rows[0]) != 1 || b.Cols[0][bar.Rows[0][0]].Ref != spare || len(bar.Rows[1]) != 0 {
		t.Fatalf("editor did not match the exact available copy: %+v", bar.Rows)
	}
	for _, v := range bar.Groups[0].Values {
		if v.Filter == "lists:Main colony" {
			t.Fatal("editor offered the list being edited")
		}
	}
	for _, query := range []string{`lists:"Main colony" lists:"Other colony" name:Lockable`, `lists:"unknown list"`} {
		result := view.FilterBoard(b, query, nil, view.MatchFacts{}, false, "1.6")
		if len(result.Rows[0])+len(result.Rows[1]) != 0 {
			t.Errorf("editor %s matched %v", query, result.Rows)
		}
	}
	allGroups := view.FilterBoard(b, "", nil, view.MatchFacts{}, true, "1.6").Groups
	for _, v := range allGroups[0].Values {
		want := 2 // Core and the exact Lockable copy
		if v.Filter == "lists:Empty list" {
			want = 0
		}
		if v.Count != want {
			t.Errorf("editor list count %+v, want %d", v, want)
		}
	}
	for _, col := range b.Cols {
		for _, it := range col {
			if got := view.ParseQuery(`lists:"Main colony" -lists:"Other colony"`).Match(&it, view.MatchFacts{}); got != (it.Ref == enabled) {
				t.Errorf("query match for %s: %v", it.Ref, got)
			}
		}
	}
	if got := view.CompleteLibrary(`lists:"ma`, lib); got != `in colony"` {
		t.Errorf("quoted completion: %q", got)
	}
	if got := view.CompleteLibrary(`-lists:"Other co`, lib); got != `lony"` {
		t.Errorf("negative completion: %q", got)
	}
	// Opening another list must not change which list is marked current.
	other, err := e.App.Store.PlanByName("Other colony")
	if err != nil || other == nil {
		t.Fatalf("other list: %v", err)
	}
	e.App.OpenPlan = other.ID
	b, err = view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	bar = view.FilterBoard(b, "", nil, view.MatchFacts{}, true, "1.6")
	if g := bar.Groups[0]; g.Key != "lists" || len(g.Values) != 2 || !g.Values[0].Cur || g.Values[0].Filter != "lists:Main colony" {
		t.Fatalf("edited/current list distinction: %+v", g)
	}
	// CLI/MCP use the same app query through include/exclude arguments.
	result, err := lib.Filter(app.LibQuery{Include: []string{"lists:Main colony", "lists:Other colony"}})
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("repeated app filters: %+v, %v", result, err)
	}
	out := e.MustRun("library", "--include", "lists:Main colony", "--exclude", "lists:Other colony", "--all")
	if !strings.Contains(out, "Lockable") {
		t.Fatalf("CLI list membership: %s", out)
	}
	// Refresh membership after an edit, retaining the catalog's empty lists.
	e.App.OpenPlan = 0
	e.MustRun("disable", enabled)
	b, err = view.BuildBoard(e.App)
	if err != nil {
		t.Fatalf("membership refresh: %v", err)
	}
	bar = view.FilterBoard(b, `lists:"Main colony" name:Lockable`, nil, view.MatchFacts{}, true, "1.6")
	if len(bar.Rows[0])+len(bar.Rows[1]) != 0 {
		t.Fatalf("disabled copy retained membership: %+v", bar.Rows)
	}
}

func BenchmarkListsFilter1354(b *testing.B) {
	board := &view.Board{Lists: &app.ListMembership{Names: []string{"Main colony", "Other colony", "Empty list"}, Current: "Main colony", Edited: "Main colony"}}
	for i := 0; i < 1354; i++ {
		board.Cols[0] = append(board.Cols[0], view.Item{ID: fmt.Sprint(i), Name: fmt.Sprintf("Mod %04d", i), Lists: []string{"Other colony"}, Sources: []string{"workshop"}})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		view.FilterBoard(board, `lists:"Other colony"`, nil, view.MatchFacts{}, true, "1.6")
	}
}
