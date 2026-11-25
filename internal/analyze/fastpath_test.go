package analyze

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

// TestFastPathMatchesEngine: every expression the defName/@Name fast path takes selects the
// same nodes, in the same order, as the XPath engine on the whole document (05 §5.2).
func TestFastPathMatchesEngine(t *testing.T) {
	body := `<ThingDef Name="BaseA" Abstract="True"><statBases><Mass>1</Mass></statBases></ThingDef>
<ThingDef ParentName="BaseA"><defName>A</defName><label>a</label><comps><li Class="C1"/><li Class="C2"/></comps></ThingDef>
<RecipeDef><defName>A</defName><label>recipe a</label></RecipeDef>
<ThingDef><defName>B</defName><label>b</label><comps><li Class="C2"/></comps></ThingDef>
<ThingDef Name="Named"><defName>C</defName><label>c</label></ThingDef>
<ThingDef><defName>D</defName><defName>B</defName><label>two names</label></ThingDef>
<ThingDef><defName>O r</defName><label>or in a name</label></ThingDef>`
	names := xdoc.NewNames()
	doc := xdoc.New(names)
	root := doc.NewElement("Defs")
	doc.AppendChild(doc.Root(), root)
	evs, err := xdoc.Parse([]byte(defs(body)))
	if err != nil {
		t.Fatal(err)
	}
	doc.Build(root, evs, true, false)
	s := newSim(doc, root, nil, "1.6", nil)

	fast := []string{
		`Defs/ThingDef[defName="B"]`,
		`Defs/ThingDef[defName="C" or defName="A"]`,
		`/Defs/ThingDef[defName="B" or defName="D"]/label`,
		`Defs/*[defName="A" or defName='B']/label`,
		`Defs/ThingDef[@Name="Named" or defName="A"]`,
		`Defs/ThingDef[@Name="BaseA" or @Name="Named"]/statBases`,
		`Defs/ThingDef[defName="A" or defName="A" or defName="Missing"]/comps/li[@Class="C2"]`,
		`Defs/ThingDef[ defName = "B"  or  defName = "C" ]`,
		`Defs/ThingDef[defName="O r" or defName="B"]`,
		`Defs/ThingDef[defName="Missing" or defName="Gone"]`,
		`Defs/ThingDef[defName="A" or defName="B"][label="b"]`,
		`*/ThingDef[defName = "B"]`,
		`/*/ThingDef[defName="A" or defName="C"]/label`,
		`*/*[@Name="Named"]`,
		`Defs/ThingDef [defName="B"]/label`,
		"/Defs/ThingDef[\n  defName=\"A\"\n  ]/comps/li[\n    @Class=\"C2\"\n  ]",
		"Defs/ThingDef[defName=\"A\"\n or defName=\"B\"]",
	}
	for _, x := range fast {
		if s.plan(x) == nil {
			t.Errorf("%s: no fast plan", x)
			continue
		}
		for _, one := range []bool{false, true} {
			e, err := s.compile(x)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := doc.Select(e, doc.Root(), one)
			got := s.fastSelect(s.plan(x), one)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s (one=%v): fast %v, engine %v", x, one, got, want)
			}
		}
	}
	slow := []string{
		`Defs/ThingDef[defName="A" and defName="B"]`,
		`Defs/ThingDef[defName="A" or label="b"]`,
		`Defs/ThingDef[(defName="A" or defName="B")]`,
		`Defs/ThingDef[defName="A" or defName="B"]/../RecipeDef`,
		`Defs/ThingDef[defName="A" or defName="B"] | Defs/RecipeDef`,
		`Defs/ThingDef[defName="A" or]`,
		`Defs/ThingDef[defName="A" or defName="B" or]`,
	}
	for _, x := range slow {
		if s.plan(x) != nil {
			t.Errorf("%s: took the fast path", x)
		}
	}
	// `*` is Defs only while Defs is the document's one element
	doc.AppendChild(doc.Root(), doc.NewElement("Other"))
	if p := s.plan(`*/ThingDef[defName="B"]`); s.usable(p) {
		t.Error("*/ used the index with a second top-level element")
	}
	if p := s.plan(`Defs/ThingDef[defName="B"]`); !s.usable(p) {
		t.Error("Defs/ lost the index with a second top-level element")
	}
}
