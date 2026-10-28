package mcp_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"reflect"
)

func TestParentProblemRefParity(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.child", Name: "Child", Files: map[string]string{"Defs/Child.xml": `<Defs><HediffDef ParentName="Base"><defName>Illness</defName></HediffDef></Defs>`}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.provider", Name: "Provider", Files: map[string]string{"Defs/Base.xml": `<Defs><HediffDef Name="Base" Abstract="True" /></Defs>`}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.child")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	for _, p := range ps.List {
		if p.Code == "def.parent-missing" {
			ref = p.Ref
			break
		}
	}
	out, isErr := call(t, connect(t, e), "list_problems", map[string]any{"ref": ref})
	if isErr || out != e.MustRun("problems", "--ref", ref) {
		t.Fatalf("parity error=%v:\n%s", isErr, out)
	}
	if !strings.Contains(out, "fix: archo enable") || strings.Contains(out, "ParentName=") {
		t.Fatal(out)
	}
}

func TestProblemRefKeepsExpansionAndGameVersion(t *testing.T) {
	got := mcp.ProblemsArgv(mcp.ProblemsIn{Ref: "p:123456", All: true, GameVersion: "1.6"})
	want := []string{"problems", "--ref", "p:123456", "--all", "--game-version", "1.6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv %v", got)
	}
}
