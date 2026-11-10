package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Two incompatible mods, one of them a library other mods need (one directly, one through
// that mod): the option to disable the mod nothing needs comes first, and the library's
// option says how many enabled mods need it. A dependency missing before does not count.
func TestDisableOneWeighsDependents(t *testing.T) {
	t.Parallel()
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2001", testkit.Mod{PackageID: "tester.lib", Name: "Lib"})
	e.AddWorkshop("2002", testkit.Mod{PackageID: "tester.solo", Name: "Solo"})
	e.AddWorkshop("2003", testkit.Mod{PackageID: "tester.uses", Name: "Uses Lib", Deps: []string{"tester.lib"}})
	e.AddWorkshop("2004", testkit.Mod{PackageID: "tester.addon", Name: "Addon", Deps: []string{"tester.uses"}})
	e.AddWorkshop("2005", testkit.Mod{PackageID: "tester.broken", Name: "Broken", Deps: []string{"tester.gone"}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "tester.lib", "tester.solo", "tester.uses", "tester.addon", "tester.broken")
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	lib, _ := st.Ix.ResolveOne("pkg:tester.lib")
	solo, _ := st.Ix.ResolveOne("pkg:tester.solo")
	broken, _ := st.Ix.ResolveOne("pkg:tester.broken")
	if deps, err := e.App.DependentsOf(st, []string{broken.ID}); err != nil || len(deps) != 0 {
		t.Fatalf("broken: %v %v", deps, err)
	}
	ps := &app.ProblemSet{List: []*app.Problem{{Finding: &finding.Finding{Code: "incompat.active", Severity: finding.Error,
		Subjects: []string{finding.Inst(lib.ID), finding.Inst(solo.ID)}}}}}
	var act *Action
	for _, it := range BuildNeeds(e.App, st, ps).Items {
		for i := range it.Actions {
			if it.Actions[i].Kind == ActChooseDisable {
				act = &it.Actions[i]
			}
		}
	}
	if act == nil || len(act.Options) != 2 {
		t.Fatalf("action %+v", act)
	}
	if o := act.Options[0]; o.Label != "disable Solo" {
		t.Fatalf("first %+v", o)
	}
	if o := act.Options[1]; o.Label != "disable Lib (2 enabled mods need it: Uses Lib, Addon)" || !strings.Contains(o.Done, "2 mods that need it") {
		t.Fatalf("second %+v", o)
	}
}
