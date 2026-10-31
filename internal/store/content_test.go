package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// putSnap writes a snapshot with its file rows and derived columns, as insertSnapshot does.
func putSnap(t *testing.T, db *sql.DB, id int64, rels ...string) {
	t.Helper()
	k, l := deriveContent(rels)
	if _, err := db.Exec(`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash, kinds, langs) VALUES(?, 'i', 1, x'00', ?, ?)`, id, k, l); err != nil {
		t.Fatal(err)
	}
	for _, r := range rels {
		if _, err := db.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) VALUES(?, ?, 1, 1)`, id, r); err != nil {
			t.Fatal(err)
		}
	}
}

// oldDerived is what the queries over snapshot_file before v27 returned for a snapshot.
func oldDerived(t *testing.T, db *sql.DB, id int64) (int, []string) {
	t.Helper()
	var dll, dp, lang, png int
	if err := db.QueryRow(`SELECT coalesce(MAX(rel_path LIKE '%.dll'), 0),
		  coalesce(MAX((rel_path LIKE 'defs/%' OR rel_path LIKE '%/defs/%' OR rel_path LIKE 'patches/%' OR rel_path LIKE '%/patches/%') AND rel_path LIKE '%.xml'), 0),
		  coalesce(MAX(rel_path LIKE 'languages/%' OR rel_path LIKE '%/languages/%'), 0),
		  EXISTS(SELECT 1 FROM snapshot_file WHERE snapshot_id=?1 AND rel_path LIKE '%textures/%.png')
		FROM snapshot_file WHERE snapshot_id=?1`, id).Scan(&dll, &dp, &lang, &png); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT rel_path FROM snapshot_file WHERE snapshot_id=? AND lower(rel_path) LIKE '%languages/%'`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(rel, "/")
		for i := 0; i+2 < len(parts); i++ {
			if strings.EqualFold(parts[i], "Languages") && (i == 0 || i == 1) {
				if k := strings.ToLower(parts[i+1]); !seen[k] {
					seen[k] = true
					out = append(out, parts[i+1])
				}
				break
			}
		}
	}
	return dll*HasDLL | dp*HasDefsPatch | lang*HasLanguages | png*HasPNG, out
}

// Path sets whose flags or languages the old queries decided in a particular way.
var derivedCases = [][]string{
	{},
	{"About/About.xml.disabled"},
	{"About/About.xml", "1.6/Languages/German (Deutsch)/Keyed/a.xml", "1.5/Languages/French/DefInjected/ThingDef/b.xml", "Languages/English/Keyed/c.xml"},
	{"Languages/ChineseSimplified (简体中文)/Keyed/x.xml", "Languages/Ägyptisch/Keyed/y.xml"},
	{"Assemblies/A.dll"},
	{"1.6/Assemblies/B.DLL"},
	{"Source/obj/Debug/x.dll", "readme.dll.txt"},
	{"readme.dll.txt"},
	{"Defs/x.xml"},
	{"1.6/Defs/ThingDefs/y.XML"},
	{"Patches/p.xml"},
	{"Mods/Sub/Patches/p.xml"},
	{"Defs/readme.txt", "MyDefs/x.xml", "Patches.xml"},
	{"Languages/English"},
	{"A/B/Languages/Fr/x.xml"},
	{"Languages.txt", "LanguagesX/y/z.xml"},
	{"LANGUAGES/German/x.xml", "languages/german/y.xml", "1.6/languages/Russian/z.xml"},
	{"Textures/Things/a.png"},
	{"1.6/Textures/b.PNG"},
	{"MyTextures/c.png"},
	{"texture.png", "UI/icon.png", "Textures/readme.txt", "Textures/a.png.bak"},
	{"Textures/.png"},
	{"textures.png"},
	{"Textures/Ü/a.png", "Sounds/x.ogg", "1.6/Assemblies/Mod.dll", "1.6/Defs/a.xml", "1.6/Languages/English/Keyed/k.xml"},
}

// The columns written with a snapshot, and the ones the migration fills, say what the old
// queries over its file rows said.
func TestDeriveContentMatchesOldQueries(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, rels := range derivedCases {
		id := int64(i + 1)
		for _, r := range rels {
			if _, err := s.DB.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) VALUES(?, ?, 1, 1)`, id, r); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.DB.Exec(`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash) VALUES(?, 'i', 1, x'00')`, id); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateV27(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for i, rels := range derivedCases {
		id := int64(i + 1)
		wantK, wantL := oldDerived(t, s.DB, id)
		k, l := deriveContent(rels)
		if k != wantK || strings.Join(wantL, "/") != l {
			t.Errorf("%q: derived %d %q, old queries %d %q", rels, k, l, wantK, wantL)
		}
		var mk int
		var ml string
		if err := s.DB.QueryRow(`SELECT kinds, langs FROM snapshot WHERE id=?`, id).Scan(&mk, &ml); err != nil {
			t.Fatal(err)
		}
		if mk != k || ml != l {
			t.Errorf("%q: migrated %d %q, written %d %q", rels, mk, ml, k, l)
		}
		got, err := s.Languages(id)
		if err != nil || !reflect.DeepEqual(got, wantL) {
			t.Errorf("%q: Languages %q, %v; old %q", rels, got, err, wantL)
		}
	}
}

func TestContentKindsScopedSnapshots(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// More than one query batch, shared snapshots, no snapshot, and paths whose
	// extensions alone must not imply Defs or Languages.
	putSnap(t, s.DB, 1, "1.6/Assemblies/code.DLL", "1.6/Defs/Things/thing.xml", "1.6/Languages/English/Keyed/key.xml", "Textures/test.png")
	for i := int64(2); i <= 505; i++ {
		putSnap(t, s.DB, i, "1.6/Assemblies/code.DLL")
	}
	putSnap(t, s.DB, 999, "other.xml", "Languages.txt", "texture.png")
	snaps := map[string]int64{"zero": 0, "absent": 700, "plain": 999, "shared": 1}
	for i := 1; i <= 505; i++ {
		snaps[fmt.Sprint("m", i)] = int64(i)
	}
	got, err := s.ContentKinds(snaps)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(snaps) || got["shared"] != HasDLL|HasDefsPatch|HasLanguages || got["zero"] != 0 || got["absent"] != 0 || got["plain"] != 0 {
		t.Fatalf("unexpected flags: %v", got)
	}
	for i := 2; i <= 505; i++ {
		if got[fmt.Sprint("m", i)] != HasDLL {
			t.Fatal("lost a snapshot across batches")
		}
	}
	png, err := s.SnapshotsWithPNGFor(map[string]int64{"yes": 1, "no": 999, "none": 0, "absent": 700})
	if err != nil || len(png) != 1 || !png[1] {
		t.Fatalf("PNG flags: %v, %v", png, err)
	}
	empty, err := s.ContentKinds(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty scope: %v, %v", empty, err)
	}
	if _, err := s.DB.Exec(`INSERT INTO instance(id, environment_id, lineage_id, source_id, rel_path, real_path, status, current_snapshot_id, first_seen, last_seen)
		VALUES('a', '', 'l', 's', 'a', 'a', 'live', 1, 1, 1), ('b', '', 'l', 's', 'b', 'b', 'live', 999, 1, 1), ('c', '', 'l', 's', 'c', 'c', 'live', NULL, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	all, err := s.SnapshotsWithPNG()
	if err != nil || !reflect.DeepEqual(all, map[int64]bool{1: true}) {
		t.Fatalf("SnapshotsWithPNG: %v, %v", all, err)
	}
}

// The flags stay with a snapshot after 02 §7.1 drops its file rows.
func TestContentKindsOutliveFileRows(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	putSnap(t, s.DB, 7, "Assemblies/a.dll", "Languages/English/Keyed/k.xml")
	if _, err := s.DB.Exec(`DELETE FROM snapshot_file WHERE snapshot_id=7`); err != nil {
		t.Fatal(err)
	}
	got, err := s.ContentKinds(map[string]int64{"i": 7})
	if err != nil || got["i"] != HasDLL|HasLanguages {
		t.Fatalf("flags %v, %v", got, err)
	}
	if ls, err := s.Languages(7); err != nil || !reflect.DeepEqual(ls, []string{"English"}) {
		t.Fatalf("languages %q, %v", ls, err)
	}
}

func v26Store(t *testing.T, path string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	steps := []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24, schemaV25, schemaV26}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(strings.Join(steps, "")); err != nil {
		t.Fatal(err)
	}
	if err := migrateV26(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO meta VALUES('schema_version','26')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db, Path: path}
}

// Opening a v26 library fills the columns from the stored file rows.
func TestMigrateV27(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	old := v26Store(t, path)
	for i, rels := range derivedCases {
		id := int64(i + 1)
		if _, err := old.DB.Exec(`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash) VALUES(?, 'i', 1, x'00')`, id); err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if _, err := old.DB.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) VALUES(?, ?, 1, 1)`, id, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := map[int64][2]any{}
	snaps := map[string]int64{}
	for i := range derivedCases {
		id := int64(i + 1)
		k, l := oldDerived(t, old.DB, id)
		want[id] = [2]any{k, l}
		snaps[fmt.Sprint(id)] = id
	}
	old.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := s.Meta("schema_version"); v != fmt.Sprint(SchemaVersion) {
		t.Fatalf("schema %s", v)
	}
	kinds, err := s.ContentKinds(snaps)
	if err != nil {
		t.Fatal(err)
	}
	png, err := s.SnapshotsWithPNGFor(snaps)
	if err != nil {
		t.Fatal(err)
	}
	for id, w := range want {
		k := kinds[fmt.Sprint(id)]
		if png[id] {
			k |= HasPNG
		}
		ls, err := s.Languages(id)
		if err != nil {
			t.Fatal(err)
		}
		if k != w[0].(int) || !reflect.DeepEqual(ls, w[1].([]string)) {
			t.Errorf("snapshot %d: %d %q after migration, %d %q before", id, k, ls, w[0], w[1])
		}
	}
}
