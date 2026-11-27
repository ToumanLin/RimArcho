package store

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func lookKeys(cs []LookChoice) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Scope+"|"+c.Look+"|"+c.Slot+"|"+c.Lineage)
	}
	return out
}

// 21 §3.1: one choice per scope, look and slot; choosing the entire look replaces its
// per-slot choices in that scope; a delete takes one look in one scope.
func TestLookChoices(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Unix(1790000000, 0).UTC()
	plan := LookScope(7)
	set := func(whole bool, cs ...LookChoice) {
		t.Helper()
		for i := range cs {
			cs[i].At = at
		}
		if err := s.SetLookChoices(cs, whole); err != nil {
			t.Fatal(err)
		}
	}
	set(false, LookChoice{Scope: plan, Look: "a/gun", Slot: "south", Lineage: "L1"}, LookChoice{Scope: plan, Look: "a/gun", Slot: "north", Lineage: "L1"})
	set(false, LookChoice{Scope: plan, Look: "a/gun", Slot: "south", Lineage: "L2"}) // replaces the row
	set(true, LookChoice{Scope: LookScopeAll, Look: "a/gun", Slot: "all", Lineage: "L3"})
	set(true, LookChoice{Scope: plan, Look: "a/hat", Slot: "all", Lineage: "L1"})
	got, err := s.LookChoices(plan, LookScopeAll)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"7|a/gun|north|L1", "7|a/gun|south|L2", "7|a/hat|all|L1", "all|a/gun|all|L3"}
	if !reflect.DeepEqual(lookKeys(got), want) || !got[0].At.Equal(at) {
		t.Fatalf("choices = %v (%v)", lookKeys(got), got[0].At)
	}
	if only, _ := s.LookChoices(LookScopeAll); len(only) != 1 || only[0].Lineage != "L3" {
		t.Fatalf("scope all = %v", lookKeys(only))
	}

	set(true, LookChoice{Scope: plan, Look: "a/gun", Slot: "all", Lineage: "L9"})
	got, _ = s.LookChoices(plan)
	if want := []string{"7|a/gun|all|L9", "7|a/hat|all|L1"}; !reflect.DeepEqual(lookKeys(got), want) {
		t.Fatalf("the entire look did not replace its slots: %v", lookKeys(got))
	}

	if n, err := s.DeleteLookChoices(plan, "a/gun"); err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	got, _ = s.LookChoices()
	if want := []string{"7|a/hat|all|L1", "all|a/gun|all|L3"}; !reflect.DeepEqual(lookKeys(got), want) {
		t.Fatalf("after delete: %v", lookKeys(got))
	}
}

// 21 §4: look_file is the whole set of files in _Archo_Looks, replaced at once.
func TestLookFiles(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := []LookFile{
		{Rel: "Textures/b.png", Source: `D:\m\b.png`, Size: 2, MTime: 20, Linked: false},
		{Rel: "Textures/a.png", Source: `D:\m\a.png`, Size: 1, MTime: 10, Linked: true},
	}
	if err := s.ReplaceLookFiles(a); err != nil {
		t.Fatal(err)
	}
	got, err := s.LookFiles()
	if err != nil || !reflect.DeepEqual(got, []LookFile{a[1], a[0]}) {
		t.Fatalf("files = %+v %v", got, err)
	}
	if err := s.ReplaceLookFiles(a[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LookFiles(); !reflect.DeepEqual(got, a[:1]) {
		t.Fatalf("replace keeps old rows: %+v", got)
	}
	if err := s.ReplaceLookFiles(nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LookFiles(); len(got) != 0 {
		t.Fatalf("not emptied: %+v", got)
	}
}

// 04 §4: schema 46 adds the two tables to a schema 45 database, and a deleted plan takes
// its choices with it (plan IDs are reused).
func TestUpgradeLooksAndPlanDelete(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`DROP TABLE look_choice; DROP TABLE look_file; UPDATE meta SET value='45' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.Meta("schema_version"); got != "46" {
		t.Fatalf("schema = %s", got)
	}
	a, err := s.CreatePlan("", "one", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreatePlan("", "two", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	cs := []LookChoice{
		{Scope: LookScope(a.ID), Look: "x", Slot: "all", Lineage: "L1"},
		{Scope: LookScope(b.ID), Look: "x", Slot: "all", Lineage: "L1"},
		{Scope: LookScopeAll, Look: "x", Slot: "all", Lineage: "L1"},
	}
	if err := s.SetLookChoices(cs, true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePlan(a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.LookChoices()
	if want := []string{LookScope(b.ID) + "|x|all|L1", "all|x|all|L1"}; !reflect.DeepEqual(lookKeys(got), want) {
		t.Fatalf("after deleting the plan: %v", lookKeys(got))
	}
}
