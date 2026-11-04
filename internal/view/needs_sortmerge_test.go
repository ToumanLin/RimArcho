package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// One mod out of order, a cycle, a weak preference and an old log (06 §5): the order item
// is the host, the cycle and the weak preference are its lines, and nothing else merges.
// Items made after the problem loop once took the last problem's code (the cycle's), so
// the order item merged into itself and vanished from the page.
func TestNeedsSortMergeEndToEnd(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	gamma := testkit.Sample("tester.gamma", "Gamma")
	gamma.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", gamma)
	e.AddWorkshop("1002", testkit.Sample("tester.user", "User"))
	after := func(pkg, name, other string) testkit.Mod {
		m := testkit.Sample(pkg, name)
		m.Files = map[string]string{"About/About.xml": strings.Replace(m.AboutXML(), "</ModMetaData>",
			"  <loadAfter><li>"+other+"</li></loadAfter>\n</ModMetaData>", 1)}
		return m
	}
	e.AddWorkshop("1003", after("tester.other", "Other", "tester.user"))
	e.AddWorkshop("1004", after("tester.cyca", "Cycle A", "tester.cycb"))
	e.AddWorkshop("1005", after("tester.cycb", "Cycle B", "tester.cyca"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.gamma", "brrainz.harmony", "tester.other", "tester.user", "tester.cyca", "tester.cycb")
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n\n"+
		"Could not resolve cross-reference: No Verse.ThingDef named Lib_Gun found to give to Verse.ThingDefCountClass Verse.ThingDefCountClass\n")
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
	var host *view.Need
	for i := range n.Items {
		it := &n.Items[i]
		if it.Kind == "order.violated" && it.Counted {
			if host != nil {
				t.Fatalf("two order items: %+v %+v", host, it)
			}
			host = it
		}
	}
	if host == nil || host.Into != "" || !strings.Contains(host.Text, "Gamma") || host.Section != view.SectionBreak {
		t.Fatalf("order item: %+v in %+v", host, n.Items)
	}
	if len(host.Also) != 2 || !strings.Contains(strings.Join(host.Also, "|"), "weaker order preferences") ||
		!strings.Contains(strings.Join(host.Also, "|"), "cycle of 2 mods") {
		t.Fatalf("also: %q", host.Also)
	}
	merged := 0
	for _, it := range n.Items {
		if it.Into == "" {
			continue
		}
		merged++
		if it.Into != host.ID || (it.Kind != "order.violated" && it.Kind != "order.cycle") {
			t.Errorf("merged by mistake: %+v", it)
		}
	}
	if merged != 2 {
		t.Errorf("merged %d items, want the weak preference and the cycle: %+v", merged, n.Items)
	}
}
