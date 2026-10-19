package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DotnetBin is testdata/dotnet/bin: the fixture assemblies built by testdata/dotnet/build.ps1.
func DotnetBin() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "dotnet", "bin")
}

func (e *Env) copyDLL(name, dir string) {
	e.T.Helper()
	b, err := os.ReadFile(filepath.Join(DotnetBin(), name))
	if err != nil {
		e.T.Fatal(err)
	}
	e.WriteFile(filepath.Join(dir, "Assemblies", name), string(b))
}

// HarmonyLogFixture builds the M4 environment (B1–B4, C5, C6, I1–I5): Harmony, four code
// mods with the fixture assemblies, an inactive library, a compat patch and a referrer that
// need it, and a Player.log with one cluster per class. It registers env "main", scans,
// and returns the log text.
//
//	ModA    tester.moda    bool prefix on Pawn.Tick (Harmony ID tester.moda.harmony), CompBroken.CompTick throws
//	ModB    tester.modb    postfix + transpiler on Pawn.Tick, prefix on Pawn.Kill(int)
//	ModC    tester.modc    transpiler on Pawn.Tick (no Harmony ID string: owner = packageId)
//	Manual  tester.manual  Harmony.Patch from code only
//	Lib     tester.lib     defines Lib_Gun, installed but disabled
//	Compat  tester.compat  PatchOperationAdd on Lib_Gun (fails: target-inactive)
//	Refer   tester.refer   RecipeDef Refer_Recipe uses Lib_Gun (cross-reference fails)
func (e *Env) HarmonyLogFixture() string {
	e.T.Helper()
	mods := filepath.Join(e.Game, "Mods")
	plain := func(pkg, name string, files map[string]string) Mod {
		return Mod{PackageID: pkg, Name: name, Author: "Tester", Files: files}
	}
	e.WriteMod(filepath.Join(mods, "Harmony"), plain("brrainz.harmony", "Harmony", nil))
	e.copyDLL("0Harmony.dll", filepath.Join(mods, "Harmony"))
	for _, m := range []struct{ dir, pkg, name, dll string }{
		{"ModA", "tester.moda", "Mod A", "ModA.dll"},
		{"ModB", "tester.modb", "Mod B", "ModB.dll"},
		{"ModC", "tester.modc", "Mod C", "ModC.dll"},
		{"Manual", "tester.manual", "Manual Patcher", "ModManual.dll"},
	} {
		e.WriteMod(filepath.Join(mods, m.dir), plain(m.pkg, m.name, nil))
		e.copyDLL(m.dll, filepath.Join(mods, m.dir))
	}
	e.WriteMod(filepath.Join(mods, "Lib"), plain("tester.lib", "Lib", map[string]string{
		"Defs/Lib.xml": "<Defs>\n  <ThingDef><defName>Lib_Gun</defName></ThingDef>\n</Defs>\n"}))
	e.WriteMod(filepath.Join(mods, "Compat"), plain("tester.compat", "Compat", map[string]string{
		"Patches/Compat.xml": "<Patch>\n  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/ThingDef[defName=\"Lib_Gun\"]</xpath>\n    <value><description>x</description></value>\n  </Operation>\n</Patch>\n"}))
	e.WriteMod(filepath.Join(mods, "Refer"), plain("tester.refer", "Refer", map[string]string{
		"Defs/Recipes.xml": "<Defs>\n  <RecipeDef>\n    <defName>Refer_Recipe</defName>\n    <products>\n      <Lib_Gun>1</Lib_Gun>\n    </products>\n  </RecipeDef>\n</Defs>\n"}))
	e.WriteModsConfig("1.6.4633 rev1261", "brrainz.harmony", "ludeon.rimworld", "tester.moda", "tester.modb", "tester.modc", "tester.manual", "tester.compat", "tester.refer")
	// The game wrote the list before the session that wrote the log (a current log).
	os.Chtimes(e.ModsConfig, e.Now().Add(-time.Hour), e.Now().Add(-time.Hour))
	e.AddEnv(false)
	e.MustRun("scan")
	log := FixtureLog(filepath.Join(mods, "Compat", "Patches", "Compat.xml"))
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, log)
	os.Chtimes(p, e.Now(), e.Now())
	return log
}

// FixtureLog is the Player.log of HarmonyLogFixture; compatFile is the failing patch file.
func FixtureLog(compatFile string) string {
	var sb strings.Builder
	sb.WriteString("Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n\n")
	// B1: a patch failure that the simulation explains (target-inactive).
	fmt.Fprintf(&sb, "[Compat] Patch operation Verse.PatchOperationAdd(Defs/ThingDef[defName=\"Lib_Gun\"]) failed\nfile: %s\n\n", compatFile)
	// B2: a cross-reference to a def of a disabled mod.
	sb.WriteString("Could not resolve cross-reference: No Verse.ThingDef named Lib_Gun found to give to Verse.ThingDefCountClass Verse.ThingDefCountClass\n\n")
	// B3: an exception inside the patched Pawn.Tick; three mods only patch it.
	pawnTick := "Exception ticking Pawn1: System.NullReferenceException: Object reference not set to an instance of an object\n" +
		"[Ref 1A2B3C]\n" +
		"  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.Tick_Patch1(Verse.Pawn)\n" +
		"    - PREFIX tester.moda.harmony: Boolean ModA.Pawn_Tick:Prefix(Verse.Pawn __instance)\n" +
		"    - POSTFIX tester.modb: Void ModB.Pawn_Tick_B:Postfix()\n" +
		"    - TRANSPILER tester.modb: IEnumerable`1 ModB.Pawn_Tick_B:Transpiler(IEnumerable`1 instructions)\n" +
		"    - TRANSPILER tester.modc: IEnumerable`1 ModC.Pawn_Tick_C:Rewrite(IEnumerable`1 instructions)\n" +
		"  at Verse.TickList.Tick () [0x00000] in <00000000000000000000000000000000>:0\n\n"
	sb.WriteString(pawnTick)
	// A stack whose top frame is a mod's own code: that mod is the source.
	sb.WriteString("Exception ticking Pawn2: System.ArgumentOutOfRangeException: Index was out of range.\n" +
		"[Ref 4D5E6F]\n" +
		"  at ModA.CompBroken.CompTick () [0x00000] in <00000000000000000000000000000000>:0\n" +
		"  at Verse.ThingWithComps.Tick () [0x00000] in <00000000000000000000000000000000>:0\n" +
		"  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.Tick_Patch1(Verse.Pawn)\n" +
		"    - PREFIX tester.moda.harmony: Boolean ModA.Pawn_Tick:Prefix(Verse.Pawn __instance)\n" +
		"  at Verse.TickList.Tick () [0x00000] in <00000000000000000000000000000000>:0\n\n")
	// B4: the same error 500 times in all (the game repeats ticking errors).
	for i := 0; i < 499; i++ {
		fmt.Fprintf(&sb, "Exception ticking Pawn%d: System.NullReferenceException: Object reference not set to an instance of an object\n[Ref 1A2B3C] Duplicate stacktrace, see ref for original\n\n", 100+i)
	}
	return sb.String()
}
