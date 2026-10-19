package analyze

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/scan"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func defs(body string) string { return "<?xml version=\"1.0\"?>\n<Defs>\n" + body + "\n</Defs>" }

func patch(ops ...string) string {
	return "<?xml version=\"1.0\"?>\n<Patch>\n" + strings.Join(ops, "\n") + "\n</Patch>"
}

type fixture struct {
	mods map[string]*Mod
	in   Input
}

func newFixture(t *testing.T) *fixture {
	root := t.TempDir()
	f := &fixture{mods: map[string]*Mod{}}
	mk := func(id, pkg, name string, pos int, decl scan.Decl) *Mod {
		m := &Mod{Instance: id, Lineage: "L" + id, Dir: filepath.Join(root, id), Pkg: pkg, Name: name, Pos: pos, SnapHash: []byte(id), Decl: decl}
		f.mods[id] = m
		f.in.Mods = append(f.in.Mods, m)
		return m
	}
	core := mk("core", "ludeon.rimworld", "Core", 1, scan.Decl{})
	write(t, filepath.Join(core.Dir, "Defs", "Things.xml"), defs(`
  <ThingDef Name="BaseGun" Abstract="True"><label>base</label></ThingDef>
  <ThingDef ParentName="BaseGun"><defName>Gun_A</defName><label>a</label></ThingDef>`))
	b := mk("modb", "test.b", "Mod B", 2, scan.Decl{})
	write(t, filepath.Join(b.Dir, "Defs", "Guns.xml"), defs(`
  <ThingDef ParentName="BaseGun"><defName>Gun_A</defName><label>b a</label></ThingDef>`))
	write(t, filepath.Join(b.Dir, "Patches", "P.xml"), patch(
		// 1 ok, contested with modc
		`<Operation Class="PatchOperationReplace"><xpath>Defs/ThingDef[defName="Gun_A"]/label</xpath><value><label>x</label></value></Operation>`,
		// 2 C4: target in an inactive mod, unguarded
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Other_X"]</xpath><value><comps/></value></Operation>`,
		// 3 guarded by FindMod: skipped, no failure
		`<Operation Class="PatchOperationFindMod"><mods><li>X Mod</li></mods><match Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Other_X"]</xpath><value><a/></value></match></Operation>`,
		// 4 MayRequire inside a Sequence: skipped
		`<Operation Class="PatchOperationSequence"><operations><li Class="PatchOperationAdd" MayRequire="test.x"><xpath>Defs/ThingDef[defName="Other_X"]</xpath><value><a/></value></li></operations></Operation>`,
		// 5 MayRequire on a top-level Operation: ignored by the game, so it fails
		`<Operation Class="PatchOperationAdd" MayRequire="test.x"><xpath>Defs/ThingDef[defName="Other_X"]/x</xpath><value><a/></value></Operation>`,
		// 6 target-later: modc adds <comps> afterwards
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Gun_A"]/comps</xpath><value><li>c</li></value></Operation>`,
		// 7 stale-name under FindMod(Mod C)
		`<Operation Class="PatchOperationFindMod"><mods><li>Mod C</li></mods><match Class="PatchOperationRemove"><xpath>Defs/ThingDef[defName="Gun_Cx"]/label</xpath></match></Operation>`,
		// 8 tree op on an attribute node throws
		`<Operation Class="PatchOperationRemove"><xpath>Defs/ThingDef[@Name="BaseGun"]/@Abstract</xpath></Operation>`,
		// 9 custom class
		`<Operation Class="SomeMod.PatchOperationMagic"><xpath>Defs</xpath></Operation>`,
		// 10 target-missing
		`<Operation Class="PatchOperationRemove"><xpath>Defs/ThingDef[defName="Nowhere"]/label</xpath></Operation>`,
	))
	c := mk("modc", "test.c", "Mod C", 3, scan.Decl{})
	write(t, filepath.Join(c.Dir, "Defs", "C.xml"), defs(`
  <ThingDef ParentName="NoSuchBase"><defName>Gun_C</defName><label>c</label></ThingDef>`))
	write(t, filepath.Join(c.Dir, "Patches", "C.xml"), patch(
		`<Operation Class="PatchOperationReplace"><xpath>Defs/ThingDef[defName="Gun_A"]/label</xpath><value><label>y</label></value></Operation>`,
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Gun_A"]</xpath><value><comps/></value></Operation>`,
	))
	d := mk("modd", "test.d", "Mod D", 4, scan.Decl{})
	write(t, filepath.Join(d.Dir, "1.5", "Defs", "Old.xml"), defs(`<ThingDef><defName>Old_Def</defName></ThingDef>`))
	write(t, filepath.Join(d.Dir, "1.6", "Patches", "D.xml"), patch(
		`<Operation Class="PatchOperationRemove"><xpath>Defs/ThingDef[defName="Old_Def"]/label</xpath></Operation>`))
	x := mk("modx", "test.x", "X Mod", 0, scan.Decl{})
	write(t, filepath.Join(x.Dir, "Defs", "X.xml"), defs(`<ThingDef><defName>Other_X</defName></ThingDef>`))
	f.in.MM = "1.6"
	return f
}

func patchBySeq(out *Output, inst string, seq int) *PatchFact {
	for _, p := range out.Patches {
		if p.Instance == inst && p.Seq == seq {
			return p
		}
	}
	return nil
}

func findingsOf(out *Output, code string) []*Finding {
	var r []*Finding
	for _, f := range out.Findings {
		if f.Code == code {
			r = append(r, f)
		}
	}
	return r
}

func TestSimulationAndReasons(t *testing.T) {
	f := newFixture(t)
	out, err := Run(f.in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]struct {
		result  string
		reasons []string
	}{
		1:  {ResultOK, nil},
		2:  {ResultFailed, []string{ReasonTargetInactive, ReasonUnguarded}},
		3:  {ResultOK, nil},
		4:  {ResultOK, nil},
		5:  {ResultFailed, []string{ReasonTargetInactive, ReasonUnguarded}},
		6:  {ResultFailed, []string{ReasonTargetLater}},
		7:  {ResultFailed, []string{ReasonStaleName}},
		8:  {ResultFailed, []string{ReasonRuntimeException}},
		9:  {ResultUnknown, nil},
		10: {ResultFailed, []string{ReasonTargetMissing}},
	}
	for seq, w := range want {
		p := patchBySeq(out, "modb", seq)
		if p == nil {
			t.Fatalf("op %d missing", seq)
		}
		if p.Result != w.result || !slices.Equal(p.Reasons, w.reasons) {
			t.Errorf("op %d (%s): result %s reasons %v detail %q; want %s %v", seq, p.Desc, p.Result, p.Reasons, p.Detail, w.result, w.reasons)
		}
	}
	if p := patchBySeq(out, "modb", 7); !strings.Contains(p.Detail, "similar: Gun_C") {
		t.Errorf("stale-name candidates: %q", p.Detail)
	}
	if p := patchBySeq(out, "modb", 5); !strings.Contains(p.Detail, "top-level Operation is ignored") {
		t.Errorf("top-level MayRequire note: %q", p.Detail)
	}
	if p := patchBySeq(out, "modd", 1); p.Result != ResultFailed || !slices.Equal(p.Reasons, []string{ReasonWrongVersion}) {
		t.Errorf("wrong-version-route: %+v", p)
	}
	if d := patchBySeq(out, "modb", 2).Desc; d != `Verse.PatchOperationAdd(Defs/ThingDef[defName="Other_X"])` {
		t.Errorf("desc = %q", d)
	}
	// C3: def.override names the effective definer.
	ov := findingsOf(out, "def.override")
	if len(ov) != 1 || ov[0].Key != "ThingDef/Gun_A" || finding.InstID(ov[0].Subjects[0]) != "modb" {
		t.Fatalf("def.override = %+v", ov)
	}
	// C4 as a finding: the inactive definer is a subject and enabling it is suggested.
	var c4 *Finding
	for _, x := range findingsOf(out, "patch.no-match") {
		if strings.Contains(x.Key, `defName="Other_X"])`) {
			c4 = x
		}
	}
	if c4 == nil || !slices.Contains(c4.Subjects, finding.Inst("modx")) || !slices.Contains(c4.Suggestions, "archo enable "+finding.Inst("modx")) {
		t.Fatalf("C4 finding = %+v", c4)
	}
	if pm := findingsOf(out, "def.parent-missing"); len(pm) != 1 || !strings.Contains(pm[0].Claim, "NoSuchBase") {
		t.Fatalf("parent-missing = %+v", pm)
	}
	if ct := findingsOf(out, "patch.contested"); len(ct) != 1 || ct[0].Key != "ThingDef/Gun_A/label" {
		t.Fatalf("contested = %+v", ct)
	}
	if u := findingsOf(out, "patch.unknown-op"); len(u) != 1 {
		t.Fatalf("unknown-op = %+v", u)
	}
	if p := patchBySeq(out, "modb", 1); !slices.Equal(p.Targets, []string{"ThingDef/Gun_A"}) {
		t.Fatalf("targets = %v", p.Targets)
	}
	if len(out.NewDefs["modx"]) != 1 {
		t.Fatalf("inactive defs = %v", out.NewDefs["modx"])
	}
}

func TestLoadFoldersConditions(t *testing.T) {
	f := newFixture(t)
	m := f.mods["modc"]
	m.Decl.LoadFolders = `<loadFolders><v1.6><li>/</li><li IfModActive="test.x">Compat/X</li><li IfModNotActive="test.x">Compat/NoX</li></v1.6></loadFolders>`
	write(t, filepath.Join(m.Dir, "Compat", "X", "Defs", "Y.xml"), defs(`<ThingDef><defName>WithX</defName></ThingDef>`))
	write(t, filepath.Join(m.Dir, "Compat", "NoX", "Defs", "N.xml"), defs(`<ThingDef><defName>WithoutX</defName></ThingDef>`))
	out, err := Run(f.in)
	if err != nil {
		t.Fatal(err)
	}
	names := func(o *Output) []string {
		var r []string
		for _, d := range o.NewDefs["modc"] {
			r = append(r, d.DefName)
		}
		slices.Sort(r)
		return r
	}
	if got := names(out); !slices.Equal(got, []string{"Gun_C", "WithoutX"}) {
		t.Fatalf("without X: %v", got)
	}
	key1 := out.Content["modc"].Key
	f.mods["modx"].Pos = 5
	out2, _ := Run(f.in)
	if got := names(out2); !slices.Equal(got, []string{"Gun_C", "WithX"}) {
		t.Fatalf("with X: %v", got)
	}
	if out2.Content["modc"].Key == key1 {
		t.Fatal("content key ignores the condition outcome")
	}
	if out2.Content["modb"].Key != out.Content["modb"].Key {
		t.Fatal("content key of an unconditional mod changed with the Loadout")
	}
}

func TestParentAfterPatchesMayRequireAndFolderGuard(t *testing.T) {
	f := newFixture(t)
	c := f.mods["modc"]
	// Parent "Gun_A" is a defName; a patch gives it a Name before inheritance runs.
	write(t, filepath.Join(c.Dir, "Defs", "Kids.xml"), defs(`
  <ThingDef ParentName="Gun_A_Base"><defName>Kid_1</defName></ThingDef>
  <ThingDef ParentName="Missing_Base" MayRequire="test.x"><defName>Kid_2</defName></ThingDef>`))
	write(t, filepath.Join(c.Dir, "Patches", "Name.xml"), patch(
		`<Operation Class="PatchOperationAttributeAdd"><xpath>Defs/ThingDef[defName="Gun_A"]</xpath><attribute>Name</attribute><value>Gun_A_Base</value></Operation>`))
	// A patch in a LoadFolders folder conditional on test.x: guarded, and aimed at it.
	c.Decl.LoadFolders = `<loadFolders><v1.6><li>/</li><li IfModActive="test.x">Compat/X</li></v1.6></loadFolders>`
	f.mods["modx"].Pos = 9
	write(t, filepath.Join(c.Dir, "Compat", "X", "Patches", "X.xml"), patch(
		`<Operation Class="PatchOperationRemove"><xpath>Defs/ThingDef[defName="Other_Xy"]/label</xpath></Operation>`))
	out, err := Run(f.in)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range findingsOf(out, "def.parent-missing") {
		if strings.Contains(x.Claim, "Kid_1") {
			t.Fatalf("parent added by a patch not seen: %s", x.Claim)
		}
	}
	// test.x is active now, so Kid_2 loads and its parent is missing.
	if pm := findingsOf(out, "def.parent-missing"); len(pm) != 2 {
		t.Fatalf("parent-missing = %v", pm)
	}
	var p *PatchFact
	for _, x := range out.Patches {
		if x.Instance == "modc" && strings.Contains(x.Desc, "Other_Xy") {
			p = x
		}
	}
	if p == nil || !slices.Equal(p.Reasons, []string{ReasonStaleName}) || !strings.Contains(p.Detail, "similar: Other_X") {
		t.Fatalf("folder-guarded patch: %+v", p)
	}
	// With test.x inactive, Kid_2 is skipped (MayRequire) and the folder is not loaded.
	f.mods["modx"].Pos = 0
	out, _ = Run(f.in)
	for _, x := range findingsOf(out, "def.parent-missing") {
		if strings.Contains(x.Claim, "Kid_2") {
			t.Fatalf("MayRequire def reported: %s", x.Claim)
		}
	}
}
