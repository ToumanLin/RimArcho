package logs

import (
	"os"
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

// A 1.7 (Waylith) log: version from the header, levels read from the line, INFO/DEBUG
// dropped, column-0 continuation lines kept in the entry, logger frames skipped.
func TestParseWaylith(t *testing.T) {
	r, err := os.Open("testdata/waylith.log")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, err := Parse(r)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Waylith || f.Format() != FormatWaylith || f.Version != "1.7.4982 rev1280" {
		t.Fatalf("format %q version %q", f.Format(), f.Version)
	}
	if len(f.Entries) != 9 {
		for _, e := range f.Entries {
			t.Logf("%d %s %q", e.Line, e.Level, e.Message)
		}
		t.Fatalf("%d entries, want 9", len(f.Entries))
	}
	vk := f.Entries[1]
	if vk.Level != "WARN" || vk.Category != "Vulkan" || !strings.HasPrefix(vk.Message, "[General] [Loader Message] Removing layer") ||
		len(vk.Extra) != 2 || vk.Extra[0] != "Objects: 1" {
		t.Fatalf("vulkan entry %+v", vk)
	}
	if top := vk.Frames[0]; top.Type != "Waylith.Graphics.GraphicsDevice" || top.Method != "cctor" {
		t.Fatalf("vulkan top frame %+v", top)
	}

	type want struct{ level, class, msg, top string }
	wants := []want{
		{"WARN", ClassOther, "[General] [Loader Message] Removing layer", "Waylith.Graphics.GraphicsDevice.cctor"},
		{"WARN", ClassXrefMissing, "Could not load reference to", "Verse.DirectXmlCrossRefLoader.ResolveAllWantedCrossReferences"},
		{"ERROR", ClassPatchFailed, "[Compat] Patch operation", "Verse.PatchOperation.Complete"},
		{"WARN", ClassOther, "Some mod did something odd", "OddMod.Startup..ctor"},
		{"ERROR", ClassException, "Error while instantiating a mod of type BadMod.BadMod", "HarmonyLib.MethodCreatorTools.ValidateBinding"},
	}
	cs := Clusters(f)
	if len(cs) != len(wants) {
		for _, c := range cs {
			t.Logf("%s %s %s %q", c.ID, c.Level, c.Class, c.Key)
		}
		t.Fatalf("%d clusters, want %d", len(cs), len(wants))
	}
	for i, w := range wants {
		c := cs[i]
		top := ""
		if len(c.First.Frames) > 0 {
			top = c.First.Frames[0].Type + "." + c.First.Frames[0].Method
		}
		if c.Level != w.level || c.Class != w.class || !strings.HasPrefix(c.First.Message, w.msg) || top != w.top {
			t.Errorf("cluster %d: level %s class %s top %s message %q; want %+v", i, c.Level, c.Class, top, c.First.Message, w)
		}
	}
	// The duplicate stacktrace resolves to the original's frames and joins its cluster.
	if cs[4].Count != 2 || len(f.Entries[8].Frames) != 2 {
		t.Fatalf("dup: count %d frames %d", cs[4].Count, len(f.Entries[8].Frames))
	}
	if pf := cs[2].First; len(pf.Extra) != 1 || !strings.HasPrefix(pf.Extra[0], "file: ") {
		t.Fatalf("patch failure extra %q", pf.Extra)
	}
}

// Unity logs are not mistaken for Waylith ones.
func TestParseUnityNotWaylith(t *testing.T) {
	f, err := Parse(strings.NewReader("Mono path[0] = 'x'\nRimWorld 1.6.4871 rev591\nSomething failed\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Waylith || f.Version != "1.6.4871 rev591" || len(f.Entries) != 3 || f.Entries[2].Level != "" {
		t.Fatalf("%+v", f)
	}
}
