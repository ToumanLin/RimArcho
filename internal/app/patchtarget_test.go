package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// patches --target with a bare def name finds an operation nested in FindMod, as the
// Type/name form does (MCP review round 2 P26: get_patches target=Beauty missed VFE's patch).
func TestPatchesBareNameFindsNested(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	lib.Files["Defs/Lib.xml"] = "<Defs><ThingDef><defName>Lib_Gun</defName></ThingDef><ThingDef><defName>LibXGun</defName></ThingDef></Defs>\n"
	delete(lib.Files, "Patches/lib.xml")
	compat := testkit.Sample("tester.compat", "Compat")
	compat.Files["Patches/Compat.xml"] = `<Patch>
  <Operation Class="PatchOperationAdd">
    <xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath>
    <value><description>top</description></value>
  </Operation>
  <Operation Class="PatchOperationFindMod">
    <mods><li>Lib</li></mods>
    <match Class="PatchOperationSequence"><operations>
      <li Class="PatchOperationAdd"><xpath>/Defs/ThingDef[defName = "Lib_Gun"]</xpath><value><x/></value></li>
    </operations></match>
  </Operation>
  <Operation Class="PatchOperationFindMod">
    <mods><li>Lib</li></mods>
    <match Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="LibXGun"]</xpath><value><x/></value></match>
  </Operation>
</Patch>
`
	delete(compat.Files, "Patches/compat.xml")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Compat"), compat)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lib", "tester.compat")
	scanNow(t, e)
	typed := e.MustRun("patches", "--target", "ThingDef/Lib_Gun")
	bare := e.MustRun("patches", "--target", "Lib_Gun")
	for name, out := range map[string]string{"typed": typed, "bare": bare} {
		if !strings.HasPrefix(out, "patches: 2 operations, 0 failed") || !strings.Contains(out, "Compat.xml:6") {
			t.Fatalf("%s:\n%s", name, out)
		}
	}
	if !strings.Contains(bare, "nested ones included") || strings.Contains(typed, "nested ones included") {
		t.Fatalf("target note:\n%s\n%s", bare, typed)
	}
	// An XPath fragment is still a substring of the top-level XPath only.
	if out := e.MustRun("patches", "--target", `defName="Lib_Gun"`); !strings.HasPrefix(out, "patches: 1 operations") {
		t.Fatalf("fragment:\n%s", out)
	}
	if out := e.MustRun("def", "ThingDef/Lib_Gun"); strings.Count(out, "Compat.xml:") != 2 {
		t.Fatalf("def:\n%s", out)
	}
}
