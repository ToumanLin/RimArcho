package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 07 §7.2, §7.3: RimStack's saved list, the text its "copy the load order" gives and the
// page it puts on Rentry read as lists; the writers read back the same.
func TestRimStackListFormats(t *testing.T) {
	t.Parallel()
	saved := parse(t, `{"name":"Medieval run","ids":["ludeon.rimworld","brrainz.harmony","tester.a_steam"]}`)
	if saved.Format != "rimstack-list" || saved.Title != "Medieval run" || strings.Join(saved.Tokens, ",") != "ludeon.rimworld,brrainz.harmony,tester.a_steam" {
		t.Fatalf("saved list: %+v", saved)
	}
	// RimStack's copied text, and the same lines as RimStack publishes them on Rentry
	copied := "Created with RimStack\nRimWorld game version this list was created for: 1.6.4633 rev1261\nTotal # of mods: 3\n\n" +
		"Core [ludeon.rimworld][https://rimworldgame.com/]\n" +
		"Harmony [brrainz.harmony][https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077]\n" +
		"[KV] A [tester.a_steam][No url specified]\n"
	rentry := "Created with RimStack\nRimWorld game version this list was created for: 1.6.4633 rev1261\nTotal # of mods: 3\n\n" +
		"Core [ludeon.rimworld][https://rimworldgame.com/] {packageid: ludeon.rimworld}\n" +
		"Harmony [brrainz.harmony][https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077] {packageid: brrainz.harmony}\n" +
		"[KV] A [tester.a_steam][No url specified] {packageid: tester.a_steam}\n"
	for what, in := range map[string]string{"copied": copied, "rentry": rentry} {
		l := parse(t, in)
		if l.Format != "rimstack-text" || l.Game != "1.6.4633" || strings.Join(l.Tokens, ",") != "ludeon.rimworld,brrainz.harmony,tester.a_steam" ||
			l.Names["brrainz.harmony"] != "Harmony" || l.Names["tester.a_steam"] != "[KV] A" || l.Workshop["brrainz.harmony"] != "2009463077" {
			t.Errorf("%s: %s %s %v %v %v\n%s", what, l.Format, l.Game, l.Tokens, l.Names, l.Workshop, in)
		}
	}
	// RimSort's copied text keeps its own name, and its header's game version
	if l := parse(t, strings.Replace(copied, "Created with RimStack", "Created with RimSort 1.0.30", 1)); l.Format != "rimsort-text" || l.Game != "1.6.4633" {
		t.Errorf("RimSort text: %s %s", l.Format, l.Game)
	}

	tokens := []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}
	ws := map[string]string{"brrainz.harmony": "2009463077"}
	names := map[string]string{"brrainz.harmony": "Harmony", "tester.a": "A [old]"}
	l := parse(t, string(app.RimStackText("1.6.4633 rev1261", tokens, ws, names)))
	if l.Format != "rimsort-text" || strings.Join(l.Tokens, ",") != strings.Join(tokens, ",") || l.Workshop["brrainz.harmony"] != "2009463077" || l.Names["tester.a"] != "A (old)" || l.Game != "1.6.4633" {
		t.Errorf("RimStackText: %+v", l)
	}
	l = parse(t, string(app.RimStackList("Mine", tokens)))
	if l.Format != "rimstack-list" || l.Title != "Mine" || strings.Join(l.Tokens, ",") != strings.Join(tokens, ",") {
		t.Errorf("RimStackList: %+v", l)
	}
	if k := app.RimStackFileKind([]byte(`{"a.b":{"tags":[],"note":"x"}}`)); k != "rimstack" {
		t.Errorf("notes kind %q", k)
	}
	if k := app.RimStackFileKind([]byte(`[{"first":"a.b","then":"c.d"}]`)); k != "rimstack-rules" {
		t.Errorf("rules kind %q", k)
	}
	if k := app.RimStackFileKind(app.RimStackList("Mine", tokens)); k != "" {
		t.Errorf("a saved list is not data: %q", k)
	}
}

// 07 §6.6: RimStack's notes, tags, ignored problems, Loaders/Last categories and rules come
// over by packageId; RimArcho's own note stays; a second import changes nothing; RimStack's
// files are only read; discovery offers its data and saved lists, not its snapshots.
func TestImportRimStack(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("note", ref(t, e, "Gamma"), "mine")

	dir := filepath.Join(e.Root, "LocalAppData", "RimStack")
	testkit.RimStackData(t, dir, map[string]testkit.RimStackNote{
		"tester.alpha": {Tags: []string{"combat", "QoL"}, Note: "keep this one", Ignored: []string{"missing:tester.ghost"}},
		"tester.beta":  {Tags: []string{}, VersionOK: true, Category: "loaders", Sub: "Weapons"},
		"tester.gamma": {Tags: []string{"qol"}, Note: "theirs", Category: "last"},
		"tester.ghost": {Tags: []string{"gone"}, Note: "not here"},
		"tester.other": {Tags: []string{}, Category: "interface"},
	}, [][2]string{{"tester.alpha", "tester.beta"}, {"tester.gamma", "tester.ghost"}, {"Tester.Alpha", "tester.beta"}},
		map[string][]string{"Medieval run": {"ludeon.rimworld", "tester.alpha", "tester.nowhere"}},
		map[string][]string{"@sorting 2026-11-20 10.00.00": {"ludeon.rimworld"}})
	notes, rules := filepath.Join(dir, "notes.json"), filepath.Join(dir, "rules.json")
	before, _ := os.ReadFile(notes)

	dry, err := e.App.ImportRimStack(app.RimStackOptions{Notes: notes, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	// ghost is not installed; other has only a category RimArcho has no equivalent for
	if dry.Records != 4 || dry.ByPackage != 3 || len(dry.Unmapped) != 1 || dry.NoteNew != 1 || len(dry.Conflicts) != 1 ||
		dry.TagNew != 6 || dry.RuleNew != 2 || dry.NotMapped["RimStack categories other than Loaders and Last"] != 1 {
		t.Fatalf("dry run: %+v", dry)
	}
	rdry, err := e.App.ImportRimStackRules(rules, true)
	if err != nil {
		t.Fatal(err)
	}
	if rdry.RulesTotal != 3 || rdry.RuleNew != 1 || rdry.RuleSame != 1 || len(rdry.RulesSkipped) != 1 {
		t.Fatalf("rules dry run: %+v", rdry)
	}
	if out := e.MustRun("rule", "list"); strings.Contains(out, "RimStack") {
		t.Fatalf("a dry run wrote rules:\n%s", out)
	}

	out := e.MustRun("import", "rimstack", "--dir", dir)
	for _, want := range []string{"mods: 4 with data: 3 mapped by packageId, 1 not mapped", "notes: 1 new", "tags: 6 new, 0 already present",
		"categories: 2 new top/bottom rules", "rules: 3: 1 new before rules, 1 already present, 1 skipped", "conflicts (1)", "tester.ghost  not installed here"} {
		if !strings.Contains(out, want) {
			t.Fatalf("import lacks %q:\n%s", want, out)
		}
	}
	if out := e.MustRun("mod", "ws:3000000002"); !strings.Contains(out, "note: keep this one") || !strings.Contains(out, "combat") || !strings.Contains(out, "RimStack: ignored warnings") {
		t.Fatalf("Alpha:\n%s", out)
	}
	if out := e.MustRun("mod", "ws:3000000003"); !strings.Contains(out, "Weapons") || !strings.Contains(out, "RimStack: ignored warnings") {
		t.Fatalf("Beta:\n%s", out)
	}
	if out := e.MustRun("mod", ref(t, e, "Gamma")); !strings.Contains(out, "note: mine") {
		t.Fatalf("Gamma's note was replaced:\n%s", out)
	}
	rl := e.MustRun("rule", "list")
	for _, want := range []string{"top", "bottom", "before", "imported from RimStack"} {
		if !strings.Contains(rl, want) {
			t.Fatalf("rules lack %q:\n%s", want, rl)
		}
	}
	again := e.MustRun("import", "rimstack", "--dir", dir)
	for _, want := range []string{"notes: 0 new", "tags: 0 new, 6 already present", "categories: 0 new top/bottom rules, 2 already present", "rules: 3: 0 new before rules, 2 already present"} {
		if !strings.Contains(again, want) {
			t.Fatalf("second import is not a no-op (%q):\n%s", want, again)
		}
	}
	if after, _ := os.ReadFile(notes); string(after) != string(before) {
		t.Fatal("RimStack's notes changed")
	}

	// discovery: notes, rules and the saved list; the snapshot is not offered
	found := app.DiscoverManagerImports(app.ManagerLocations{RimStack: []string{dir}})
	kinds := map[string]string{}
	for _, f := range found {
		if f.Manager != "RimStack" {
			t.Fatalf("manager %q", f.Manager)
		}
		kinds[f.Kind+" "+filepath.Base(f.Path)] = f.Path
	}
	if len(found) != 3 || kinds["rimstack notes.json"] == "" || kinds["rimstack-rules rules.json"] == "" || kinds["list Medieval run.json"] == "" {
		t.Fatalf("discovery: %+v", found)
	}
	// the survey: everything is in RimArcho now, the list is new and missing one Mod
	parts := map[string]app.MigrationPart{}
	for _, m := range e.App.SurveyMigration(found, app.ManagerTagNames{}) {
		for _, g := range m.Groups {
			if g.Error != "" {
				t.Fatalf("%s: %s", g.Kind, g.Error)
			}
			for _, p := range g.Parts {
				parts[g.Kind+" "+p.Part+p.Name] = p
			}
		}
	}
	for key, want := range map[string]string{"player notes": "had", "player tags": "had", "player ignored": "had", "player categories": "had", "rules rules": "had", "lists Medieval run": "new"} {
		if p := parts[key]; p.State != want {
			t.Errorf("%s: %q, want %q (%+v)", key, p.State, want, parts)
		}
	}
	if l := parts["lists Medieval run"].List; l == nil || l.Mods != 3 || l.Missing != 1 {
		t.Errorf("list facts: %+v", l)
	}
	o, err := e.App.MigratePart(app.MigrationPick{Source: "list", Path: kinds["list Medieval run.json"]}, app.ManagerTagNames{})
	if err != nil || o.Plan == nil || o.Plan.Name != "Medieval run" {
		t.Fatalf("list part: %+v %v", o, err)
	}
	// a chosen data folder reads the same
	if got := app.ManagerSourcesIn(dir); len(got) != 3 {
		t.Errorf("a chosen RimStack folder: %+v", got)
	}
}

// 07 §6.6: a part brought over alone imports only that part.
func TestImportRimStackParts(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	dir := filepath.Join(e.Root, "RimStack")
	testkit.RimStackData(t, dir, map[string]testkit.RimStackNote{
		"tester.alpha": {Tags: []string{"combat"}, Note: "n", Category: "loaders"},
	}, nil, nil, nil)
	o, err := e.App.MigratePart(app.MigrationPick{Source: "rimstack", Path: filepath.Join(dir, "notes.json"), Part: app.PartTags}, app.ManagerTagNames{})
	if err != nil || o.Count.New != 1 {
		t.Fatalf("tags part: %+v %v", o, err)
	}
	out := e.MustRun("mod", "ws:3000000002")
	if !strings.Contains(out, "combat") || strings.Contains(out, "note: n") {
		t.Fatalf("only the tags should have come:\n%s", out)
	}
	if rl := e.MustRun("rule", "list"); strings.Contains(rl, "top") {
		t.Fatalf("a top rule came with the tags:\n%s", rl)
	}
	v, err := e.App.MigrationPartView(app.MigrationPick{Source: "rimstack", Path: filepath.Join(dir, "notes.json"), Part: app.PartCategories}, app.ManagerTagNames{})
	if err != nil || len(v.Items) != 1 || v.Items[0].Value != "top" || v.Items[0].Mod != "Alpha" {
		t.Fatalf("categories view: %+v %v", v, err)
	}
}
