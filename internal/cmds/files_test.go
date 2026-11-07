package cmds_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func searchFixture(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Search.xml"), "<Defs><ThingDef><defName>Target</defName></ThingDef></Defs>")
	e.WriteMod(filepath.Join(e.Game, "Mods", "SearchA"), testkit.Mod{PackageID: "tester.search.a", Name: "Search A", Files: map[string]string{
		"1.6/Defs/A.xml": "<Defs><ThingDef><defName>Holder</defName>\n" + strings.Repeat("<link>Target</link>\n", 6) + "</ThingDef><ThingDef><defName>Target</defName></ThingDef></Defs>",
		"1.5/Defs/A.xml": "<Defs><ThingDef><defName>OldTarget</defName></ThingDef></Defs>",
		"Defs/A.xml":     "ShadowedTarget", "Stray/Noise.xml": "Target",
		"Languages/English/Keyed/a.xml": "<LanguageData><Target.label>Target</Target.label></LanguageData>",
		"Languages/French/Keyed/a.xml":  "<LanguageData><Target.label>FrenchTarget</Target.label></LanguageData>",
		"Textures/Things/Target.png":    "binary", "Assemblies/Target.dll": "binary",
	}})
	e.WriteMod(filepath.Join(e.Game, "Mods", "SearchB"), testkit.Mod{PackageID: "tester.search.b", Name: "Search B", Files: map[string]string{
		"Defs/B.xml":    "<Defs><ThingDef ParentName=\"Target\"><defName>Child</defName></ThingDef></Defs>",
		"Patches/B.xml": "<Patch><Operation><xpath>Defs/ThingDef[defName=\"Target\"]</xpath>\n<value>Target</value></Operation></Patch>",
	}})
	e.WriteMod(filepath.Join(e.Game, "Mods", "Inactive"), testkit.Mod{PackageID: "tester.search.inactive", Name: "Inactive Search", Files: map[string]string{"Defs/Inactive.xml": "Target"}})
	e.WriteMod(filepath.Join(e.Game, "Mods", "Twin"), testkit.Mod{PackageID: "tester.search.twin", Name: "Local Twin", Files: map[string]string{"Defs/Local.xml": "Target"}})
	e.AddWorkshop("3000000999", testkit.Mod{PackageID: "tester.search.twin", Name: "Workshop Twin", Files: map[string]string{"Defs/Workshop.xml": "Target"}})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.search.a", "tester.search.b", "tester.search.twin_steam")
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func TestFilesSearchFlags(t *testing.T) {
	t.Parallel()
	e := searchFixture(t)
	out := e.MustRun("files", "search", "Target")
	for _, text := range []string{"active list, 1.6, defs,patches,about", "official Core", "load: 1.6, /", "1.6/Defs/A.xml", "(+2 more in this file)", "in ThingDef/Holder"} {
		if !strings.Contains(out, text) {
			t.Fatalf("missing %q:\n%s", text, out)
		}
	}
	for _, text := range []string{"OldTarget", "ShadowedTarget", "Stray/", "Languages/", "Inactive Search"} {
		if strings.Contains(out, text) {
			t.Fatalf("unexpected %q:\n%s", text, out)
		}
	}
	cases := []struct {
		args         []string
		want, absent string
	}{
		{[]string{"Target", "--game", "without"}, "Search A", "official Core"},
		{[]string{"Target", "--game", "only"}, "official Core", "Search A"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--languages"}, "Languages/French/Keyed/a.xml", "Inactive Search"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--lang", "eNGLISH"}, "Languages/English/Keyed/a.xml", "Languages/French"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--lang", "English", "--lang", "French", "--kind", "languages"}, "Languages/French/Keyed/a.xml", "Defs/A.xml"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--kind", "textures"}, "Things/Target.png", "binary"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--kind", "assemblies"}, "Target.dll", "binary"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--all-versions"}, "OldTarget", "Stray/Noise"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--game-version", "1.5"}, "OldTarget", "1.6/Defs/A.xml"},
		{[]string{"Target", "--mod", "pkg:tester.search.inactive"}, "- ", "Search A"},
		{[]string{"target", "--case"}, "0 matches", "<link>Target</link>"},
		{[]string{"T.rget", "--regex", "--word", "--case", "--mod", "pkg:tester.search.b"}, "3 matches", "Search A"},
		{[]string{"Target", "--mod", "pkg:tester.search.a", "--per-file", "20", "--all"}, "<defName>Target</defName>", "next:"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			got := e.MustRun(append([]string{"files", "search"}, tc.args...)...)
			if !strings.Contains(got, tc.want) || strings.Contains(got, tc.absent) {
				t.Fatalf("want %q, not %q:\n%s", tc.want, tc.absent, got)
			}
		})
	}
	if e.App.GameVersion != "" {
		t.Fatal("game version override leaked")
	}
	out = e.MustRun("files", "search", "Target", "--mod", "pkg:tester.search.b", "--mod", "pkg:tester.search.a", "--mod", "pkg:tester.search.a")
	if !strings.Contains(out, "(2 mods,") || strings.Index(out, "Search A") > strings.Index(out, "Search B") {
		t.Fatal(out)
	}
}

func TestFilesSearchCopySelection(t *testing.T) {
	t.Parallel()
	e := searchFixture(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	copies, err := st.Ix.Resolve("pkg:tester.search.twin")
	if err != nil || len(copies) != 2 {
		t.Fatal(copies, err)
	}
	active, inactive := copies[0], copies[1]
	if st.Position(active.ID) == 0 {
		active, inactive = inactive, active
	}
	out := e.MustRun("files", "search", "Target", "--mod", "pkg:tester.search.twin")
	if !strings.Contains(out, active.Name) || strings.Contains(out, inactive.Name) || !strings.Contains(out, fmt.Sprintf("%d %s", st.Position(active.ID), st.Ix.Ref(active.ID))) {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "Target", "--mod", st.Ix.Ref(inactive.ID))
	if !strings.Contains(out, "- "+st.Ix.Ref(inactive.ID)) {
		t.Fatal(out)
	}
	e.MustRun("disable", st.Ix.Ref(active.ID))
	out, code := e.Run("files", "search", "Target", "--mod", "pkg:tester.search.twin")
	if code != 3 || !strings.Contains(out, active.Name) || !strings.Contains(out, inactive.Name) {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	// Official detection uses the instance's source kind, not a packageId prefix.
	if active.Kind == source.Official {
		t.Fatal("fixture copy has the wrong source")
	}
}

func TestFilesSearchPagingModesAndSummary(t *testing.T) {
	t.Parallel()
	e := searchFixture(t)
	out := e.MustRun("files", "search", "Target", "--def", "--limit", "1", "--per-file", "1")
	for _, text := range []string{"summary:", "defined:", "Core (ThingDef/Target)", "Search A (ThingDef/Target)", "(last wins)", "children:", "Search B (ThingDef/Child)", "patched by: 1 operations in 1 mods", "--def --limit 1 --per-file 1 --page 2 Target"} {
		if !strings.Contains(out, text) {
			t.Fatalf("missing %q:\n%s", text, out)
		}
	}
	if strings.Contains(out, "  2 ") {
		t.Fatal("second mod's matching rows leaked onto page 1:", out)
	}
	out = e.MustRun("files", "search", "Target", "--mode", "files", "--mod", "pkg:tester.search.a", "--per-file", "1", "--limit", "1")
	if !strings.Contains(out, "1.6/Defs/A.xml  7") || strings.Contains(out, "<link>") {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "Target", "--mode", "files", "--mod", "pkg:tester.search.b", "--limit", "1")
	if !strings.Contains(out, "--mode files --limit 1 --page 2 Target") || !strings.Contains(out, "Defs/B.xml  1") {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "Target", "--mode", "files", "--mod", "pkg:tester.search.b", "--limit", "1", "--page", "2")
	if !strings.Contains(out, "Patches/B.xml  2") || strings.Contains(out, "Defs/B.xml") || strings.Contains(out, "next:") {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "Target", "--mode", "count", "--mod", "pkg:tester.search.b", "--per-file", "1")
	if !strings.Contains(out, "Search B  3 matches in 2 files") || strings.Contains(out, "Patches/B.xml") {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "absent-search-text")
	if !strings.Contains(out, "--languages, --all-versions, or --mod <ref>") {
		t.Fatal(out)
	}
	out = e.MustRun("files", "search", "Target", "--mod", "pkg:tester.search.b", "--limit", "0", "--per-file", "0")
	if strings.Contains(out, "next:") || !strings.Contains(out, "3 matches") {
		t.Fatal(out)
	}
}

func TestFilesSearchInvalidFlags(t *testing.T) {
	t.Parallel()
	e := searchFixture(t)
	for _, flags := range [][]string{{"--mode", "bad"}, {"--game", "bad"}, {"--kind", "bad"}, {"--limit", "-1"}, {"--page", "0"}, {"--page", "x"}, {"--per-file", "-1"}, {"--regex"}, {"--page", "9223372036854775807", "--limit", "40"}, {"--game-version", "bad"}} {
		out, code := e.Run(append([]string{"files", "search", "["}, flags...)...)
		if code != 2 || !strings.Contains(out, "error:") {
			t.Fatalf("%v: exit %d\n%s", flags, code, out)
		}
	}
}
