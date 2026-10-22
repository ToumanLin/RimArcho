package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// logCallersLog: one entry per rule of logcallers.go and the field-context filter (G7).
const logCallersLog = `RimWorld 1.6.4633 rev1261
Error in static constructor of ModA.Boot: System.TypeInitializationException: The type initializer for 'ModA.Boot' threw an exception. ---> HarmonyLib.HarmonyException: Patching exception
  at HarmonyLib.PatchClassProcessor.Patch () [0x00000] in <abc>:0
  at System.Runtime.CompilerServices.RuntimeHelpers.RunClassConstructor (System.RuntimeTypeHandle type) [0x00000] in <abc>:0
  at ModB.Boot.Init () [0x00000] in <abc>:0
Error in static constructor of LogRefs.Gone.Main: System.TypeInitializationException: The type initializer for 'LogRefs.Gone.Main' threw an exception.
  at ModB.Pawn_Kill.Prefix () [0x00000] in <abc>:0
[HugsLib][ERR] HTTP/1.1 400 Bad Request
  at HugsLib.Logs.LogPublisher.OnRequestError (System.String error) [0x00000] in <abc>:0
Could not resolve type with token 0100006c from typeref (expected class 'Harmony.CodeInstruction' in assembly '0Harmony, Version=1.2.0.1, Culture=neutral, PublicKeyToken=null')
  at System.Reflection.Assembly.GetTypes () [0x00000] in <abc>:0
  at Verse.ModAssemblyHandler.AssemblyIsUsable (System.Reflection.Assembly asm) [0x00000] in <abc>:0
ReflectionTypeLoadException getting types in assembly AchtungMod: System.Reflection.ReflectionTypeLoadException: Exception of type 'System.Reflection.ReflectionTypeLoadException' was thrown.
Error patching RimWorld.Foo - Void Bar() - System.TypeInitializationException: The type initializer for 'ModA.Boot' threw an exception. ---> System.UnauthorizedAccessException: Access to the path "[Home_dir]/Desktop/harmony.log.txt" is denied.
System.TypeLoadException: Could not resolve type with token 01000030 from typeref (expected class 'Other.Thing' in assembly 'Other, Version=1.0.0.0, Culture=neutral, PublicKeyToken=null')
ReflectionTypeLoadException getting types in assembly ModB: System.Reflection.ReflectionTypeLoadException: Exception of type 'System.Reflection.ReflectionTypeLoadException' was thrown.
System.TypeLoadException: Could not load type of field 'LogRefs.Holder+<>c:<>9__1' (4) due to: Could not resolve type with token 0100017b from typeref (expected class 'System.Linq.IGrouping' in assembly 'System.Linq, Version=10.0.0.0')
XML error: <selectionWeight>2</selectionWeight> doesn't correspond to any field in type LayoutRoomDef. Context: <LayoutRoomDef><defName>Ancient_Missing</defName><selectionWeight>2</selectionWeight></LayoutRoomDef>
`

func TestLogCallerRules(t *testing.T) {
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
	mod("Failer", "tester.failer", nil, "ModA.dll")
	mod("Caller", "tester.caller", nil, "ModB.dll")
	mod("Coder", "tester.coder", nil, "LogRefs.dll")
	mod("HugsLib", "unlimitedhugs.hugslib", nil)
	mod("Gener", "tester.gener", map[string]string{"Defs/Genes.xml": "<Defs>\n  <GeneDef>\n    <defName>Some_Gene</defName>\n    <selectionWeight>2</selectionWeight>\n  </GeneDef>\n</Defs>\n"})
	mod("Roomer", "tester.roomer", map[string]string{"Defs/Rooms.xml": "<Defs>\n  <LayoutRoomDef>\n    <defName>Roomer_Room</defName>\n    <selectionWeight>2</selectionWeight>\n  </LayoutRoomDef>\n</Defs>\n"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.failer", "tester.caller", "tester.coder", "unlimitedhugs.hugslib", "tester.gener", "tester.roomer")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	rep, err := e.App.AnalyzeLogText(logCallersLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, sub string
		want      map[string]string // package → role (only the listed ones are checked)
		absent    []string
		text      []string
	}{
		{"failing type's owner, not the caller", "static constructor of ModA.Boot",
			map[string]string{"tester.failer": "source", "tester.caller": "mentioned"}, nil, []string{"calls into the failing code"}},
		{"failing type by its namespace's assembly", "static constructor of LogRefs.Gone.Main",
			map[string]string{"tester.coder": "source", "tester.caller": "mentioned"}, nil, []string{"its assembly LogRefs.dll holds namespace LogRefs"}},
		{"HugsLib log upload", "[HugsLib][ERR]",
			map[string]string{"unlimitedhugs.hugslib": "mentioned"}, nil, []string{"log upload failed"}},
		{"Harmony 1.x type reference", "Harmony.CodeInstruction",
			nil, nil, []string{"Harmony 1.x", "no enabled mod loads AchtungMod.dll"}},
		{"unwritable harmony.log.txt is the machine", "Error patching RimWorld.Foo",
			map[string]string{"tester.failer": "mentioned"}, nil, []string{"harmony.log.txt cannot be written"}},
		{"type reference: the assembly named on the next line", "expected class 'Other.Thing'",
			map[string]string{"tester.caller": "source"}, nil, []string{"loads ModB.dll"}},
		{"type of a field: its declaring type's mod", "Could not load type of field",
			map[string]string{"tester.coder": "source"}, nil, nil},
		{"field error only in files of the context's def type", "<selectionWeight>",
			map[string]string{"tester.roomer": "source"}, []string{"tester.gener"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			roles, text := ruleCluster(t, e, rep, c.sub)
			for pkg, role := range c.want {
				if roles[pkg] != role {
					t.Errorf("%s: role %q, want %q (all: %v)\n%s", pkg, roles[pkg], role, roles, text)
				}
			}
			for _, pkg := range c.absent {
				if _, ok := roles[pkg]; ok {
					t.Errorf("%s should not be a candidate (all: %v)", pkg, roles)
				}
			}
			if c.want == nil {
				for pkg, role := range roles {
					if role == "dependency" || role == "source" {
						t.Errorf("%s is %s; want no dependency/source", pkg, role)
					}
				}
			}
			for _, s := range c.text {
				if !strings.Contains(text, s) {
					t.Errorf("missing %q in\n%s", s, text)
				}
			}
		})
	}
}
