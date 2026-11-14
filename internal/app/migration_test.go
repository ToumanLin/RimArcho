package app_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 07 §6.5, 16 §2.18: the survey counts each part; one part comes over without the others;
// the next survey shows it as already in RimArcho; a list file becomes a list once, is
// marked imported (and changed when its file changes), and a copy keeps the record.
func TestMigrationByParts(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	alpha := e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")

	home := filepath.Join(e.Root, "RimCrow")
	crow := filepath.Join(home, "data")
	os.MkdirAll(filepath.Join(crow, "rules"), 0o755)
	db := filepath.Join(crow, "mod_manager.db")
	rimcrowDB(t, db,
		`INSERT INTO modasset VALUES ('h1','tester.alpha','`+alpha+`','workshop',0)`,
		`INSERT INTO usermoddata VALUES ('tester.alpha','Alpha (main)','a note','["combat"]','#ff0000',NULL,'ia','["x"]'),
			('tester.beta',NULL,NULL,'[]',NULL,NULL,'ia','[]')`,
		`INSERT INTO modinterlock VALUES ('ia','["tester.alpha","tester.beta"]')`,
		`INSERT INTO groupdata VALUES ('g1','Weapons','#ffffff',0,1)`,
		`INSERT INTO groupmod VALUES ('g1','tester.alpha',0), ('g1','tester.beta',1)`)
	os.WriteFile(filepath.Join(crow, "rules", "user_rules.json"), []byte(`{"settings": {"user_mod_rules_enabled": false},
		"mod_rules": {"tester.beta": {"loadAfter": {"tester.gamma": {}}}}}`), 0o644)
	sortRoot := filepath.Join(e.Root, "RimSort")
	os.MkdirAll(filepath.Join(sortRoot, "modlists"), 0o755)
	list := filepath.Join(sortRoot, "modlists", "Main colony.xml")
	writeList := func(ids ...string) {
		x := "<ModsConfigData><activeMods>"
		for _, id := range ids {
			x += "<li>" + id + "</li>"
		}
		os.WriteFile(list, []byte(x+"</activeMods></ModsConfigData>"), 0o644)
	}
	writeList("ludeon.rimworld", "tester.alpha", "tester.nowhere")

	found := app.DiscoverManagerImports(app.ManagerLocations{RimSort: []string{sortRoot}, Installs: []string{home}})
	survey := func() map[string]app.MigrationPart {
		out := map[string]app.MigrationPart{}
		for _, m := range e.App.SurveyMigration(found, app.ManagerTagNames{}) {
			for _, g := range m.Groups {
				if g.Error != "" {
					t.Fatalf("%s %s: %s", m.Name, g.Kind, g.Error)
				}
				for _, p := range g.Parts {
					out[m.Name+" "+g.Kind+" "+p.Part+p.Name] = p
				}
			}
		}
		return out
	}
	first := survey()
	for key, want := range map[string]string{
		"RimCrow player aliases": "new 1", "RimCrow player notes": "new 1", "RimCrow player groups": "new 2",
		"RimCrow player tags": "new 1", "RimCrow player colours": "new 1", "RimCrow player ignored": "new 1",
		"RimCrow rules rules-off": "new 1", "RimCrow rules interlocks": "new 1", "RimSort lists Main colony": "new 1",
	} {
		p, ok := first[key]
		if got := p.State + " " + itoa(p.New); !ok || got != want {
			t.Fatalf("%s: %q, want %q (%v)", key, got, want, first)
		}
	}
	if first["RimCrow rules rules-off"].Default || !first["RimCrow player notes"].Default {
		t.Fatal("switched-off rules must not be picked by default; the rest must")
	}
	if l := first["RimSort lists Main colony"].List; l == nil || l.Mods != 3 || l.Missing != 1 {
		t.Fatalf("list facts: %+v", l)
	}

	// notes alone
	if _, err := e.App.MigratePart(app.MigrationPick{Source: "rimcrow", Path: db, Part: app.PartNotes}, app.ManagerTagNames{}); err != nil {
		t.Fatal(err)
	}
	if out := e.MustRun("mod", "ws:3000000002"); !strings.Contains(out, "note: a note") || strings.Contains(out, "Alpha (main)") || strings.Contains(out, "combat") {
		t.Fatalf("only the notes should have come over:\n%s", out)
	}
	second := survey()
	if p := second["RimCrow player notes"]; p.State != app.PartHad || p.Same != 1 {
		t.Fatalf("notes after import: %+v", p)
	}
	if second["RimCrow player aliases"].State != app.PartNew {
		t.Fatal("aliases should still be new")
	}

	// a list file becomes a list once
	out, err := e.App.MigratePart(app.MigrationPick{Source: "list", Path: list}, app.ManagerTagNames{})
	if err != nil || out.Plan == nil || out.Plan.Name != "Main colony" {
		t.Fatalf("list: %+v %v", out, err)
	}
	p := survey()["RimSort lists Main colony"]
	if p.State != app.PartListed || p.List.Plan != out.Plan.ID || p.List.Changed || p.Default {
		t.Fatalf("imported list: %+v %+v", p, p.List)
	}
	writeList("ludeon.rimworld", "tester.alpha", "tester.beta")
	if p := survey()["RimSort lists Main colony"]; !p.List.Changed {
		t.Fatal("a changed file should be marked")
	}
	cp, err := e.App.MigratePart(app.MigrationPick{Source: "list", Path: list, Mode: "copy"}, app.ManagerTagNames{})
	if err != nil || cp.Plan.Name != "Main colony 2" {
		t.Fatalf("copy: %+v %v", cp, err)
	}
	if e.App.ImportedPlan(list) != out.Plan.ID {
		t.Fatal("a copy must keep the record of the list the file became")
	}
	for _, m := range e.App.SurveyMigration(found, app.ManagerTagNames{}) {
		if m.Imported == 0 {
			t.Fatalf("%s: no import date", m.Name)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
