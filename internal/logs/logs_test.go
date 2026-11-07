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

// A log uploaded by HugsLib: its header (Mod list, Harmony patches, some with "Error" in a
// method name) is not part of the game's log, and the game's version and the 1.7 format are
// found after it, however long it is.
func TestParseHugsLibUpload(t *testing.T) {
	header := "Log uploaded on Thursday, October 22, 2026, 12:03:55 PM\n" +
		"Loaded mods:\n" +
		"Harmony(brrainz.harmony)[v:2.4.1.0]: 0Harmony(2.4.1), HarmonyMod(2.4.1)\n" +
		"Active Harmony patches:\n" +
		strings.Repeat("Log.Error: PRE: ModA.Patches.Log_Error_Patch.Prefix\n", 80) +
		"Harmony versions present: 2.4.1.0: net.pardeike.rimworld.lib.harmony\n" +
		"Platform information: (hidden, use publishing options to include)\n" +
		"\n"
	for _, c := range []struct {
		name, log, version string
		waylith            bool
		entries            int
	}{
		{"1.6", header + "Log file contents:\nMono path[0] = 'C:/RimWorld/RimWorldWin64_Data/Managed'\nRimWorld 1.6.4633 rev1261\n" +
			"Could not find a type named ModA.Thing\n", "1.6.4633 rev1261", false, 3},
		{"1.7", header + "Log file contents:\n=== Waylith log ===\nVersion:   1.7.4985.1269\n\n" +
			"2026-10-22 11:30:11.692  WARN [Verse] Could not find a type named ModA.Thing\n", "1.7.4985 rev1269", true, 1},
		{"cut short", header + "RimWorld 1.6.4633 rev1261\nCould not find a type named ModA.Thing\n", "1.6.4633 rev1261", false, 2},
	} {
		f, err := Parse(strings.NewReader(c.log))
		if err != nil {
			t.Fatal(err)
		}
		if f.Version != c.version || f.Waylith != c.waylith || len(f.Entries) != c.entries {
			for _, e := range f.Entries {
				t.Logf("%s: entry %d %q", c.name, e.Line, e.Message)
			}
			t.Errorf("%s: version %q waylith %v entries %d, want %q %v %d", c.name, f.Version, f.Waylith, len(f.Entries), c.version, c.waylith, c.entries)
		}
		if cs := Clusters(f); len(cs) != 1 || !strings.Contains(cs[0].Template, "ModA.Thing") && !strings.Contains(cs[0].Template, "Could not find a type") {
			t.Errorf("%s: clusters %+v", c.name, cs)
		}
	}
}

// Unity's "Type:Method (args)" stacks in a 1.6 log: the stack Unity adds to every message when
// full stack traces are on (logger frames dropped, the rest caller frames), Unity's own exception
// stack after "[Ref X] Duplicate stacktrace" (exception frames), and a mod message with a stack
// (caller frames, still not interesting). No frame line becomes an entry of its own.
func TestParseUnityStacks(t *testing.T) {
	text := "RimWorld 1.6.4633 rev1261\n" +
		"Initializing new game\n" +
		"UnityEngine.StackTraceUtility:ExtractStackTrace ()\n" +
		"UnityEngine.DebugLogHandler:LogFormat (UnityEngine.LogType,UnityEngine.Object,string,object[])\n" +
		"UnityEngine.Logger:Log (UnityEngine.LogType,object)\n" +
		"UnityEngine.Debug:Log (object)\n" +
		"Verse.Log:Message (string)\n" +
		"Verse.Root/<>c:<Start>b__10_1 ()\n" +
		"\n" +
		"Error in static constructor of ModA.Boot: System.TypeInitializationException: The type initializer for 'ModA.Boot' threw an exception.\n" +
		"[Ref 5CA72C60]\n" +
		"  at Verse.StaticConstructorOnStartupUtility.CallAll () [0x00025] in <cb53cd4422904947932d33f561ad8d15>:0 \n" +
		"UnityEngine.StackTraceUtility:ExtractStackTrace ()\n" +
		"UnityEngine.Debug:LogError (object)\n" +
		"Verse.Log:Error (string)\n" +
		"Verse.StaticConstructorOnStartupUtility:CallAll ()\n" +
		"(wrapper dynamic-method) MonoMod.Utils.DynamicMethodDefinition:Verse.Root.Update_Patch1 (Verse.Root)\n" +
		"\n" +
		"ArgumentOutOfRangeException: Index was out of range. Must be non-negative and less than the size of the collection.\n" +
		"Parameter name: index\n" +
		"[Ref DA333A7B] Duplicate stacktrace, see ref for original\n" +
		"UnityEngine.GameObject:AddComponent(Type)\n" +
		"ModB.Shots:<Make>b__3_0()\n" +
		"System.Collections.Generic.List`1<Verse.Thing>:ForEach(System.Action`1<Verse.Thing>)\n" +
		"System.Threading.Tasks.Parallel/<>c__DisplayClass33_0`2<System.Type, object>:<ForEachWorker>b__0 (int)\n" +
		"HarmonyLib.CollectionExtensions:Do<System.Type> (System.Collections.Generic.IEnumerable`1<System.Type>,System.Action`1<System.Type>)\n" +
		"MonoMod.Utils.DynamicMethodDefinition:Verse.Map.MapUpdate_Patch2(Map)\n" +
		"\n" +
		"GAGARIN: cache disabled\n" +
		"Gagarin.LoadableXmlAsset_Constructor_Patch:Process():0\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 5 {
		for _, e := range f.Entries {
			t.Logf("entry %q", e.Message)
		}
		t.Fatalf("entries %d, want 5", len(f.Entries))
	}
	msg, sc, exc, mod := f.Entries[1], f.Entries[2], f.Entries[3], f.Entries[4]
	if len(msg.Frames) != 1 || !msg.Frames[0].Caller || msg.Top().Type != "Verse.Root" || msg.Top().Method != "Start" {
		t.Fatalf("message frames %+v", msg.Frames)
	}
	if ok, _ := interesting(msg); ok {
		t.Fatal("a message with a full stack trace is not a problem")
	}
	if len(sc.Frames) != 3 || sc.Frames[0].Caller || !sc.Frames[1].Caller || !sc.Frames[2].Caller ||
		sc.Frames[2].Type != "Verse.Root" || sc.Frames[2].Method != "Update" || !sc.Frames[2].Patched {
		t.Fatalf("static constructor frames %+v", sc.Frames)
	}
	want := [][2]string{{"UnityEngine.GameObject", "AddComponent"}, {"ModB.Shots", "Make"}, {"System.Collections.Generic.List`1", "ForEach"},
		{"System.Threading.Tasks.Parallel", "ForEachWorker"}, {"HarmonyLib.CollectionExtensions", "Do"}, {"Verse.Map", "MapUpdate"}}
	if !exc.Dup || len(exc.Frames) != len(want) {
		t.Fatalf("exception frames %+v", exc.Frames)
	}
	for i, w := range want {
		if fr := exc.Frames[i]; fr.Caller || fr.Type != w[0] || fr.Method != w[1] {
			t.Errorf("frame %d = %s.%s caller %v, want %s.%s", i, fr.Type, fr.Method, fr.Caller, w[0], w[1])
		}
	}
	if !exc.Frames[5].Patched || Classify(exc) != ClassException {
		t.Fatalf("patched %v class %s", exc.Frames[5].Patched, Classify(exc))
	}
	if len(mod.Frames) != 1 || !mod.Frames[0].Caller || mod.Top().Type != "Gagarin.LoadableXmlAsset_Constructor_Patch" {
		t.Fatalf("mod message frames %+v", mod.Frames)
	}
	if ok, _ := interesting(mod); ok {
		t.Fatal("a mod's status message with a stack is not a problem")
	}
	for _, c := range Clusters(f) {
		if strings.Contains(c.Template, ":") && !strings.Contains(c.Template, " ") {
			t.Errorf("a frame line became a cluster: %q", c.Template)
		}
	}
}

// An XML Extensions block in a 1.7 log becomes "[Mod] <first failure>" with its file, as in 1.6:
// a patch failure, one cluster per failure, the caller stack kept as caller frames.
func TestParseWaylithXMLExtensions(t *testing.T) {
	block := func(ts, def string) string {
		return ts + " ERROR [Verse] [Progression: Arsenal - Start of stack trace]\n" +
			`Verse.PatchOperationReplace(xpath="Defs/ResearchProjectDef[defName="` + def + `"]/label"): Failed to find a node with the given xpath` + "\n" +
			"Verse.PatchOperationSequence: Error in the operation at position=2\n" +
			"[End of stack trace]\n" +
			`Source file: D:\Mods\Arsenal\Patches\` + def + ".xml\n" +
			"\n" +
			"    at XmlExtensions.ErrorManager.PrintErrors(String source, ModContentPack mod) [0x6A]\n" +
			"    at .Verse.PatchOperation.Apply_Patch2(PatchOperation this, XmlDocument xml) [0xFFFFFFFF]\n"
	}
	text := "=== Waylith log ===\nVersion:   1.7.4985.1269\n\n" +
		block("2026-10-22 11:31:14.428", "VWE_TrenchWarfare") +
		"2026-10-22 11:31:14.430  INFO [Verse] Loaded 12 patches\n" +
		block("2026-10-22 11:31:14.431", "VWE_Artillery") +
		"2026-10-22 11:31:14.432 ERROR [Verse] [Cut - Start of stack trace]\n" +
		`Verse.PatchOperationAdd(xpath="Defs/ThingDef"): Failed to find a node with the given xpath` + "\n" +
		"    at XmlExtensions.ErrorManager.PrintErrors(String source, ModContentPack mod) [0x6A]\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	var got []*Entry
	for _, e := range f.Entries {
		if e.Level == "ERROR" {
			got = append(got, e)
		}
	}
	if len(got) != 3 {
		t.Fatalf("error entries %d, want 3", len(got))
	}
	e := got[0]
	if !strings.HasPrefix(e.Message, `[Progression: Arsenal] Verse.PatchOperationReplace(xpath="Defs/ResearchProjectDef[defName="VWE_TrenchWarfare"]`) ||
		len(e.Extra) != 2 || e.Extra[0] != "Verse.PatchOperationSequence: Error in the operation at position=2" || e.Extra[1] != `file: D:\Mods\Arsenal\Patches\VWE_TrenchWarfare.xml` {
		t.Fatalf("block %q extra %q", e.Message, e.Extra)
	}
	if len(e.Frames) != 2 || !e.Frames[0].Caller || Classify(e) != ClassPatchFailed {
		t.Fatalf("frames %+v class %s", e.Frames, Classify(e))
	}
	if cut := got[2]; !strings.HasPrefix(cut.Message, "[Cut] Verse.PatchOperationAdd(") || len(cut.Frames) != 1 {
		t.Fatalf("block without its end line: %q frames %d", cut.Message, len(cut.Frames))
	}
	cs := Clusters(f)
	if len(cs) != 3 {
		for _, c := range cs {
			t.Logf("cluster %s %q", c.Class, c.Template)
		}
		t.Fatalf("clusters %d, want one per failure (3)", len(cs))
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
		// XML Extensions blocks: a mod's own operation class, an "Error" ending, an xpath cut by a
		// line break, brackets in the mod's name.
		{`[1trickPwnyta's Anomaly Patch] AnomalyPatch.PatchOperationRemoveIf(xpath="/Defs/BackstoryDef[defName="UnknownAdulthood"]/bodyTypeGlobal"): Failed to find a node with the given xpath`, ClassPatchFailed, true},
		{`[JustFloorTiles] Verse.PatchOperationAttributeAdd(xpath=Defs/TerrainDef[defName="WoodPlankFloor"], attribute=Name, value=WoodPlankBase): Error`, ClassPatchFailed, true},
		{`[Combat Extended] Verse.PatchOperationAdd(xpath="Defs/thingDef[defName="Gun_A" or`, ClassPatchFailed, true},
		{`[[FSF] Vanilla Bionics Expansion] Verse.PatchOperationReplace(xpath="Defs/HediffDef"): Failed to find a node with the given xpath`, ClassPatchFailed, true},
		{`[SafePatcher]The xPath: "Defs/ThingDef[defName="VFES_Gun_HMGComplex"]" not found`, ClassModReport, true},
		{"<color=#66ffb5>[MeleeAnim]</color> ancot.milirarace 'Milira Race' has 5 missing weapon tweak data.", ClassModReport, true},
		{"TradingControl :: 23.3.24.18192  - 02 - LandingZoneCanBeUsed not found, skipping.", ClassModReport, true},
		{"[RimHUD v1.17.5] Initialized", ClassModReport, false},
		{"GUI Error: You are pushing more GUIClips than you are popping. Make sure they are balanced.", ClassOther, true},
		{"ERROR: Shader GodHand/ObjectOutline shader is not supported on this GPU (none of subshaders/fallbacks are suitable)", ClassOther, true},
		{"Could not get load ID. We're asking for something which was never added during LoadingVars.", ClassSave, true},
		{"Shader 'Camera+/Bordered': fallback shader 'Diffuse' not found", ClassOther, false},
		// Known noise: an informational cluster with a note (KnownNoise), not dropped.
		{"Translation data for language English has 25 errors. Generate translation report for more info.", ClassNoise, true},
		{"[XML Extensions] 17555 total patch operations run in 199805ms, 4 failed", ClassModReport, false},
		// 1.6 Log.Error without a stack in the game's wording: kept; game-error when it names mod code,
		// also when the game glued a word to the name ("lord jobNs.Type").
		{"Tried to add pawn Ann to lord with lord jobProgressionEducation.LordJob_AttendClass but this pawn is already a member of lord with lord jobRecreationalSexWithEuterpe.LordJob_Birthday. Pawns can't be members of more than one lord at the same time.", ClassGameError, true},
		{"Tried to register in RegisterforPostLoadInit when already registered: AdaptiveStorage.StorageGroup", ClassGameError, true},
		{"Tried to add pawn Militor5 to lord with lord job but this pawn is already a member of lord with lord jobRimWorld.LordJob_MechanoidsDefend. Pawns can't be members of more than one lord at the same time.", ClassOther, true},
		{"Tried to register the same load ID twice: Thing_Steel123, pathRelToParent=/things/li", ClassOther, true},
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

// The solid-colour material made in code and the translation error count are known noise:
// informational clusters with a note, whatever their stack (MCP transcript review 2026-11-04
// §2.2). A material of a def's icon stays a material problem.
func TestKnownNoise(t *testing.T) {
	solid := "Material 'SolidColorMat-Custom/Solid Color-RGBA(0.000, 0.000, 0.000, 0.000)' with Shader 'Custom/Solid Color' doesn't have a texture property '_MainTex'"
	trans := "Translation data for language English has 19 errors. Generate translation report for more info."
	defMat := "Material 'MyMod_Icon' with Shader 'Custom/Solid Color' doesn't have a texture property '_MainTex'"
	for _, c := range []struct {
		msg, id string
	}{
		{solid, "solid-color-material"},
		{"Material 'SolidColorMat-Standard-RGBA(1.000, 0.500, 0.000, 1.000)' with Shader 'Standard' doesn't have a texture property '_MainTex'", "solid-color-material"},
		{trans, "translation-count"},
		{"Translation data for language ChineseSimplified (Simplified Chinese) has 3 errors. Generate translation report for more info.", "translation-count"},
		{defMat, ""},
		{"Translation data for language English loaded.", ""},
	} {
		n := KnownNoise(c.msg)
		got := ""
		if n != nil {
			got = n.ID
		}
		if got != c.id {
			t.Errorf("%q: known noise %q, want %q", c.msg, got, c.id)
		}
		if n == nil {
			continue
		}
		e := &Entry{Message: c.msg}
		if Classify(e) != ClassNoise || !Interesting(e) || !Informational(ClassNoise) {
			t.Errorf("%q: class %s, interesting %v: want an informational noise cluster", c.msg, Classify(e), Interesting(e))
		}
		if n.Note == "" || n.Label == "" || strings.Contains(n.Note, "uiIconColor but") {
			t.Errorf("%s: note %q", n.ID, n.Note)
		}
	}
	if c := Classify(&Entry{Message: defMat}); c != ClassMaterial {
		t.Errorf("a def's material: class %s, want material", c)
	}
	// With a stack (the companion log's copy carries one) it is still noise.
	f, err := Parse(strings.NewReader(solid + "\n" + solid + "\n" + trans + "\n  at Verse.Log.Error ()\n"))
	if err != nil {
		t.Fatal(err)
	}
	cs := Clusters(f)
	if len(cs) != 2 || cs[0].Count != 2 {
		t.Fatalf("clusters: %d, want the material x2 and the translation count", len(cs))
	}
	for _, c := range cs {
		if c.Class != ClassNoise {
			t.Errorf("%s: class %s", c.Key, c.Class)
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

// "Exception loading … from file." names the file on the next line; each file is its own cluster.
func TestParseAbsFilePath(t *testing.T) {
	text := "Exception loading UnityEngine.AudioClip from file.\n" +
		"absFilePath: [Rimworld_dir]/Mods/A/Sounds/x.mp3\n" +
		"Exception: System.ArgumentNullException: Value cannot be null.\n" +
		"Exception loading UnityEngine.AudioClip from file.\n" +
		"absFilePath: [Rimworld_dir]/Mods/B/Sounds/y.mp3\n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 3 || len(f.Entries[0].Extra) != 1 || f.Entries[0].Extra[0] != "absFilePath: [Rimworld_dir]/Mods/A/Sounds/x.mp3" {
		t.Fatalf("entries: %+v", f.Entries)
	}
	if k0, _, _ := ClusterKey(f.Entries[0]); k0 == func() string { k, _, _ := ClusterKey(f.Entries[2]); return k }() {
		t.Fatalf("two files share a cluster key %q", k0)
	}
}

func TestUnglue(t *testing.T) {
	for in, want := range map[string]string{
		"jobProgressionEducation.LordJob_AttendClass": "ProgressionEducation.LordJob_AttendClass",
		"jobRimWorld.LordJob_Joinable_Party":          "RimWorld.LordJob_Joinable_Party",
		"RimWorld.LordJob_Joinable_Party":             "RimWorld.LordJob_Joinable_Party",
		"aRandomKiwi.RimThemes":                       "RandomKiwi.RimThemes", // callers try the name first
		"lowercase.only":                              "lowercase.only",
		"ModA.Boot":                                   "ModA.Boot",
	} {
		if got := Unglue(in); got != want {
			t.Errorf("Unglue(%q) = %q, want %q", in, got, want)
		}
	}
}

// Lord.AddPawn errors of different pawns are one cluster.
func TestNormalizeLordPawn(t *testing.T) {
	a := Normalize("Tried to add pawn Big Red to lord with lord jobModA.LordJob_X but this pawn is already a member of lord with lord jobModB.LordJob_Y.")
	b := Normalize("Tried to add pawn Ann to lord with lord jobModA.LordJob_X but this pawn is already a member of lord with lord jobModB.LordJob_Y.")
	if a != b || !strings.Contains(a, "<pawn>") {
		t.Errorf("keys differ or keep the name:\n%s\n%s", a, b)
	}
}

// One bug in a pawn comp throws for every colonist and world pawn: one cluster, not one per name
// (the SSR Body repro had 20). A ThingID keeps its defName.
func TestNormalizeTickPawn(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"Exception ticking Hubbard (at (121, 0, 135)): System.InvalidOperationException: Collection was modified; enumeration operation may not execute.",
			"Exception ticking 'Big' Jim (at (116, 0, 138)): System.InvalidOperationException: Collection was modified; enumeration operation may not execute.",
			"Exception ticking <pawn> (at (#, #, #)): System.InvalidOperationException: Collection was modified; enumeration operation may not execute."},
		{"Exception ticking Nik. Suppressing further errors. Exception: System.ArgumentOutOfRangeException: Index was out of range.",
			"Exception ticking Teej (at (122, 0, 122)). Suppressing further errors. Exception: System.ArgumentOutOfRangeException: Index was out of range.",
			""},
		{"Exception ticking world pawn Martin. Suppressing further errors. System.InvalidOperationException: Collection was modified",
			"Exception ticking world pawn Irobarmar. Suppressing further errors. System.InvalidOperationException: Collection was modified",
			"Exception ticking world pawn <pawn>. Suppressing further errors. System.InvalidOperationException: Collection was modified"},
		{"Exception ticking hediff (Flu Torso ticksSinceCreation=40) for pawn Ann. Removing hediff... Exception: System.NullReferenceException",
			"Exception ticking hediff (Flu Torso ticksSinceCreation=90) for pawn Big Red. Removing hediff... Exception: System.NullReferenceException",
			"Exception ticking hediff (Flu Torso ticksSinceCreation=#) for pawn <pawn>. Removing hediff... Exception: System.NullReferenceException"},
	}
	for _, c := range cases {
		a, b := Normalize(c.a), Normalize(c.b)
		if a != b && c.want != "" || c.want != "" && a != c.want || !strings.Contains(a, "<pawn>") {
			t.Errorf("\n%s\n%s", a, b)
		}
	}
	for _, m := range []string{
		"Exception ticking Wall12345 (at (3, 0, 4)): System.NullReferenceException",
		"Exception ticking UnnamedDef-7. Suppressing further errors. Exception: System.NullReferenceException",
		"Exception ticking Pawn7: System.NullReferenceException",
	} {
		if n := Normalize(m); strings.Contains(n, "<pawn>") {
			t.Errorf("ThingID replaced: %s", n)
		}
	}
	if Normalize("Exception ticking Wall12 (at (3, 0, 4)): x") == Normalize("Exception ticking Door12 (at (3, 0, 4)): x") {
		t.Error("ThingIDs of different defs merged")
	}
}
