package analyze

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/scan"
)

// A patch whose target is only in a conditional load folder that does not load (the target
// Mod's IfModActive folder) is folder-inactive, naming the Mod that turns the folder on, not
// wrong-version-route because an older version folder also has it (VM 2026-11-23:
// Progression: Storage's Reel Storage patches read "only in another game version's files";
// Reel loads those defs only with VE Props).
func TestPatchTargetInInactiveConditionalFolder(t *testing.T) {
	for _, installed := range []bool{true, false} {
		root := t.TempDir()
		var in Input
		mk := func(id, pkg, name string, pos int, lf string) *Mod {
			m := &Mod{Instance: id, Lineage: "L" + id, Dir: filepath.Join(root, id), Pkg: pkg, Name: name, Pos: pos, SnapHash: []byte(id), Decl: scan.Decl{LoadFolders: lf}}
			in.Mods = append(in.Mods, m)
			return m
		}
		core := mk("core", "ludeon.rimworld", "Core", 1, "")
		write(t, filepath.Join(core.Dir, "Defs", "Things.xml"), defs(`<ThingDef><defName>Shelf</defName></ThingDef>`))
		reel := mk("reel", "test.reel", "Reel", 2, `<loadFolders><v1.6><li>/</li><li>1.6</li><li IfModActive="test.props">1.6/Props</li></v1.6></loadFolders>`)
		write(t, filepath.Join(reel.Dir, "1.6", "Defs", "R.xml"), defs(`<ThingDef><defName>Barrel</defName></ThingDef>`))
		write(t, filepath.Join(reel.Dir, "1.6", "Props", "Defs", "P.xml"), defs(`<ThingDef><defName>WineRack</defName></ThingDef>`))
		write(t, filepath.Join(reel.Dir, "1.5", "Defs", "Old.xml"), defs(`<ThingDef><defName>WineRack</defName></ThingDef>`))
		prog := mk("prog", "test.prog", "Progression", 3, `<loadFolders><v1.6><li>/</li><li IfModActive="test.reel">Compat/Reel</li></v1.6></loadFolders>`)
		write(t, filepath.Join(prog.Dir, "Compat", "Reel", "Patches", "P.xml"), patch(
			`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Barrel"]</xpath><value><x/></value></Operation>`,
			`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="WineRack"]</xpath><value><x/></value></Operation>`))
		if installed {
			mk("props", "test.props", "Props", 0, "")
		}
		in.MM = "1.6"
		out, err := Run(in)
		if err != nil {
			t.Fatal(err)
		}
		if p := patchBySeq(out, "prog", 1); p == nil || p.Result != ResultOK {
			t.Fatalf("installed %v: Barrel patch %+v", installed, p)
		}
		p := patchBySeq(out, "prog", 2)
		if p == nil || p.Result != ResultFailed || !slices.Equal(p.Reasons, []string{ReasonFolderInactive}) {
			t.Fatalf("installed %v: WineRack patch %+v", installed, p)
		}
		want := "pkg:test.props is active"
		if installed {
			want = finding.Inst("props") + " is active"
		}
		if !strings.Contains(p.Detail, "1.6/Props/Defs/P.xml") || !strings.HasSuffix(p.Detail, "which loads only when "+want) {
			t.Fatalf("installed %v: detail %q", installed, p.Detail)
		}
		var f *Finding
		for _, x := range findingsOf(out, "patch.no-match") {
			if strings.Contains(x.Key, "WineRack") {
				f = x
			}
		}
		if f == nil {
			t.Fatal("no finding")
		}
		// the installed Mod that turns the folder on is a subject to enable; Reel, where the
		// def is, is not
		subj := []string{finding.Inst("prog")}
		if installed {
			subj = append(subj, finding.Inst("props"))
		}
		if !slices.Equal(f.Subjects, subj) {
			t.Fatalf("installed %v: subjects %v", installed, f.Subjects)
		}
	}
}
