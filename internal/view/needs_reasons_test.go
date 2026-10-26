package view_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Every row icon comes with what it is (06 §4.2): the badge's popover lists the items
// behind it, worst first, and the first one has the icon's status.
func TestRowIconsHaveReasons(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	dir := e.AddWorkshop("1002", testkit.Sample("tester.h", "Hidden One"))
	e.Move(filepath.Join(dir, "About", "About.xml"), filepath.Join(dir, "About", "About.xml.disabled"))
	e.WriteModsConfig("1.6.4633 rev1261", "brrainz.harmony", "ludeon.rimworld", "tester.gone")
	e.AddEnv(false)
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, b.St, ps)
	if len(n.Status) == 0 {
		t.Fatalf("no row icons: %+v", n.Items)
	}
	// The pinned popover offers 能不能跑's actions: a reason has the actions of its item.
	acts := 0
	for _, rs := range n.Reasons {
		for _, r := range rs {
			acts += len(r.Actions)
			for _, a := range r.Actions {
				if a.Kind == "fold" {
					t.Fatalf("the popover has nothing to unfold: %+v", r)
				}
			}
		}
	}
	if acts == 0 {
		t.Fatalf("no reason carries an action: %+v", n.Reasons)
	}
	for id, s := range n.Status {
		rs := n.Reasons[id]
		if len(rs) == 0 || rs[0].Status != s || rs[0].Text == "" {
			t.Fatalf("%s: icon %d, reasons %+v", id, s, rs)
		}
		for i := 1; i < len(rs); i++ {
			if rs[i].Status > rs[i-1].Status {
				t.Fatalf("%s: reasons not worst first: %+v", id, rs)
			}
		}
	}
}
