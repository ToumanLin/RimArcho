package logs

import "testing"

func TestILDump(t *testing.T) {
	for _, m := range []string{
		"AdjustParms - ldfld System.Collections.Generic.List`1[RimWorld.RenderSkipFlagDef] renderSkipFlags [0],",
		"AdjustParms - callvirt Verse.Pawn get_RaceProps()",
		"IL_001a: ldarg.0",
		"stloc.s 5 (System.Int32)",
		"call static System.Void Foo::Bar()",
		"ret",
	} {
		if !ILDump(m) {
			t.Errorf("not an IL line: %q", m)
		}
	}
	for _, m := range []string{
		"Exception ticking Pawn1: System.NullReferenceException",
		"Could not resolve cross-reference: No Verse.ThingDef named Lib_Gun found",
		"[Compat] Patch operation Verse.PatchOperationAdd failed",
		"called from Foo",
		"Loaded file (Mods/X/Defs/a.xml) is null",
		"stats - loaded 12 defs",
		"Retrying load of Foo",
	} {
		if ILDump(m) {
			t.Errorf("IL line: %q", m)
		}
	}
}

func TestILDumpProse(t *testing.T) {
	for _, m := range []string{"and then it failed", "switch to the main menu", "call me later", "box", "throw away"} {
		if ILDump(m) {
			t.Errorf("IL line: %q", m)
		}
	}
}
