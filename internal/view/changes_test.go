package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.9: 变化 narrowed to one mod lists the changes of every copy of its Lineage and
// nothing else; without it, every mod's.
func TestChangesOfLineage(t *testing.T) {
	e := libEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	lock := libRow(t, libPage(t, e, ""), "Lockable")
	id := lib.Instance(lock.Insts[0].Ref).Mod.ID
	if ids := view.LineageIDs(st.Ix, id); len(ids) != 2 {
		t.Fatalf("Lockable's lineage: %v", ids)
	}
	all, err := view.Changes(e.App, st, "")
	if err != nil {
		t.Fatal(err)
	}
	of, err := view.Changes(e.App, st, id)
	if err != nil {
		t.Fatal(err)
	}
	count := func(cs []view.ChangeScan) (n int, others bool) {
		for _, c := range cs {
			for _, ev := range c.Events {
				n++
				if ev.Mod != "Lockable" {
					others = true
				}
			}
		}
		return
	}
	nAll, _ := count(all)
	nOf, others := count(of)
	if nOf != 2 || others || nAll <= nOf {
		t.Fatalf("lineage: %d changes (others %v), every mod: %d", nOf, others, nAll)
	}
	if got, _ := view.Changes(e.App, st, "nope"); got != nil {
		t.Fatalf("unknown mod: %+v", got)
	}
}
