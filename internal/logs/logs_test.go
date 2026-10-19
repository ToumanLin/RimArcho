package logs

import (
	"strings"
	"testing"
)

// A pasted stack trace: a two-line exception message, frames, Harmony lines on the patched
// frame (the I4 self-check found the second message line split into its own entry).
func TestParsePastedStack(t *testing.T) {
	text := "Exception ticking Human123 (at (120, 0, 80)): System.ArgumentOutOfRangeException: Index was out of range.\n" +
		"Parameter name: index\n" +
		"[Ref 7F3A21C0]\n" +
		"  at System.Collections.Generic.List`1[T].get_Item (System.Int32 index) [0x00009] in <00000000000000000000000000000000>:0\n" +
		"  at Verse.ThingWithComps.Tick () [0x0001a] in <00000000000000000000000000000000>:0\n" +
		"  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.Tick_Patch1(Verse.Pawn)\n" +
		"    - PREFIX palpha.godhands: Boolean GodHandMod.HarmonyPatches_StasisLock+Patch_Pawn_Tick_Freeze:Prefix(Verse.Pawn __instance)\n" +
		"    - POSTFIX mlie.yayoscombat3: Void yayoCombat.HarmonyPatches.Pawn_Tick:Postfix(Verse.Pawn __instance)\n" +
		"  at Verse.TickList.Tick () [0x0015c] in <00000000000000000000000000000000>:0\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("%d entries, want 1", len(f.Entries))
	}
	e := f.Entries[0]
	if len(e.Frames) != 4 || e.Ref != "7F3A21C0" || e.Extra[0] != "Parameter name: index" {
		t.Fatalf("frames %d ref %q extra %q", len(e.Frames), e.Ref, e.Extra)
	}
	pt := e.Frames[2]
	if pt.Type != "Verse.Pawn" || pt.Method != "Tick" || !pt.Patched || len(pt.Harmony) != 2 || pt.Harmony[0].PatchType != "GodHandMod.HarmonyPatches_StasisLock+Patch_Pawn_Tick_Freeze" {
		t.Fatalf("patched frame %+v", pt)
	}
	cs := Clusters(f)
	if len(cs) != 1 || cs[0].Class != ClassHarmony {
		t.Fatalf("clusters %+v", cs)
	}
}
