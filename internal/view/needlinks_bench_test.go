package view

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Compare the same synthetic snapshot with the pre-link BuildNeeds pipeline as a
// control. No log runs here: the only extra work is the structured name projection.
func BenchmarkNeeds1354(b *testing.B) {
	SetLang("zh")
	b.Cleanup(func() { SetLang("en") })
	e := testkit.New(b)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	var packages []string
	packages = append(packages, "ludeon.rimworld", "brrainz.harmony")
	for i := 0; i < 1352; i++ {
		pkg := fmt.Sprintf("tester.bench%d", i)
		e.AddWorkshop(fmt.Sprint(3000000000+i), testkit.Sample(pkg, fmt.Sprintf("测试 Mod %04d Framework", i)))
		packages = append(packages, pkg)
	}
	e.WriteModsConfig("1.6", packages...)
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		b.Fatal(err)
	}
	ps := &app.ProblemSet{}
	for i := 0; i < 100; i++ {
		x, _ := st.Ix.ResolveOne("pkg:" + packages[2+i])
		y, _ := st.Ix.ResolveOne("pkg:" + packages[3+i])
		ps.List = append(ps.List, &app.Problem{Finding: &finding.Finding{Code: "incompat.active", Severity: finding.Error,
			Subjects: []string{finding.Inst(x.ID), finding.Inst(y.ID)}}})
	}
	for _, linked := range []bool{false, true} {
		label := "plain"
		if linked {
			label = "linked"
		}
		b.Run(label, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if linked {
					BuildNeeds(e.App, st, ps)
					continue
				}
				n := &Needs{}
				n.Items = append(mergeDependencies(st, buildItems(e.App, st, ps, e.App.MissingMods(st))), directoryNeeds(e.App)...)
				n.Items = append(n.Items, linkNeeds(e.App, ps.GameLinks)...)
				for i := range n.Items {
					n.Items[i].Section = SectionOf(n.Items[i].Group)
					identifyNeed(&n.Items[i])
				}
				indexNeeds(n)
			}
		})
	}
}
