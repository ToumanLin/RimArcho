package app_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// rimsortAuxDB writes a database shaped like RimSort's aux_metadata.db (SQLAlchemy models
// AuxMetadataEntry, TagsEntry and the tags_table association).
func rimsortAuxDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := []string{
		`CREATE TABLE auxiliary_metadata (path VARCHAR NOT NULL PRIMARY KEY, type VARCHAR DEFAULT 'Unknown' NOT NULL, published_file_id VARCHAR,
			acf_time_touched INTEGER DEFAULT '-1' NOT NULL, acf_time_updated INTEGER DEFAULT '-1' NOT NULL, external_time_created INTEGER DEFAULT '-1' NOT NULL,
			external_time_updated INTEGER DEFAULT '-1' NOT NULL, user_notes VARCHAR DEFAULT '' NOT NULL, color_hex VARCHAR, ignore_warnings BOOLEAN DEFAULT '0' NOT NULL,
			outdated BOOLEAN DEFAULT '0' NOT NULL, db_time_touched DATETIME)`,
		`CREATE TABLE mod_tags (id INTEGER NOT NULL PRIMARY KEY, tag VARCHAR NOT NULL UNIQUE)`,
		`CREATE TABLE tags_table (left_id INTEGER REFERENCES auxiliary_metadata(path), right_id INTEGER REFERENCES mod_tags(id))`,
	}
	for _, s := range append(schema, stmts...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// 07 §6.4: notes and tags by path, then Workshop ID; a colour becomes a coloured tag and
// ignore warnings a tag; conflicts keep RimArcho's value; repeatable; the file is not written.
func TestImportRimSortAux(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	alpha := e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	gamma := filepath.Join(e.Game, "Mods", "Gamma")
	e.WriteMod(gamma, testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("note", ref(t, e, "Gamma"), "mine")

	path := filepath.Join(e.Root, "RimSort", "instances", "Default", "aux_metadata.db")
	q := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	rimsortAuxDB(t, path,
		// Alpha by path; Beta moved (another library) so by Workshop ID; Gamma has a note
		// already; Ghost is not installed; Plain has nothing of the player's.
		`INSERT INTO auxiliary_metadata (path, published_file_id, user_notes, color_hex, ignore_warnings) VALUES
			('`+q(alpha)+`', '3000000002', 'keep this one', '#E06C75', 0),
			('D:\elsewhere\3000000003', '3000000003', '', '#ff336699', 1),
			('`+q(gamma)+`', NULL, 'theirs', 'not a colour', 0),
			('D:\elsewhere\Ghost', '3999999999', 'gone', NULL, 0),
			('D:\elsewhere\Plain', NULL, '', NULL, 0)`,
		`INSERT INTO mod_tags (id, tag) VALUES (1, 'combat'), (2, 'qol')`,
		`INSERT INTO tags_table VALUES ('`+q(alpha)+`', 1), ('`+q(alpha)+`', 2), ('D:\elsewhere\3000000003', 2)`)
	before, _ := os.ReadFile(path)

	dry, err := e.App.ImportRimSortAux(app.RimSortAuxOptions{DB: path, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Records != 4 || dry.ByPath != 2 || dry.ByPackage != 1 || len(dry.Unmapped) != 1 || dry.NoteNew != 1 || len(dry.Conflicts) != 1 ||
		dry.TagNew != 6 || dry.TagColors != 2 || dry.NotMapped["color_hex"] != 1 {
		t.Fatalf("dry run: %+v", dry)
	}
	if colors, _ := e.App.Store.TagColors(); len(colors) > 0 && colors["rimsort colour #e06c75"] != "" {
		t.Fatal("dry run wrote a tag colour")
	}

	out := e.MustRun("import", "rimsort", "--db", path)
	for _, want := range []string{"mods: 4 with data: 3 mapped (2 by path, 1 by Workshop ID), 1 not mapped", "notes: 1 new", "tags: 6 new, 0 already present, 2 tags given a colour", "conflicts: 1", "not-mapped: color_hex 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("import lacks %q:\n%s", want, out)
		}
	}
	if out := e.MustRun("mod", "ws:3000000002"); !strings.Contains(out, "note: keep this one") || !strings.Contains(out, "combat") || !strings.Contains(out, "RimSort colour #e06c75") {
		t.Fatalf("Alpha:\n%s", out)
	}
	if out := e.MustRun("mod", "ws:3000000003"); !strings.Contains(out, "qol") || !strings.Contains(out, "RimSort: ignored warnings") || !strings.Contains(out, "RimSort colour #336699") {
		t.Fatalf("Beta:\n%s", out)
	}
	if out := e.MustRun("mod", ref(t, e, "Gamma")); !strings.Contains(out, "note: mine") {
		t.Fatalf("Gamma's note was replaced:\n%s", out)
	}
	colors, _ := e.App.Store.TagColors()
	if colors["rimsort colour #e06c75"] != "#e06c75" || colors["rimsort colour #336699"] != "#336699" {
		t.Fatalf("tag colours: %v", colors)
	}
	again := e.MustRun("import", "rimsort", "--db", path)
	if !strings.Contains(again, "notes: 0 new, 0 already set") && !strings.Contains(again, "notes: 0 new, 1 already set") || !strings.Contains(again, "tags: 0 new, 6 already present, 0 tags given a colour") {
		t.Fatalf("second import is not a no-op:\n%s", again)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the RimSort database changed")
	}
	if ents, _ := filepath.Glob(filepath.Join(e.Home, "rimcrow-*")); len(ents) > 0 {
		t.Fatalf("temp copies left behind: %v", ents)
	}

	// discovery lists each instance's database
	found := app.DiscoverManagerImports(app.ManagerLocations{RimSort: []string{filepath.Join(e.Root, "RimSort")}})
	if len(found) != 1 || found[0].Kind != "rimsort-aux" || found[0].Path != path {
		t.Fatalf("discovery: %+v", found)
	}
}
