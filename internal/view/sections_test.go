package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §4.2: patches and Harmony are analysed only for the current list (05 §5). A mod outside
// it says so, without a count, instead of "0" over a patch file and a DLL it does have.
func TestSectionsOutsideList(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	patch := `<Patch><Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="OnDef"]</xpath><value><x/></value></Operation></Patch>`
	on := testkit.Sample("tester.on", "On")
	on.Files = map[string]string{"Defs/a.xml": "<Defs><ThingDef><defName>OnDef</defName></ThingDef></Defs>", "Patches/p.xml": patch}
	off := testkit.Sample("tester.off", "Off")
	off.Files = map[string]string{"Defs/b.xml": "<Defs><ThingDef><defName>OffDef</defName></ThingDef></Defs>", "Patches/q.xml": patch}
	e.AddWorkshop("3001", on)
	e.AddWorkshop("3002", off)
	dll, err := os.ReadFile(filepath.Join("..", "..", "testdata", "dotnet", "bin", "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.Workshop, "3002", "Assemblies", "ModA.dll"), string(dll))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.on")
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]view.Section{}
	for _, m := range st.Ix.Mods {
		if m.PackageID == "tester.on" || m.PackageID == "tester.off" {
			got[m.PackageID] = map[string]view.Section{}
			for _, s := range view.Sections(e.App, st.Ix, m) {
				got[m.PackageID][s.Title] = s
			}
		}
	}
	if s := got["tester.on"]["patches"]; s.Count != 1 {
		t.Fatalf("listed mod's patches: %+v", s)
	}
	if s := got["tester.off"]["Defs"]; s.Count != 1 {
		t.Fatalf("unlisted mod's Defs are read too: %+v", s)
	}
	for _, title := range []string{"patches", "Harmony"} {
		s := got["tester.off"][title]
		if s.Count != -1 || !strings.Contains(strings.Join(s.Lines, "\n"), "not analysed (not in the current list)") {
			t.Fatalf("unlisted mod's %s: %+v", title, s)
		}
	}
}
