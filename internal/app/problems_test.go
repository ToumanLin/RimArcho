package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

func problems(t *testing.T, e *testkit.Env) *app.ProblemSet {
	t.Helper()
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ps.AnalysisErr != nil {
		t.Fatal(ps.AnalysisErr)
	}
	return ps
}

func byCode(ps []*app.Problem, code string) []*app.Problem {
	var out []*app.Problem
	for _, p := range ps {
		if p.Code == code {
			out = append(out, p)
		}
	}
	return out
}

func instOf(t *testing.T, e *testkit.Env, ref string) string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

const aboutWithURL = `<?xml version="1.0" encoding="utf-8"?>
<ModMetaData>
  <packageId>tester.needy</packageId>
  <name>Needy</name>
  <supportedVersions><li>1.6</li></supportedVersions>
  <modDependencies>
    <li>
      <packageId>tester.absent</packageId>
      <displayName>Absent Library</displayName>
      <steamWorkshopUrl>https://steamcommunity.com/sharedfiles/filedetails/?id=123456</steamWorkshopUrl>
    </li>
  </modDependencies>
</ModMetaData>
`

// C1: a mod whose dependency is not installed → dep.missing with the workshop link.
func TestC1DependencyMissing(t *testing.T) {
	e := setup(t, false)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Needy", "About", "About.xml"), aboutWithURL)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.needy")
	scanNow(t, e)
	ps := problems(t, e)
	dm := byCode(ps.List, "dep.missing")
	if len(dm) != 1 {
		t.Fatalf("dep.missing = %+v", ps.List)
	}
	p := dm[0]
	if p.Severity != finding.Error || p.SubjectInstance() != instOf(t, e, "pkg:tester.needy") {
		t.Fatalf("dep.missing = %+v", p.Finding)
	}
	if !slices.Contains(p.Suggestions, "subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=123456") ||
		!slices.Contains(p.Suggestions, "steam://url/CommunityFilePage/123456") {
		t.Fatalf("suggestions = %v", p.Suggestions)
	}
	out := e.MustRun("problems", "--code", "dep")
	for _, want := range []string{"error dep.missing", "Needy requires Absent Library (pkg:tester.absent), which is not installed",
		"fix: subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=123456"} {
		if !strings.Contains(out, want) {
			t.Fatalf("problems output lacks %q:\n%s", want, out)
		}
	}
}

// C2: the workshop and the local copy of one packageId both enabled → pkg.duplicate-active.
func TestC2DuplicateActive(t *testing.T) {
	e := setup(t, false)
	m := testkit.Sample("tester.dup", "Dup")
	e.WriteMod(filepath.Join(e.Game, "Mods", "DupLocal"), m)
	e.AddWorkshop("700", m)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.dup", "tester.dup_steam")
	scanNow(t, e)
	ps := problems(t, e)
	d := byCode(ps.List, "pkg.duplicate-active")
	if len(d) != 1 || len(d[0].Subjects) != 2 || d[0].Severity != finding.Error {
		t.Fatalf("pkg.duplicate-active = %+v\nall: %+v", d, ps.List)
	}
	if len(d[0].Suggestions) != 1 || !strings.HasPrefix(d[0].Suggestions[0], "archo disable ") {
		t.Fatalf("suggestions = %v", d[0].Suggestions)
	}
}

// C3: two mods define the same ThingDef → def.override naming the one that wins.
func TestC3DefOverride(t *testing.T) {
	e := setup(t, false)
	a := testkit.Sample("tester.first", "First")
	a.Files["Defs/Shared.xml"] = "<Defs>\n  <ThingDef><defName>Shared_Gun</defName></ThingDef>\n</Defs>\n"
	b := testkit.Sample("tester.second", "Second")
	b.Files["Defs/Shared.xml"] = "<Defs>\n  <ThingDef>\n    <defName>Shared_Gun</defName>\n  </ThingDef>\n</Defs>\n"
	e.WriteMod(filepath.Join(e.Game, "Mods", "First"), a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Second"), b)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.first", "tester.second")
	scanNow(t, e)
	ps := problems(t, e)
	ov := byCode(ps.List, "def.override")
	second := instOf(t, e, "pkg:tester.second")
	if len(ov) != 1 || ov[0].Key != "ThingDef/Shared_Gun" || ov[0].SubjectInstance() != second {
		t.Fatalf("def.override = %+v", ov)
	}
	out := e.MustRun("def", "ThingDef/Shared_Gun")
	lines := strings.Split(out, "\n")
	var eff string
	for _, l := range lines {
		if strings.Contains(l, "← effective") {
			eff = l
		}
	}
	if !strings.Contains(eff, "Second") || !strings.Contains(eff, "Defs/Shared.xml:2") {
		t.Fatalf("def output:\n%s", out)
	}
}

// C4: a patch whose target mod is not enabled and which is not guarded by FindMod →
// patch.no-match with target-inactive and unguarded-optional.
func TestC4PatchTargetInactive(t *testing.T) {
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	lib.Files["Defs/Lib.xml"] = "<Defs><ThingDef><defName>Lib_Gun</defName></ThingDef></Defs>\n"
	compat := testkit.Sample("tester.compat", "Compat")
	compat.Files["Patches/Compat.xml"] = `<Patch>
  <Operation Class="PatchOperationAdd">
    <xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath>
    <value><description>patched</description></value>
  </Operation>
  <Operation Class="PatchOperationFindMod">
    <mods><li>Lib</li></mods>
    <match Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath><value><x/></value></match>
  </Operation>
</Patch>
`
	delete(compat.Files, "Patches/compat.xml") // the samples' own patches have no <value>
	delete(lib.Files, "Patches/lib.xml")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Compat"), compat)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.compat")
	scanNow(t, e)
	ps := problems(t, e)
	nm := byCode(ps.List, "patch.no-match")
	if len(nm) != 1 {
		t.Fatalf("patch.no-match = %+v", nm)
	}
	p := nm[0]
	if !strings.Contains(p.Claim, "Patches/Compat.xml:2") || !strings.Contains(p.Claim, "target-inactive, unguarded-optional") {
		t.Fatalf("claim = %q", p.Claim)
	}
	libID := instOf(t, e, "pkg:tester.lib")
	if !slices.Contains(p.Suggestions, "archo enable "+finding.Inst(libID)) {
		t.Fatalf("suggestions = %v", p.Suggestions)
	}
	out := e.MustRun("patches", "--failed")
	if !strings.Contains(out, "patch.no-match: target-inactive, unguarded-optional") || !strings.Contains(out, "is defined by #") {
		t.Fatalf("patches --failed:\n%s", out)
	}
	// Enabling the library fixes it (and makes the analysis stale first).
	e.MustRun("enable", "pkg:tester.lib", "--before", "pkg:tester.compat")
	st, _ := e.App.Loadout()
	if s, _ := e.App.Analysis(st); s.Fresh {
		t.Fatal("analysis still fresh after the Loadout changed")
	}
	if nm := byCode(problems(t, e).List, "patch.no-match"); len(nm) != 0 {
		t.Fatalf("still failing after enabling the target: %+v", nm)
	}
}

// Ignoring a problem hides it until its mod changes (03 §4.2).
func TestIgnoreExpiresWhenModChanges(t *testing.T) {
	e := setup(t, false)
	dir := filepath.Join(e.Game, "Mods", "Needy")
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), aboutWithURL)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.needy")
	scanNow(t, e)
	p := byCode(problems(t, e).List, "dep.missing")[0]
	out := e.MustRun("problems", "ignore", p.Ref)
	if !strings.Contains(out, "ignored "+p.Ref) {
		t.Fatal(out)
	}
	ps := problems(t, e)
	if len(byCode(ps.List, "dep.missing")) != 0 || len(byCode(ps.Ignored, "dep.missing")) != 1 {
		t.Fatalf("not ignored: %+v", ps.List)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "ignored: 1 (archo problems --ignored)") {
		t.Fatal(out)
	}
	// The mod updates: the ignore no longer applies.
	e.Advance(3600e9)
	e.WriteFile(filepath.Join(dir, "Defs", "New.xml"), "<Defs><ThingDef><defName>Needy_New</defName></ThingDef></Defs>\n")
	scanNow(t, e)
	ps = problems(t, e)
	dm := byCode(ps.List, "dep.missing")
	if len(dm) != 1 || !dm[0].IgnoreExpired || dm[0].Ref != p.Ref {
		t.Fatalf("after update: %+v", dm)
	}
	if out := e.MustRun("problems", "--code", "dep.missing"); !strings.Contains(out, "[ignore expired: the mod changed]") {
		t.Fatal(out)
	}
}

func TestDeclarationLayer(t *testing.T) {
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	user := testkit.Sample("tester.user", "User")
	user.Deps = []string{"tester.lib"}
	old := testkit.Sample("tester.old", "Old")
	old.Supported = []string{"1.4", "1.5"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteMod(filepath.Join(e.Game, "Mods", "User"), user)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Old"), old)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Hater", "About", "About.xml"), `<ModMetaData><packageId>tester.hater</packageId><name>Hater</name>
<supportedVersions><li>1.6</li></supportedVersions><incompatibleWith><li>tester.old</li></incompatibleWith><loadAfter><li>tester.old</li></loadAfter></ModMetaData>`)
	e.WriteModsConfig("1.6.4633 rev1261", "tester.hater", "ludeon.rimworld", "tester.user", "tester.old", "tester.ghost")
	scanNow(t, e)
	ps := problems(t, e)
	codes := map[string]int{}
	for _, p := range ps.List {
		codes[p.Code]++
	}
	for _, c := range []string{"dep.inactive", "version.unsupported", "incompat.active", "order.violated", "official.order", "entry.missing"} {
		if codes[c] == 0 {
			t.Errorf("no %s in %v", c, codes)
		}
	}
	if di := byCode(ps.List, "dep.inactive"); len(di) == 0 || !slices.Contains(di[0].Suggestions, "archo enable "+finding.Inst(instOf(t, e, "pkg:tester.lib"))) {
		t.Fatalf("dep.inactive = %+v", di)
	}
	// Errors first.
	if ps.List[0].Severity != finding.Error {
		t.Fatalf("first problem is %s", ps.List[0].Severity)
	}
	out := e.MustRun("why", "pkg:tester.hater", "pkg:tester.old")
	for _, want := range []string{"incompatibleWith", "loadAfter", "VIOLATED"} {
		if !strings.Contains(out, want) {
			t.Fatalf("why lacks %q:\n%s", want, out)
		}
	}
}

func TestFilesSearch(t *testing.T) {
	e := setup(t, false)
	m := testkit.Sample("tester.findme", "FindMe")
	m.Files["1.6/Defs/V.xml"] = "<Defs><ThingDef><defName>Needle_Current</defName></ThingDef></Defs>\n"
	m.Files["1.4/Defs/V.xml"] = "<Defs><ThingDef><defName>Needle_Old</defName></ThingDef></Defs>\n"
	e.WriteMod(filepath.Join(e.Game, "Mods", "FindMe"), m)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.findme")
	scanNow(t, e)
	out := e.MustRun("files", "search", "needle_")
	if !strings.Contains(out, "1.6/Defs/V.xml:1") || strings.Contains(out, "Needle_Old") {
		t.Fatalf("files search:\n%s", out)
	}
}
