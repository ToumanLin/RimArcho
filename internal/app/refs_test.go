package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A bare packageId is pkg:<packageId> (05 §2); no other reference form has its shape.
func TestBarePackageID(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{
		"khamenman.outfitstandsplus":                true,
		"OskarPotocki.VanillaFactionsExpanded.Core": true,
		"ludeon.rimworld":                           true,
		"brrainz.harmony_steam":                     true,
		"my-mod.v2":                                 true,
		"3dtoy.thing":                               true,
		"a3f9":                                      false, // instance ID prefix
		"#a3f9":                                     false,
		"L#7c2e":                                    false,
		"pkg:tester.moda":                           false, // prefixed: PackageRef handles it
		"ws:2890901044":                             false,
		"def:ThingDef/Gun_Revolver":                 false,
		"ThingDef/Gun_Revolver":                     false,
		"Gun_Revolver":                              false, // a bare defName has no dot
		"1.6":                                       false, // a game version
		"1.6.4633":                                  false,
		"Mods/Foo.Bar":                              false, // a path
		`C:\Mods\Foo.Bar`:                           false,
		"Combat Extended":                           false, // a name
		"combat extended.core":                      false,
		"tester..moda":                              false,
		".moda":                                     false,
		"moda.":                                     false,
		"Alpha":                                     false,
	} {
		if got := app.BarePackageID(s); got != want {
			t.Errorf("BarePackageID(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestResolveBarePackageID(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	want, err := ix.ResolveOne("pkg:tester.moda")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"tester.moda", "Tester.ModA", " TESTER.MODA "} {
		m, err := ix.ResolveOne(ref)
		if err != nil || m.ID != want.ID {
			t.Errorf("ResolveOne(%q) = %v, %v; want %s", ref, m, err, ix.Ref(want.ID))
		}
	}
	if p, ok := app.PackageRef("Tester.ModA"); !ok || p != "tester.moda" {
		t.Errorf("PackageRef(Tester.ModA) = %q, %v", p, ok)
	}

	_, err = ix.Resolve("khamenman.outfitstandsplus")
	var re *app.RefError
	if !errors.As(err, &re) {
		t.Fatalf("no match: %v", err)
	}
	cli := "khamenman.outfitstandsplus matches nothing (no mod has packageId khamenman.outfitstandsplus; search with archo find outfitstandsplus)"
	if re.Error() != cli {
		t.Errorf("CLI text:\n%s\nwant\n%s", re.Error(), cli)
	}
	if mcp := strings.Replace(cli, "archo find outfitstandsplus", "find_mods query=outfitstandsplus", 1); re.MCPError() != mcp {
		t.Errorf("MCP text:\n%s\nwant\n%s", re.MCPError(), mcp)
	}

	// Words that are no reference and no packageId keep the "not a reference" error.
	_, err = ix.Resolve("Combat Extended")
	if err == nil || !strings.Contains(err.Error(), `not a reference`) || !strings.Contains(err.Error(), `archo find "Combat Extended"`) {
		t.Errorf("name: %v", err)
	}
	// A version or a Def is not taken for a packageId.
	for _, ref := range []string{"1.6", "ThingDef/Lib_Gun"} {
		if _, err := ix.Resolve(ref); err == nil || !strings.Contains(err.Error(), "not a reference") {
			t.Errorf("%s: %v", ref, err)
		}
	}
}

// Disabling by a bare packageId removes it like pkg: does.
func TestDisableBarePackageID(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("enable", "Tester.ModA")
	if out := e.MustRun("list", "--active"); !strings.Contains(out, "Mod A") {
		t.Fatalf("enable by bare packageId:\n%s", out)
	}
	e.MustRun("disable", "TESTER.MODA")
	if out := e.MustRun("list", "--active"); strings.Contains(out, "Mod A") {
		t.Fatalf("disable by bare packageId:\n%s", out)
	}
}
