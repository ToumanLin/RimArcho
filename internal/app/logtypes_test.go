package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A missing type nothing installed defines: the mod whose XML names it is the source (a
// patch's Class="…" value, a def's <thingClass>), and the nearest types of the analysed
// assemblies are offered (03 §6.2, MCP review round 2 P5). A type an enabled mod does define
// is a version question, not a typo: the XML naming it is not searched.
func TestMissingTypeSourceAndNearest(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "Coder"), testkit.Mod{PackageID: "tester.coder", Name: "Coder", Author: "Tester"})
	b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "LogRefs.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(mods, "Coder", "Assemblies", "LogRefs.dll"), string(b))
	e.WriteMod(filepath.Join(mods, "Typo"), testkit.Mod{PackageID: "tester.typo", Name: "Typo", Author: "Tester", Files: map[string]string{
		"Patches/Stats.xml": "<Patch>\n  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/StatDef[defName=\"Beauty\"]/parts</xpath>\n    <value>\n      <li Class=\"LogRefs.Holdr\"/>\n    </value>\n  </Operation>\n</Patch>\n",
		"Defs/Things.xml":   "<Defs>\n  <ThingDef>\n    <defName>Typo_Stand</defName>\n    <thingClass>LogRefs.Inner.Holder</thingClass>\n  </ThingDef>\n</Defs>\n",
	}})
	e.WriteMod(filepath.Join(mods, "User"), testkit.Mod{PackageID: "tester.user", Name: "User", Author: "Tester", Files: map[string]string{
		"Defs/Use.xml": "<Defs>\n  <ThingDef>\n    <defName>User_Thing</defName>\n    <thingClass>LogRefs.Holder</thingClass>\n  </ThingDef>\n</Defs>\n",
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.coder", "tester.typo", "tester.user")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	rep, err := e.App.AnalyzeLogText("RimWorld 1.6.4633 rev1261\n" +
		"Exception loading def from file Stats_Basics_General.xml: System.ArgumentException: Could not find type named LogRefs.Holdr\n" +
		"Could not find a type named LogRefs.Inner.Holder\n" +
		"Could not find a type named LogRefs.Holder\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sub  string
		want map[string]string
		text []string
	}{
		{"LogRefs.Holdr", map[string]string{"tester.typo": "source"},
			[]string{"names class LogRefs.Holdr at Patches/Stats.xml:5, which no installed mod defines", "did you mean: LogRefs.Holder (#", "LogRefs.dll)"}},
		{"LogRefs.Inner.Holder", map[string]string{"tester.typo": "source"},
			[]string{"names class LogRefs.Inner.Holder at Defs/Things.xml:4", "did you mean: LogRefs.Holder ("}},
		{"named LogRefs.Holder", map[string]string{"tester.coder": "mentioned"},
			[]string{"provides type LogRefs.Holder and is enabled"}},
	} {
		roles, text := ruleCluster(t, e, rep, c.sub)
		if len(roles) != len(c.want) {
			t.Errorf("%s: roles %v, want %v\n%s", c.sub, roles, c.want, text)
		}
		for pkg, role := range c.want {
			if roles[pkg] != role {
				t.Errorf("%s: %s is %q, want %q\n%s", c.sub, pkg, roles[pkg], role, text)
			}
		}
		for _, s := range c.text {
			if !strings.Contains(text, s) {
				t.Errorf("%s: missing %q in\n%s", c.sub, s, text)
			}
		}
		if c.sub == "named LogRefs.Holder" && strings.Contains(text, "did you mean") {
			t.Errorf("%s: a defined type got a suggestion\n%s", c.sub, text)
		}
	}
}

// Two enabled mods ship a DLL with the same name and different types: the copy first in load
// order is the one the game keeps, so the later copy's types are missing in game (MCP eval
// h2). A static finding names both, the log's missing type names the earlier mod as source,
// and the mod page lists its assemblies with the same-name mark. Identical copies (a shared
// library) are no finding (03 §4.3 asm.duplicate-name).
func TestSameNameAssemblies(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	dll := func(dir, src, name string) {
		b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), src))
		if err != nil {
			t.Fatal(err)
		}
		e.WriteFile(filepath.Join(mods, dir, "Assemblies", name), string(b))
	}
	for _, d := range []string{"Stale", "Real", "LibA", "LibB"} {
		e.WriteMod(filepath.Join(mods, d), testkit.Mod{PackageID: "tester." + strings.ToLower(d), Name: d, Author: "Tester"})
	}
	dll("Stale", "LogRefs.dll", "Shared.dll")  // listed first: its copy is loaded
	dll("Real", "LogLoader.dll", "Shared.dll") // defines LogLoader.Loader, never loaded
	dll("LibA", "ModA.dll", "Lib.dll")
	dll("LibB", "ModA.dll", "Lib.dll")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.stale", "tester.real", "tester.liba", "tester.libb")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	out := e.MustRun("problems", "--code", "asm.duplicate-name", "--all")
	if !strings.Contains(out, "loads its Shared.dll first; the copy in") || !strings.Contains(out, "LogLoader.Loader") || strings.Contains(out, "Lib.dll") {
		t.Fatalf("problems:\n%s", out)
	}
	rep, err := e.App.AnalyzeLogText("RimWorld 1.6.4633 rev1261\nCould not find a type named LogLoader.Loader\n")
	if err != nil {
		t.Fatal(err)
	}
	roles, text := ruleCluster(t, e, rep, "LogLoader.Loader")
	if roles["tester.stale"] != "source" || roles["tester.real"] != "mentioned" ||
		!strings.Contains(text, "ships Assemblies/Shared.dll") || !strings.Contains(text, "without LogLoader.Loader and loads before") ||
		!strings.Contains(text, "loads first and lacks it") || strings.Contains(text, "version mismatch") {
		t.Fatalf("roles %v\n%s", roles, text)
	}
	page := e.MustRun("mod", "pkg:tester.real")
	if !strings.Contains(page, "assemblies:") || !strings.Contains(page, "Assemblies/Shared.dll") || !strings.Contains(page, "same name as") {
		t.Fatalf("mod page:\n%s", page)
	}
}
