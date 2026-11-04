package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestDependenciesMergeByCanonicalTarget(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.a", Name: "A"})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.b", Name: "B"})
	e.AddWorkshop("1003", testkit.Mod{PackageID: "test.framework", Name: "Framework"})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.a", "test.b")
	e.AddEnv(false)
	e.MustRun("scan")
	st, _ := e.App.Loadout()
	a, _ := st.Ix.ResolveOne("pkg:test.a")
	b, _ := st.Ix.ResolveOne("pkg:test.b")
	f, _ := st.Ix.ResolveOne("pkg:test.framework")
	for _, tc := range []struct {
		name    string
		targets []string
		code    string
		want    string
		kind    string
	}{
		{"missing", []string{"pkg:Test.Missing", "pkg:test.missing_steam"}, "dep.missing", "test.missing", ActSubscribe},
		{"installed", []string{finding.Inst(f.ID), "pkg:TEST.FRAMEWORK_steam"}, "dep.inactive", "test.framework", ActEnable},
		{"folder alias", []string{"pkg:1003", finding.Inst(f.ID)}, "dep.inactive", "test.framework", ActEnable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := &app.ProblemSet{State: st}
			for i, id := range []string{a.ID, b.ID} {
				ps.List = append(ps.List, &app.Problem{Finding: &finding.Finding{Code: tc.code, Severity: finding.Error, Key: tc.want, Subjects: []string{finding.Inst(id), tc.targets[i]}, Claim: "requires Framework (pkg:" + tc.want + ")", Suggestions: []string{"subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=1234"}}})
			}
			n := BuildNeeds(e.App, st, ps)
			var deps []Need
			for _, it := range n.Items {
				if it.Target == tc.want {
					deps = append(deps, it)
				}
			}
			if len(deps) != 1 {
				t.Fatalf("got %d merged dependencies: %+v", len(deps), n.Items)
			}
			it := deps[0]
			if len(it.RequiredBy) != 2 || len(it.Mods) != 2 || it.Actions[0].Kind != tc.kind || it.ID == "" || it.Actions[0].ID == "" {
				t.Fatalf("merged item: %+v", it)
			}
			p := ProblemsOf(n)
			for _, id := range []string{a.ID, b.ID} {
				if p.Mods[id] == nil {
					t.Fatal("consumer missing badge")
				}
			}
			if p.Mods[f.ID] != nil {
				t.Fatal("inactive dependency blamed as consumer")
			}
		})
	}
}
