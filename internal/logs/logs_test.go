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

// A 1.7 (Waylith) log: levels, logger frames removed from the caller stack, the exception stack
// kept apart from the caller stack, the .NET 10 Harmony line form, INFO lines not clustered.
func TestParseWaylith(t *testing.T) {
	text := "=== Waylith log ===\n" +
		"Started:   2026-10-19 11:19:07.807 -05:00\n" +
		"Version:   1.7.4982.1280\n" +
		"\n" +
		"2026-10-19 11:19:07.891  INFO [VulkanStartup] Vulkan loader: bundled.\n" +
		"    at Brutal.Logging.LogCategory.Info(LogString`1 message, String sourceMemberName) [0x0]\n" +
		"    at Waylith.Graphics.VulkanStartup.Prepare(EngineApplicationOptions options) [0xDB]\n" +
		"2026-10-19 11:19:09.518  WARN [Verse] Mod Faction - Orbiters dependency (ICC.AOV.OSEC) needs to have <downloadUrl> specified.\n" +
		"    at Brutal.Logging.LogCategory.Warning(LogString`1 message, String sourceMemberName) [0x0]\n" +
		"    at Verse.Log.Warning(String text) [0x19]\n" +
		"    at Verse.ModMetaData+ModMetaDataInternal.ValidateDependencies(Boolean logIssues) [0x107]\n" +
		"    at Verse.ModMetaData.Init() [0x116]\n" +
		"2026-10-19 11:19:25.082 ERROR [Verse] Exception loading def from file RangedIndustrial.xml: System.ArgumentException: Could not find type named VFECore.ThingDefExtension\n" +
		"[Ref C7CD6A8B]\n" +
		"   at Verse.DirectXmlToObjectNew.ResolveTypeForNode(Type defaultType, XmlNode node)\n" +
		"   at Verse.DirectXmlToObjectNew.DefFromNodeNew_Patch3(XmlNode node, LoadableXmlAsset loadingAsset)\n" +
		"    - PREFIX SmashPhil.VehicleFramework: Verse.DirectXmlToObjectNew.DefFromNodeNew <- Boolean SmashTools.Patching.Patch_XmlParsing:PreProcessAttributesOnDef(Def& __result, XmlNode node)\n" +
		"    at .Verse.Log.Error_Patch3(String text) [0xFFFFFFFF]\n" +
		"    at .Verse.DirectXmlToObjectNew.DefFromNodeNew_Patch3(XmlNode node, LoadableXmlAsset loadingAsset) [0xFFFFFFFF]\n" +
		"2026-10-19 11:19:26.000  WARN [Verse] first line\n" +
		"second line at column 0\n" +
		"    at Verse.Log.Warning(String text) [0x19]\n" +
		"    at MyMod.Thing.Do() [0x1]\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Waylith || f.Version != "1.7.4982 rev1280" || len(f.Entries) != 4 {
		t.Fatalf("waylith %v version %q entries %d", f.Waylith, f.Version, len(f.Entries))
	}
	warn, exc, multi := f.Entries[1], f.Entries[2], f.Entries[3]
	if warn.Level != "WARN" || len(warn.Frames) != 2 || !warn.Frames[0].Caller || warn.Top().Type != "Verse.ModMetaData+ModMetaDataInternal" {
		t.Fatalf("warning %+v", warn)
	}
	if Classify(warn) != ClassOther {
		t.Fatalf("a warning with only a caller stack is %s, not an exception", Classify(warn))
	}
	if exc.Ref != "C7CD6A8B" || len(exc.Frames) != 3 || exc.Frames[0].Caller || !exc.Frames[2].Caller {
		t.Fatalf("exception frames %+v", exc.Frames)
	}
	pf := exc.Frames[1]
	if pf.Type != "Verse.DirectXmlToObjectNew" || pf.Method != "DefFromNodeNew" || !pf.Patched || len(pf.Harmony) != 1 ||
		pf.Harmony[0].Owner != "SmashPhil.VehicleFramework" || pf.Harmony[0].PatchType != "SmashTools.Patching.Patch_XmlParsing" {
		t.Fatalf("patched frame %+v", pf)
	}
	if exc.Top().Method != "ResolveTypeForNode" || exc.Frames[2].Type != "Verse.DirectXmlToObjectNew" {
		t.Fatalf("top %+v, caller %+v", exc.Top(), exc.Frames[2])
	}
	if len(multi.Extra) != 1 || multi.Extra[0] != "second line at column 0" || multi.Top().Type != "MyMod.Thing" {
		t.Fatalf("multi-line warning %+v", multi)
	}
	cs := Clusters(f)
	if len(cs) != 3 || cs[1].Class != ClassXMLLoad || cs[0].Class != ClassOther {
		t.Fatalf("clusters: INFO must not cluster, WARN must; got %d", len(cs))
	}
}

// Frameless 1.6 lines from the log corpus: real problems get a class, a mod's report on itself is
// mod-report (and needs an error word to be kept), engine status lines are not clustered.
func TestClassifyCorpusLines(t *testing.T) {
	for _, c := range []struct {
		msg, class string
		keep       bool
	}{
		{"Could not load Texture2D at 'UI/Ability/Grenade' in any active mod or in base resources.", ClassTexture, true},
		{"Failed to find any textures at Things/Skyfaller/Player/HeavyTransportLifter while constructing Multi(initPath=…)", ClassTexture, true},
		{"Could not resolve reference to object with loadID Lord_64 of type Verse.AI.Group.Lord. Was it compressed away", ClassSave, true},
		{"Object with load ID Thing_Human3262543 is referenced (xml node name: li) but is not deep-saved. This will cause errors during loading.", ClassSave, true},
		{"Failed to find Verse.TerrainDef named AncientHeavyBridge. There are 636 defs of this type loaded.", ClassXrefMissing, true},
		{"Verse.PatchOperationSequence: Error in the operation at position=5", ClassPatchFailed, true},
		{`[SafePatcher]The xPath: "Defs/ThingDef[defName="VFES_Gun_HMGComplex"]" not found`, ClassModReport, true},
		{"<color=#66ffb5>[MeleeAnim]</color> ancot.milirarace 'Milira Race' has 5 missing weapon tweak data.", ClassModReport, true},
		{"TradingControl :: 23.3.24.18192  - 02 - LandingZoneCanBeUsed not found, skipping.", ClassModReport, true},
		{"[RimHUD v1.17.5] Initialized", ClassModReport, false},
		{"GUI Error: You are pushing more GUIClips than you are popping. Make sure they are balanced.", ClassOther, true},
		{"ERROR: Shader GodHand/ObjectOutline shader is not supported on this GPU (none of subshaders/fallbacks are suitable)", ClassOther, true},
		{"Could not get load ID. We're asking for something which was never added during LoadingVars.", ClassSave, true},
		{"Shader 'Camera+/Bordered': fallback shader 'Diffuse' not found", ClassOther, false},
		{"Translation data for language English has 25 errors. Generate translation report for more info.", ClassOther, false},
		{"[XML Extensions] 17555 total patch operations run in 199805ms, 4 failed", ClassModReport, false},
	} {
		e := &Entry{Message: c.msg}
		if got := Classify(e); got != c.class {
			t.Errorf("%q: class %s, want %s", c.msg, got, c.class)
		}
		if got := Interesting(e); got != c.keep {
			t.Errorf("%q: interesting %v, want %v", c.msg, got, c.keep)
		}
	}
}

func TestTemplate(t *testing.T) {
	for _, c := range [][2]string{
		{"Could not load Texture2D at 'UI/Ability/Grenade' in any active mod or in base resources.",
			"Could not load Texture#D at '…' in any active mod or in base resources."},
		{"Could not load Texture2D at 'Things/Apparel/Beret/headgear_icon' for def 'DMS_Apparel_Beret' in any active mod or in base resources.",
			"Could not load Texture#D at '…' for def '…' in any active mod or in base resources."},
		{`[SafePatcher]The xPath: "Defs/ThingDef[defName="VFES_Gun_HMGComplex"]" not found`,
			`[SafePatcher]The xPath: "…" not found`},
		{"<color=#66ffb5>[MeleeAnim]</color> ancot.milirarace 'Milira Race' has 5 missing weapon tweak data.",
			"[MeleeAnim] <pkg> '…' has # missing weapon tweak data."},
		{"Type DoExtraIconsTweak probably needs a StaticConstructorOnStartup attribute, because it has a field TechprintIcon_Missing of type Texture2D",
			"Type … probably needs a StaticConstructorOnStartup attribute, because it has a field … of type …"},
		{"could not find original method through ReflectedType or TypeByName(WorkGiver_DoBill), continuing",
			"could not find original method through ReflectedType or TypeByName(…), continuing"},
	} {
		if got := Template(c[0]); got != c[1] {
			t.Errorf("Template(%q)\n got %q\nwant %q", c[0], got, c[1])
		}
	}
}

// A Mono native frame at column 0 continues the stack (the log corpus's most frequent template
// was this line as a message of its own, with the exception's stack cut in two).
func TestParseNativeFrame(t *testing.T) {
	text := "Exception in Verse.Mod constructor: System.Reflection.TargetInvocationException: boom\n" +
		"  at HarmonyLib.Harmony.PatchAll () [0x0001d] in <8124cc12bdf242eab0a5f7e7edecf387>:0 \n" +
		"<0x2693319a2b0 + 0x000d2> <unknown method>\n" +
		"(wrapper managed-to-native) System.Reflection.RuntimeConstructorInfo.InternalInvoke(System.Reflection.RuntimeConstructorInfo,object,object[],System.Exception&)\n" +
		"  at MyMod.Startup..ctor () [0x00005] in <51fded79cd284d4d911c5949aff4cb21>:0 \n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 1 || len(f.Entries[0].Frames) != 3 || f.Entries[0].Top().Type != "HarmonyLib.Harmony" {
		t.Fatalf("entries %d, frames %+v", len(f.Entries), f.Entries[0].Frames)
	}
}

// An XML Extensions error block becomes one patch-failed entry named after the mod, with the
// source file as a "file:" line (28% of the log corpus has such blocks).
func TestParseXMLExtensionsBlock(t *testing.T) {
	text := "2025-07-15T10:05:28.296Z|0x60d0|[ReBuild: Doors and Corners - Start of stack trace]\n" +
		"Verse.PatchOperationAdd(xpath=\"Defs/ThingDef[defName=\"VFEArch_CellDoor\"]\"): Failed to find a node with the given xpath\n" +
		"Verse.PatchOperationSequence: Error in the operation at position=2\n" +
		"[End of stack trace]\n" +
		`Source file: c:\program files (x86)\steam\steamapps\workshop\content\294100\3262718980\1.6\Patches\PerspectiveDoors-Mods.xml` + "\n" +
		"[[FSF] Vanilla Bionics Expansion - Start of stack trace]\n" +
		"Verse.PatchOperationReplace(xpath=\"/Defs/ThingDef[@Name = \"BodyPartArchotechBase\"]/graphicData/color\"): Failed to find a node with the given xpath\n" +
		"[End of stack trace]\n" +
		"GAGARIN: Cache created!\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 3 {
		t.Fatalf("%d entries, want 3", len(f.Entries))
	}
	a, b := f.Entries[0], f.Entries[1]
	if !strings.HasPrefix(a.Message, "[ReBuild: Doors and Corners] Verse.PatchOperationAdd(") || Classify(a) != ClassPatchFailed ||
		len(a.Extra) != 2 || !strings.HasPrefix(a.Extra[1], `file: c:\program files`) {
		t.Fatalf("first block %q %q class %s", a.Message, a.Extra, Classify(a))
	}
	if !strings.HasPrefix(b.Message, "[[FSF] Vanilla Bionics Expansion] Verse.PatchOperationReplace(") || len(b.Extra) != 0 {
		t.Fatalf("second block (no source file) %q %q", b.Message, b.Extra)
	}
	if f.Entries[2].Message != "GAGARIN: Cache created!" {
		t.Fatalf("line after a block: %q", f.Entries[2].Message)
	}
}
