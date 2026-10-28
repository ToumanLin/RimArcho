package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPatchInspectionFocusesAffectedChanges(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.child", Name: "Child", Files: map[string]string{
		"Patches/A.xml": `<Patch><Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Target"]</xpath><value><label>changed</label></value></Operation></Patch>`,
		"Patches/B.xml": `<Patch><Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Target"]</xpath><value><description>changed too</description></value></Operation></Patch>`,
	}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.target", Name: "Target Mod", Files: map[string]string{"Defs/Target.xml": `<Defs><ThingDef><defName>Target</defName></ThingDef></Defs>`}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.child")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var p *app.Problem
	for _, q := range ps.List {
		if q.Code == "patch.no-match" && len(q.Subjects) > 1 {
			p = q
			break
		}
	}
	if p == nil {
		t.Fatal("no inactive target finding")
	}
	panel, err := view.BuildProblemPanel(e.App, ps, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(panel.Affected) != 2 || len(panel.Actions) != 2 || panel.Actions[0].Kind != view.ActEnable || panel.Actions[1].Kind != view.ActOpenURL {
		t.Fatalf("panel %+v", panel)
	}
	if strings.Contains(panel.Title, "PatchOperation") || !strings.Contains(panel.Meaning, "Target Mod") {
		t.Fatalf("panel %+v", panel)
	}
	out := e.MustRun("problems", "--ref", p.Ref)
	if !strings.Contains(out, "cannot take effect") || !strings.Contains(out, "fix: archo enable") || strings.Contains(out, "archo patches --mod") {
		t.Fatal(out)
	}
	// The known-error inspection reuses the item's existing copy and corpus chip.
	mod := ps.State.Ix.ByID[p.SubjectInstance()]
	known := &app.Problem{Ref: "p:abcdef", Finding: &finding.Finding{Code: "mod.known-error", Severity: finding.Info, Subjects: []string{finding.Inst(mod.ID)}, Claim: "known error", Evidence: []finding.Evidence{{Source: finding.Community, Text: "log corpus: test.child in 9 of 10 shared logs"}}}}
	panel, err = view.BuildProblemPanel(e.App, &app.ProblemSet{State: ps.State, List: []*app.Problem{known}}, known.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(panel.Actions) != 1 || panel.Actions[0].Kind != view.ActOpenURL || !strings.Contains(panel.Title, "known error") {
		t.Fatalf("known error %+v", panel)
	}
}
