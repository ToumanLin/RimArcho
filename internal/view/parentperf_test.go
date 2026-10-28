package view_test

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Only synthetic declarations/findings; the timed page build has no file scan.
func BenchmarkParentNeeds(b *testing.B) {
	e := testkit.New(b)
	for i := 0; i < 100; i++ {
		e.AddWorkshop(fmt.Sprint(1000+i), testkit.Mod{PackageID: fmt.Sprintf("test.mod%d", i), Name: fmt.Sprintf("Mod %d", i)})
	}
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		b.Fatal(err)
	}
	ps := &app.ProblemSet{State: st}
	for _, m := range st.Ix.Mods {
		if m.PackageID == "ludeon.rimworld" {
			continue
		}
		for j := 0; j < 5; j++ {
			ps.List = append(ps.List, &app.Problem{Ref: fmt.Sprintf("p:%06x", len(ps.List)), Finding: &finding.Finding{Code: "def.parent-missing", Severity: finding.Error, Subjects: []string{finding.Inst(m.ID)}, Claim: fmt.Sprintf(`PawnKindDef Pawn%d inherits ParentName="Base", which no loaded def has as its Name (after patches)`, j), Evidence: []finding.Evidence{{Source: finding.Declared, Text: "Defs/X.xml:1"}}}})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view.BuildNeeds(e.App, st, ps)
	}
}
