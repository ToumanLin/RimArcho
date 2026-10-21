package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// logRulesLog has one line per attribution rule added from the log corpus (G1 gaps).
const logRulesLog = `RimWorld 1.6.4633 rev1261
Could not load Texture2D at 'Things/Tex/Lamp_north' in any active mod or in base resources.
Could not load Texture2D at 'UI/LogRefs/Icon' in any active mod or in base resources.
Could not load Texture2D at 'UI/Lib/Badge' in any active mod or in base resources.
Failed to find Verse.ThingDef named LogRefs_Missing. There are 12 defs of this type loaded.
Could not resolve cross-reference: No Verse.SoundDef named Self_Sound found to give to Verse.VerbProperties VerbProperties(gun) (using undefined sound instead)
FileNotFoundException: Cannot resolve dependency to assembly 'UnityEngine.InputLegacyModule, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null' because it has not been preloaded. When using the ReflectionOnly APIs, dependent assemblies must be pre-loaded or loaded on demand through the ReflectionOnlyAssemblyResolve event.
FileNotFoundException: Cannot resolve dependency to assembly 'Extra, Version=1.0.0.0, Culture=neutral, PublicKeyToken=null' because it has not been preloaded. When using the ReflectionOnly APIs, dependent assemblies must be pre-loaded or loaded on demand through the ReflectionOnlyAssemblyResolve event.
Could not resolve reference to object with loadID Thing_Human12 of type Verse.Pawn. Was it compressed away, destroyed, had no ID number, or not saved/loaded right? curParent=LogRefs.Holder curPathRelToParent=/target
Could not resolve reference to object with loadID Battle_225 of type Verse.Battle. Was it compressed away, destroyed, had no ID number, or not saved/loaded right? curParent=RimWorld.Pawn_RecordsTracker curPathRelToParent=
Could not find think node with key 12345
Could not load reference to Verse.ThingDef named Gone_Thing
Material 'Custom/X_null' with Shader 'Custom/X' doesn't have a texture property '_MainTex'
`

// logRulesFixture: mods whose files hold the names the log reports missing.
//
//	Texer   tester.texer   ThingDef with texPath Things/Tex/Lamp (a Graphic_Multi base path)
//	TexLib  tester.texlib  ships Textures/UI/Lib/Badge.png, installed but disabled
//	Coder   tester.coder   LogRefs.dll: literals "UI/LogRefs/Icon" and "LogRefs_Missing", type
//	                       LogRefs.Holder, a ReflectionOnlyAssemblyResolve handler
//	Loader  tester.loader  LogLoader.dll: Assembly.ReflectionOnlyLoadFrom
//	Lister  tester.lister  XML that lists LogRefs_Missing and Gone_Thing
//	Selfer  tester.selfer  defines SoundDef Self_Sound and uses it
//	AsmLib  tester.asmlib  ships Assemblies/Extra.dll, installed but disabled
func logRulesFixture(t *testing.T) (*testkit.Env, *app.LogReport) {
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	mod := func(dir, pkg string, files map[string]string, dlls ...string) {
		e.WriteMod(filepath.Join(mods, dir), testkit.Mod{PackageID: pkg, Name: dir, Author: "Tester", Files: files})
		for _, d := range dlls {
			b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), d))
			if err != nil {
				t.Fatal(err)
			}
			e.WriteFile(filepath.Join(mods, dir, "Assemblies", d), string(b))
		}
	}
	mod("Texer", "tester.texer", map[string]string{"Defs/Things.xml": "<Defs>\n  <ThingDef>\n    <defName>Tex_Lamp</defName>\n    <graphicData>\n      <texPath>Things/Tex/Lamp</texPath>\n    </graphicData>\n  </ThingDef>\n</Defs>\n"})
	mod("TexLib", "tester.texlib", map[string]string{"Textures/UI/Lib/Badge.png": "png"})
	mod("Coder", "tester.coder", nil, "LogRefs.dll")
	mod("Loader", "tester.loader", nil, "LogLoader.dll")
	mod("Lister", "tester.lister", map[string]string{"Defs/Lists.xml": "<Defs>\n  <ThingDef>\n    <defName>Lister_Thing</defName>\n    <comps>\n      <li>LogRefs_Missing</li>\n      <li>Gone_Thing</li>\n    </comps>\n  </ThingDef>\n</Defs>\n"})
	mod("Selfer", "tester.selfer", map[string]string{"Defs/Self.xml": "<Defs>\n  <SoundDef>\n    <defName>Self_Sound</defName>\n  </SoundDef>\n  <ThingDef>\n    <defName>Self_Gun</defName>\n    <verbs>\n      <li>\n        <soundCast>Self_Sound</soundCast>\n      </li>\n    </verbs>\n  </ThingDef>\n</Defs>\n"})
	mod("AsmLib", "tester.asmlib", map[string]string{"Assemblies/Extra.dll": "MZ"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.texer", "tester.coder", "tester.loader", "tester.lister", "tester.selfer")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	rep, err := e.App.AnalyzeLogText(logRulesLog)
	if err != nil {
		t.Fatal(err)
	}
	return e, rep
}

// cluster returns the cluster whose key contains sub, its candidates as package → role, and
// its evidence and details as one text.
func ruleCluster(t *testing.T, e *testkit.Env, rep *app.LogReport, sub string) (map[string]string, string) {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, lc := range rep.Clusters {
		if !strings.Contains(lc.Key, sub) {
			continue
		}
		roles := map[string]string{}
		var text []string
		for _, at := range lc.Attr {
			roles[strings.ToLower(st.Ix.ByID[at.Instance].PackageID)] = at.Role
			text = append(text, at.Evidence...)
		}
		return roles, strings.Join(append(text, lc.Details...), "\n")
	}
	t.Fatalf("no cluster %q", sub)
	return nil, ""
}

func TestLogRulesFromCorpus(t *testing.T) {
	e, rep := logRulesFixture(t)
	for _, c := range []struct {
		name, sub string
		want      map[string]string // package → role; nil: no candidates
		text      []string          // evidence or details that must appear
	}{
		{"texture named in a def's XML (Graphic_Multi base path)", "Things/Tex/Lamp_north",
			map[string]string{"tester.texer": "source"}, []string{"references Things/Tex/Lamp at Defs/Things.xml:5", "missing: texture Things/Tex/Lamp_north"}},
		{"texture named by code", "UI/LogRefs/Icon",
			map[string]string{"tester.coder": "source"}, []string{"in its code (Assemblies/LogRefs.dll string literal)"}},
		{"texture shipped by a disabled mod", "UI/Lib/Badge",
			map[string]string{"tester.texlib": "dependency"}, []string{"ships Textures/UI/Lib/Badge.png but is not enabled"}},
		{"GetNamed: code is the source, XML only mentioned", "named LogRefs_Missing",
			map[string]string{"tester.coder": "source", "tester.lister": "mentioned"}, []string{"failed lookup was made by code", "DefDatabase.GetNamed"}},
		{"a mod that defines and uses the missing def", "Self_Sound",
			map[string]string{"tester.selfer": "source"}, []string{"defines it itself (Defs/Self.xml:2)", "update the mod"}},
		{"game assembly: the reflection-only resolver", "UnityEngine.InputLegacyModule",
			map[string]string{"tester.coder": "source", "tester.loader": "mentioned"}, []string{"ships with the game", "AppDomain.ReflectionOnlyAssemblyResolve", "LogLoader.dll"}},
		{"mod assembly shipped by a disabled mod", "assembly 'Extra",
			map[string]string{"tester.asmlib": "dependency"}, []string{"ships Assemblies/Extra.dll but is not enabled"}},
		{"save reference held by a mod's type", "curParent=LogRefs.Holder",
			map[string]string{"tester.coder": "source"}, []string{"its type LogRefs.Holder holds the reference", "Verse.Pawn Thing_Human12"}},
		{"save reference held by the game", "curParent=RimWorld.Pawn_RecordsTracker",
			nil, []string{"held by the game's own RimWorld.Pawn_RecordsTracker"}},
		{"think node keys", "think node with key",
			nil, []string{"patches think trees"}},
		{"a save's def reference is not matched against mod XML", "named Gone_Thing",
			nil, []string{"providers: none among installed mods", "the mod that defined it was removed"}},
		{"_MainTex shader", "_MainTex",
			nil, []string{"shader Custom/X has no _MainTex"}},
	} {
		roles, text := ruleCluster(t, e, rep, c.sub)
		if len(roles) != len(c.want) {
			t.Errorf("%s: candidates %v, want %v\n%s", c.name, roles, c.want, text)
		}
		for pkg, role := range c.want {
			if roles[pkg] != role {
				t.Errorf("%s: %s is %q, want %q (%v)\n%s", c.name, pkg, roles[pkg], role, roles, text)
			}
		}
		for _, s := range c.text {
			if !strings.Contains(text, s) {
				t.Errorf("%s: no %q in\n%s", c.name, s, text)
			}
		}
	}
}
