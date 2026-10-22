package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// v13DB builds a schema v13 database: every environment with its own copy of the shared
// folders, as RimArcho left it before 02 §0.
func v13DB(t *testing.T, path string, rows ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := strings.Join([]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9,
		schemaV10, schemaV11, schemaV12, schemaV13}, "")
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', '13')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range rows {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func q1(t *testing.T, s *Store, q string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := s.DB.QueryRow(q, args...).Scan(&v); err != nil && err != sql.ErrNoRows {
		t.Fatalf("%s: %v", q, err)
	}
	return v.String
}

func col(t *testing.T, s *Store, q string, args ...any) string {
	t.Helper()
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		rows.Scan(&v)
		out = append(out, v.String)
	}
	return strings.Join(out, ",")
}

// 02 §0.1: two environments with their own copies of one Workshop folder (and the monitor
// folder) merge into one source each; instances merge by folder name, their lineages with
// them; plans keep exactly their lists and order; player data, rules, ignores, notes, origin
// markers, staging and the undo stack follow the IDs kept.
func TestMigrateV14SharedFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rimarcho.db")
	v13DB(t, path,
		`INSERT INTO environment(id, name, game_root, user_data_root, created_at, current_plan) VALUES
			('e1', 'one', 'G1', 'U', 1, 1), ('e2', 'two', 'G2', 'U', 2, 2)`,
		`INSERT INTO source(id, environment_id, kind, label, root, status) VALUES
			('s1o', 'e1', 'official', 'official', 'G1/Data', 'ok'), ('s1l', 'e1', 'local', 'local', 'G1/Mods', 'ok'),
			('s1w', 'e1', 'workshop', 'workshop', 'W', 'ok'), ('s1x', 'e1', 'extra', 'monitor', 'M', 'ok'),
			('s1k', 'e1', 'extra', 'locked', 'L/one', 'ok'),
			('s2o', 'e2', 'official', 'official', 'G2/Data', 'ok'), ('s2l', 'e2', 'local', 'local', 'G2/Mods', 'ok'),
			('s2w', 'e2', 'workshop', 'workshop', 'w', 'ok'), ('s2x', 'e2', 'extra', 'monitor', 'M', 'ok'),
			('s2k', 'e2', 'extra', 'locked', 'L/two', 'ok'), ('s2c', 'e2', 'steamcmd', 'steamcmd', 'S', 'ok')`,
		`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) VALUES
			('la1', 'e1', 'i1a', 10), ('lb1', 'e1', 'i1b', 11), ('la2', 'e2', 'i2a', 20), ('lb2', 'e2', 'i2b', 21), ('lm2', 'e2', 'i2m', 22)`,
		`INSERT INTO instance(id, environment_id, lineage_id, lineage_basis, source_id, rel_path, status, current_snapshot_id, first_seen, last_seen) VALUES
			('i1a', 'e1', 'la1', '', 's1w', '111', 'live', 1, 10, 30), ('i1b', 'e1', 'lb1', '', 's1w', '222', 'live', 2, 11, 30),
			('i1c', 'e1', 'la1', 'confirmed', 's1l', 'Copy', 'live', 3, 12, 30),
			('i2a', 'e2', 'la2', '', 's2w', '111', 'live', 4, 20, 30), ('i2b', 'e2', 'lb2', '', 's2w', '222', 'live', 5, 5, 30),
			('i2c', 'e2', 'la2', 'confirmed', 's2l', 'Copy2', 'live', 6, 21, 30), ('i2m', 'e2', 'lm2', '', 's2c', '333', 'live', 7, 22, 30)`,
		`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash, origin) VALUES
			(1, 'i1a', 1, x'01', ''), (2, 'i1b', 1, x'02', ''), (3, 'i1c', 1, x'03', '{"origin_instance":"i1a","origin_lineage":"la1"}'),
			(4, 'i2a', 2, x'01', ''), (5, 'i2b', 2, x'02', ''), (6, 'i2c', 2, x'03', '{"origin_instance":"i2a","origin_lineage":"la2","workshop_id":"111"}'),
			(7, 'i2m', 2, x'07', '')`,
		`INSERT INTO instance_alias(instance_id, source_id, rel_path, real_path) VALUES ('i2a', 's2l', '_Link_111', 'W/111')`,
		`INSERT INTO scan(id, environment_id, started_at) VALUES (1, 'e1', 100), (2, 'e2', 200)`,
		`INSERT INTO change_event(scan_id, instance_id, kind, detail) VALUES
			(1, 'i1a', 'discovered', ''), (2, 'i2a', 'discovered', ''), (2, 'i2a', 'updated', 'u2'), (2, 'i2c', 'copied', '')`,
		`INSERT INTO plan(id, environment_id, name, created_at, updated_at) VALUES (1, 'e1', 'default', 1, 1), (2, 'e2', 'default', 2, 2)`,
		`INSERT INTO workspace(plan_id, entries, updated_at) VALUES
			(1, '[{"i":"i1c"},{"i":"i1a"},{"m":"gone.mod"},{"i":"i1b"}]', 1), (2, '[{"i":"i2b"},{"i":"i2a"},{"i":"i2c"},{"i":"i2m"}]', 2)`,
		`INSERT INTO workspace_undo(id, plan_id, stack, entries, op) VALUES (1, 2, 'undo', '[{"i":"i2a"}]', 'enable'), (2, 2, 'redo', '[{"i":"i2a"},{"i":"i2b"}]', 'move')`,
		`INSERT INTO loadout_version(plan_id, n, created_at, source, entries) VALUES (2, 1, 2, 'import', '[{"i":"i2a"},{"i":"i2b"}]')`,
		`INSERT INTO lineage_data(environment_id, lineage_id, alias, note) VALUES ('e1', 'la1', 'A', 'n1'), ('e2', 'la2', 'B', 'n2'), ('e2', 'lb2', '', 'only two')`,
		`INSERT INTO lineage_tag(environment_id, lineage_id, tag) VALUES ('e1', 'la1', 'x'), ('e2', 'la2', 'X'), ('e2', 'lb2', 'y')`,
		`INSERT INTO user_rule(id, environment_id, kind, a_lineage, b_lineage, note, created_at) VALUES
			(1, 'e1', 'after', 'la1', 'lb1', '', 1), (2, 'e2', 'after', 'la2', 'lb2', 'why', 2), (3, 'e2', 'incompatible', 'la2', 'lm2', '', 3)`,
		`INSERT INTO finding_ignore(environment_id, lineage_id, code, key, created_at) VALUES ('e2', 'la2', 'mod.x', 'k', 1), ('e1', 'la1', 'mod.x', 'k', 1)`,
		`INSERT INTO instance_note(instance_id, text, made) VALUES ('i1a', 'note A', ''), ('i2a', 'note B', '{"kind":"pin"}')`,
		`INSERT INTO identity_question(environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id, score) VALUES
			('e2', 2, 'i2c', 6, 'i2a', 0.7), ('e1', 1, 'i1b', 2, 'i2b', 0.7)`,
		`INSERT INTO source_cache(dll_hash, instance_id, file, status, decompiled_at) VALUES ('h', 'i2a', 'A.dll', 'ok', 1)`,
		`INSERT INTO def_fact(instance_id, def_type, file, line) VALUES ('i2a', 'ThingDef', 'a.xml', 1), ('i1a', 'ThingDef', 'a.xml', 1)`,
		`INSERT INTO analysis(id, environment_id, run_key, game_mm, version, started_at, finished_at) VALUES (1, 'e2', 'r', '1.6', 1, 1, 1)`,
		`INSERT INTO meta(key, value) VALUES ('stage:2', '[{"i":"i2a","a":"i2b"}]'), ('current_env', 'e1')`,
	)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if got := col(t, s, `SELECT id || ':' || environment_id || ':' || label FROM source ORDER BY id`); got !=
		"s1k::locked-one,s1l:e1:local,s1o:e1:official,s1w::workshop,s1x::monitor,s2c::steamcmd,s2k::locked-two,s2l:e2:local,s2o:e2:official" {
		t.Fatalf("sources: %s", got)
	}
	if got := col(t, s, `SELECT id || ':' || environment_id || ':' || source_id || ':' || lineage_id FROM instance ORDER BY id`); got !=
		"i1a::s1w:la1,i1c:e1:s1l:la1,i2b::s1w:lb1,i2c:e2:s2l:la1,i2m::s2c:lm2" {
		t.Fatalf("instances: %s (i2a merged into the older i1a, i1b into the older i2b)", got)
	}
	if got := col(t, s, `SELECT id || ':' || environment_id || ':' || primary_instance_id FROM lineage ORDER BY id`); got != "la1::i1a,lb1::i2b,lm2::i2m" {
		t.Fatalf("lineages: %s", got)
	}
	if got := col(t, s, `SELECT old_id || '>' || new_id || ':' || kind FROM id_alias ORDER BY old_id`); got !=
		"i1b>i2b:instance,i2a>i1a:instance,la2>la1:lineage,lb2>lb1:lineage" {
		t.Fatalf("id_alias: %s", got)
	}
	// Plans: the same lists, in the same order.
	if got := q1(t, s, `SELECT entries FROM workspace WHERE plan_id=1`); got != `[{"i":"i1c"},{"i":"i1a"},{"m":"gone.mod"},{"i":"i2b"}]` {
		t.Fatalf("plan 1: %s", got)
	}
	if got := q1(t, s, `SELECT entries FROM workspace WHERE plan_id=2`); got != `[{"i":"i2b"},{"i":"i1a"},{"i":"i2c"},{"i":"i2m"}]` {
		t.Fatalf("plan 2: %s", got)
	}
	if got := col(t, s, `SELECT entries FROM workspace_undo ORDER BY id`); got != `[{"i":"i1a"}],[{"i":"i1a"},{"i":"i2b"}]` {
		t.Fatalf("undo stack: %s", got)
	}
	if got := q1(t, s, `SELECT entries FROM loadout_version WHERE plan_id=2`); got != `[{"i":"i1a"},{"i":"i2b"}]` {
		t.Fatalf("version: %s", got)
	}
	if got := s.Meta("stage:2"); got != `[{"a":"i2b","i":"i1a"}]` {
		t.Fatalf("staging: %s", got)
	}
	// Player data on lineages is global and merged.
	pd, _ := s.PlayerDataAll("e2")
	if p := pd["la1"]; p == nil || p.Alias != "A" || p.Note != "n1\n\nn2" || strings.Join(p.Tags, ",") != "x" {
		t.Fatalf("la1 data: %+v", p)
	}
	if p := pd["lb1"]; p == nil || p.Note != "only two" || strings.Join(p.Tags, ",") != "y" {
		t.Fatalf("lb1 data: %+v", p)
	}
	if got := col(t, s, `SELECT DISTINCT environment_id FROM lineage_data UNION SELECT DISTINCT environment_id FROM lineage_tag`); got != "" {
		t.Fatalf("player data environment: %q", got)
	}
	// User rules: global; the duplicate goes, its note stays on the first.
	rs, _ := s.Rules("e1")
	var rules []string
	for _, r := range rs {
		rules = append(rules, r.Kind+":"+r.A+":"+r.B+":"+r.Note)
	}
	if strings.Join(rules, ",") != "after:la1:lb1:why,incompatible:la1:lm2:" {
		t.Fatalf("rules: %v", rules)
	}
	// Ignores stay per environment.
	if got := col(t, s, `SELECT environment_id || ':' || lineage_id FROM finding_ignore ORDER BY environment_id`); got != "e1:la1,e2:la1" {
		t.Fatalf("ignores: %s", got)
	}
	if got := q1(t, s, `SELECT text || '|' || made FROM instance_note WHERE instance_id='i1a'`); got != "note A\n\nnote B|{\"kind\":\"pin\"}" {
		t.Fatalf("copy note: %q", got)
	}
	if n := q1(t, s, `SELECT count(*) FROM instance_note`); n != "1" {
		t.Fatalf("notes: %s", n)
	}
	if got := q1(t, s, `SELECT origin FROM snapshot WHERE id=6`); got != `{"origin_instance":"i1a","origin_lineage":"la1","workshop_id":"111"}` {
		t.Fatalf("origin: %s", got)
	}
	if got := col(t, s, `SELECT instance_id || ':' || hex(snapshot_hash) FROM snapshot WHERE id IN (1,4) ORDER BY id`); got != "i1a:01,i1a:01" {
		t.Fatalf("snapshots move to the instance kept: %q", got)
	}
	if got := col(t, s, `SELECT instance_id || ':' || kind FROM change_event ORDER BY id`); got != "i1a:discovered,i1a:updated,i2c:copied" {
		t.Fatalf("events: %s", got)
	}
	if got := col(t, s, `SELECT environment_id || ':' || instance_id || ':' || candidate_instance_id || ':' || status FROM identity_question ORDER BY id`); got !=
		"e2:i2c:i1a:open,:i2b:i2b:resolved" {
		t.Fatalf("questions: %s", got)
	}
	if got := col(t, s, `SELECT instance_id || ':' || source_id FROM instance_alias`); got != "i1a:s2l" {
		t.Fatalf("aliases: %s", got)
	}
	if got := col(t, s, `SELECT instance_id FROM source_cache`); got != "i1a" {
		t.Fatalf("source cache: %s", got)
	}
	if got := col(t, s, `SELECT instance_id FROM def_fact`); got != "i1a" {
		t.Fatalf("facts: %s", got)
	}
	if n := q1(t, s, `SELECT count(*) FROM analysis`); n != "0" {
		t.Fatal("analysis runs are dropped after a merge")
	}

	// What an environment sees: the shared folders and its own.
	ms, _ := s.Mods("e2")
	var seen []string
	for _, m := range ms {
		seen = append(seen, m.ID)
	}
	if strings.Join(seen, ",") != "i1a,i2b,i2c,i2m" {
		t.Fatalf("e2 sees %v", seen)
	}
	srcs, _ := s.Sources("e1")
	var labels []string
	for _, x := range srcs {
		labels = append(labels, x.Label)
	}
	if strings.Join(labels, ",") != "official,local,workshop,steamcmd,locked-one,locked-two,monitor" {
		t.Fatalf("e1 sources: %v", labels)
	}
	al, _ := s.IDAliases()
	if al["i2a"] != "i1a" || al["la2"] != "la1" {
		t.Fatalf("aliases: %v", al)
	}
}

// A single environment (the common case) loses nothing: its folders become shared, no
// instance or lineage is merged, no alias is written, caches stay.
func TestMigrateV14SingleEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rimarcho.db")
	v13DB(t, path,
		`INSERT INTO environment(id, name, game_root, created_at) VALUES ('e1', 'main', 'G', 1)`,
		`INSERT INTO source(id, environment_id, kind, label, root, status) VALUES
			('so', 'e1', 'official', 'official', 'G/Data', 'ok'), ('sw', 'e1', 'workshop', 'workshop', 'W', 'ok'),
			('sr', 'e1', 'steamcmd', 'steamcmd-removed', 'S', 'retired')`,
		`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) VALUES ('l1', 'e1', 'i1', 1), ('l2', 'e1', 'i2', 1)`,
		`INSERT INTO instance(id, environment_id, lineage_id, source_id, rel_path, status, first_seen, last_seen) VALUES
			('i1', 'e1', 'l1', 'so', 'Core', 'live', 1, 1), ('i2', 'e1', 'l2', 'sw', '111', 'live', 1, 1)`,
		`INSERT INTO analysis(id, environment_id, run_key, game_mm, version, started_at, finished_at) VALUES (1, 'e1', 'r', '1.6', 1, 1, 1)`,
		`INSERT INTO lineage_data(environment_id, lineage_id, alias) VALUES ('e1', 'l2', 'mine')`,
	)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := col(t, s, `SELECT id || ':' || environment_id FROM instance ORDER BY id`); got != "i1:e1,i2:" {
		t.Fatalf("instances: %s", got)
	}
	if got := col(t, s, `SELECT id || ':' || environment_id || ':' || label FROM source ORDER BY id`); got != "so:e1:official,sr::steamcmd-removed,sw::workshop" {
		t.Fatalf("sources: %s", got)
	}
	if n := q1(t, s, `SELECT count(*) FROM id_alias`); n != "0" {
		t.Fatal("nothing merged, no alias")
	}
	if n := q1(t, s, `SELECT count(*) FROM analysis`); n != "1" {
		t.Fatal("analysis kept when nothing merged")
	}
	if pd, _ := s.PlayerDataAll("x"); pd["l2"] == nil || pd["l2"].Alias != "mine" {
		t.Fatal("player data kept")
	}
}
