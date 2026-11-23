package analyze

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

// dumpNode renders the subtree at n as compact XML (attributes in document order).
func dumpNode(b *strings.Builder, d *xdoc.Doc, n int32) {
	switch d.Kind(n) {
	case xdoc.KindText:
		b.WriteString(d.Text(n))
		return
	case xdoc.KindElement:
	default:
		for c := d.FirstChild(n); c != xdoc.None; c = d.Next(c) {
			dumpNode(b, d, c)
		}
		return
	}
	b.WriteString("<" + d.Name(n))
	for _, a := range d.Attrs(n) {
		fmt.Fprintf(b, " %s=%q", d.Names().Str(a.Name), a.Value)
	}
	b.WriteString(">")
	for c := d.FirstChild(n); c != xdoc.None; c = d.Next(c) {
		dumpNode(b, d, c)
	}
	b.WriteString("</" + d.Name(n) + ">")
}

// dumpOp renders the simulation state of o and its nested operations.
func dumpOp(b *strings.Builder, s *sim, o *Op, depth int) {
	g := s.guards[o]
	fmt.Fprintf(b, "%s%s ran=%t result=%t err=%t hits=%d guard=%v/%v/%t/%v", strings.Repeat("  ", depth), o.Class, o.ran, o.result, o.err, o.hits,
		g.findMods, g.mayReq, g.cond, g.folder)
	if o.lastFailed != nil {
		b.WriteString(" lastFailed=" + o.lastFailed.String())
	}
	b.WriteByte('\n')
	for _, c := range []*Op{o.match, o.nomatch} {
		if c != nil {
			dumpOp(b, s, c, depth+1)
		}
	}
	for _, c := range o.ops {
		dumpOp(b, s, c, depth+1)
	}
}

// simulate runs each operation against a fresh copy of the Defs body and reports, per
// operation, its result, the simulation state of its tree and the document afterwards.
func simulate(t *testing.T, body string, ops ...string) string {
	t.Helper()
	var b strings.Builder
	active := []*Mod{{Instance: "a", Pkg: "test.a", Name: "Mod A", Pos: 1}, {Instance: "b", Pkg: "test.b", Name: "Mod B", Pos: 2}}
	for i, op := range ops {
		names := xdoc.NewNames()
		doc := xdoc.New(names)
		root := doc.NewElement("Defs")
		doc.AppendChild(doc.Root(), root)
		evs, err := xdoc.Parse([]byte(defs(body)))
		if err != nil {
			t.Fatal(err)
		}
		doc.Build(root, evs, true, false)
		pevs, err := xdoc.Parse([]byte(patch(op)))
		if err != nil {
			t.Fatalf("op %d: %v", i, err)
		}
		pd := xdoc.New(names)
		pd.Build(pd.Root(), pevs, false, true)
		s := newSim(doc, root, active, "1.6", nil)
		for _, c := range pd.Children(pd.DocElement()) {
			top := &topOp{mod: active[0], op: parseOp(pd, c), folderReq: []string{"test.f"}}
			s.run(top)
			fmt.Fprintf(&b, "#%d ok=%t err=%t\n", i, top.ok, top.err)
			dumpOp(&b, s, top.op, 1)
		}
		dumpNode(&b, doc, root)
		b.WriteString("\n")
		fmt.Fprintf(&b, "contests=%d\n", len(s.contests))
	}
	return b.String()
}

const simDefs = `<ThingDef Name="Base"><defName>A</defName><label>a</label><comps><li>x</li></comps></ThingDef>
<ThingDef><defName>B</defName><label>b</label><modExtensions><li>m</li></modExtensions></ThingDef>`

// Every operation class and branch of the simulation, pinned to its exact effect.
func TestWorkerOperations(t *testing.T) {
	op := func(class, inner string) string {
		if class == "" {
			return "<Operation>" + inner + "</Operation>"
		}
		return `<Operation Class="` + class + `">` + inner + "</Operation>"
	}
	xp := func(x string) string { return "<xpath>" + x + "</xpath>" }
	val := "<value><n1/><n2/></value>"
	got := simulate(t, simDefs,
		// Add
		op("PatchOperationAdd", xp(`Defs/ThingDef[defName="A"]`)),
		op("PatchOperationAdd", xp(`Defs/ThingDef[`)+val),
		op("PatchOperationAdd", xp(`Defs/ThingDef/@Name`)+val),
		op("PatchOperationAdd", xp(`Defs/ThingDef[defName="A"]/comps`)+val+"<order>Prepend</order>"),
		op("PatchOperationAdd", xp(`Defs/ThingDef/comps`)+val),
		op("PatchOperationAdd", xp(`Defs/ThingDef[defName="None"]`)+val),
		// Insert
		op("PatchOperationInsert", xp(`Defs/ThingDef/label`)),
		op("PatchOperationInsert", xp(`Defs/ThingDef[`)+val),
		op("PatchOperationInsert", xp(`Defs/ThingDef/@Name`)+val),
		op("PatchOperationInsert", xp(`/`)+val),
		op("PatchOperationInsert", xp(`Defs/ThingDef[defName="A"]/label`)+val+"<order>Append</order>"),
		op("PatchOperationInsert", xp(`Defs/ThingDef[defName="A"]/label`)+val),
		op("PatchOperationInsert", xp(`Defs/ThingDef[defName="A"]/label/text()`)+"<value>t</value>"),
		op("PatchOperationInsert", xp(`Defs/Nothing`)+val),
		// Replace
		op("PatchOperationReplace", xp(`Defs/ThingDef/label`)),
		op("PatchOperationReplace", xp(`Defs/ThingDef[`)+val),
		op("PatchOperationReplace", xp(`Defs/ThingDef/@Name`)+val),
		op("PatchOperationReplace", xp(`/`)+val),
		op("PatchOperationReplace", xp(`Defs/ThingDef/label`)+val),
		op("PatchOperationReplace", xp(`Defs/Nothing`)+val),
		// Remove
		op("PatchOperationRemove", xp(`Defs/ThingDef[`)),
		op("PatchOperationRemove", xp(`Defs/ThingDef/@Name`)),
		op("PatchOperationRemove", xp(`/`)),
		op("PatchOperationRemove", xp(`Defs/ThingDef[defName="B"]/label`)),
		op("PatchOperationRemove", xp(`Defs/Nothing`)),
		// Attributes
		op("PatchOperationAttributeAdd", xp(`Defs/ThingDef[`)+"<attribute>K</attribute><value>v</value>"),
		op("PatchOperationAttributeAdd", xp(`Defs/ThingDef`)+"<attribute>Name</attribute><value>v</value>"),
		op("PatchOperationAttributeAdd", xp(`Defs/ThingDef/label/text()|Defs/ThingDef/@Name`)+"<attribute>K</attribute><value>v</value>"),
		op("PatchOperationAttributeSet", xp(`Defs/ThingDef`)+"<attribute>Name</attribute><value>v</value>"),
		op("PatchOperationAttributeSet", xp(`Defs/ThingDef/label/text()`)+"<attribute>K</attribute><value>v</value>"),
		op("PatchOperationAttributeSet", xp(`Defs/ThingDef/@Name`)+"<attribute>K</attribute><value>v</value>"),
		op("PatchOperationAttributeSet", xp(`Defs/Nothing`)+"<attribute>K</attribute><value>v</value>"),
		op("PatchOperationAttributeRemove", xp(`Defs/ThingDef`)+"<attribute>Name</attribute>"),
		op("PatchOperationAttributeRemove", xp(`Defs/ThingDef[defName="B"]`)+"<attribute>Name</attribute>"),
		op("PatchOperationAttributeRemove", xp(`Defs/ThingDef/label/text()`)+"<attribute>Name</attribute>"),
		// SetName
		op("PatchOperationSetName", xp(`Defs/ThingDef[`)+"<name>X</name>"),
		op("PatchOperationSetName", xp(`Defs/ThingDef/@Name`)+"<name>X</name>"),
		op("PatchOperationSetName", xp(`/`)+"<name>X</name>"),
		op("PatchOperationSetName", xp(`Defs/ThingDef/label`)+"<name>title</name>"),
		op("PatchOperationSetName", xp(`Defs/Nothing`)+"<name>X</name>"),
		// AddModExtension
		op("PatchOperationAddModExtension", xp(`Defs/ThingDef`)),
		op("PatchOperationAddModExtension", xp(`Defs/ThingDef[`)+val),
		op("PatchOperationAddModExtension", xp(`Defs/ThingDef/@Name`)+val),
		op("PatchOperationAddModExtension", xp(`Defs/ThingDef`)+val),
		op("PatchOperationAddModExtension", xp(`Defs/Nothing`)+val),
		// Test
		op("PatchOperationTest", xp(`Defs/ThingDef`)),
		op("PatchOperationTest", xp(`Defs/Nothing`)),
		op("PatchOperationTest", xp(`Defs/ThingDef[`)),
		// Conditional
		op("PatchOperationConditional", xp(`Defs/ThingDef[`)),
		op("PatchOperationConditional", xp(`Defs/ThingDef`)+`<match Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</match>`),
		op("PatchOperationConditional", xp(`Defs/ThingDef`)+`<nomatch Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</nomatch>`),
		op("PatchOperationConditional", xp(`Defs/ThingDef`)),
		op("PatchOperationConditional", xp(`Defs/Nothing`)+`<nomatch Class="PatchOperationRemove">`+xp(`Defs/Nothing`)+`</nomatch>`),
		op("PatchOperationConditional", xp(`Defs/Nothing`)+`<match Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</match>`),
		op("PatchOperationConditional", xp(`Defs/Nothing`)),
		// FindMod
		op("PatchOperationFindMod", `<mods><li>Nope</li><li>Mod B</li></mods><match Class="PatchOperationFindMod"><mods><li>Mod A</li></mods><match Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</match></match>`),
		op("PatchOperationFindMod", `<mods><li>Mod A</li></mods><nomatch Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</nomatch>`),
		op("PatchOperationFindMod", `<mods><li>Nope</li></mods><nomatch Class="PatchOperationRemove">`+xp(`Defs/Nothing`)+`</nomatch>`),
		op("PatchOperationFindMod", `<mods><li>Nope</li></mods><match Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</match>`),
		// Sequence
		op("PatchOperationSequence", `<operations>`+
			`<li Class="PatchOperationRemove" MayRequire="test.x">`+xp(`Defs/ThingDef/label`)+`</li>`+
			`<li Class="PatchOperationRemove" MayRequireAnyOf="test.x,test.b_steam">`+xp(`Defs/ThingDef[defName="A"]/label`)+`</li>`+
			`<li Class="PatchOperationSequence" MayRequire="Test.B"><operations><li Class="PatchOperationRemove">`+xp(`Defs/ThingDef[defName="B"]/label`)+`</li></operations></li>`+
			`<li Class="PatchOperationTest">`+xp(`Defs/ThingDef`)+`</li></operations>`),
		op("PatchOperationSequence", `<operations><li Class="PatchOperationTest">`+xp(`Defs/ThingDef`)+`</li><li Class="PatchOperationTest">`+xp(`Defs/Nothing`)+`</li><li Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</li></operations>`),
		op("PatchOperationSequence", `<operations><li Class="PatchOperationTest">`+xp(`Defs/ThingDef[`)+`</li><li Class="PatchOperationRemove">`+xp(`Defs/ThingDef/label`)+`</li></operations>`),
		op("PatchOperationSequence", ""),
		// Base class, custom class, success overrides
		op("", xp(`Defs`)),
		op("PatchOperation", xp(`Defs`)),
		op("SomeMod.PatchOperationMagic", xp(`Defs/Nothing`)),
		op("PatchOperationTest", xp(`Defs/Nothing`)+"<success>Always</success>"),
		op("PatchOperationTest", xp(`Defs/ThingDef`)+"<success>Never</success>"),
		op("PatchOperationTest", xp(`Defs/Nothing`)+"<success>Invert</success>"),
		op("PatchOperationTest", xp(`Defs/ThingDef[`)+"<success>Always</success>"),
	)
	b, err := os.ReadFile("testdata/sim_worker.golden")
	if err != nil {
		t.Fatal(err)
	}
	if workerWant := strings.ReplaceAll(string(b), "\r\n", "\n"); got != workerWant {
		t.Errorf("simulation changed:\n%s", got)
		gl, wl := strings.Split(got, "\n"), strings.Split(workerWant, "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("first difference at line %d:\n got %s\nwant %s", i+1, gl[i], wl[i])
			}
		}
	}
}
