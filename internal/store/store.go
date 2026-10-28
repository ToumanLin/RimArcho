// Package store is RimArcho's SQLite fact store (02 §7), using modernc.org/sqlite in WAL mode
// with synchronous=NORMAL (07 §7 item 4).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// SchemaVersion is bumped with every migration.
const SchemaVersion = 24

const schemaV1 = `
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE environment(
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, game_root TEXT NOT NULL,
  user_data_root TEXT NOT NULL DEFAULT '', game_version TEXT NOT NULL DEFAULT '',
  launch TEXT NOT NULL DEFAULT 'direct', created_at INTEGER NOT NULL);
CREATE TABLE source(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL REFERENCES environment(id),
  kind TEXT NOT NULL, label TEXT NOT NULL, root TEXT NOT NULL, status TEXT NOT NULL,
  pending_root TEXT NOT NULL DEFAULT '', UNIQUE(environment_id, label));
CREATE TABLE lineage(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, primary_instance_id TEXT, created_at INTEGER NOT NULL);
CREATE TABLE instance(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL,
  lineage_basis TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL, rel_path TEXT NOT NULL,
  real_path TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, residue INTEGER NOT NULL DEFAULT 0,
  current_snapshot_id INTEGER, first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL, tombstone_at INTEGER);
CREATE INDEX instance_env ON instance(environment_id);
CREATE INDEX instance_lineage ON instance(lineage_id);
CREATE TABLE instance_alias(
  instance_id TEXT NOT NULL, source_id TEXT NOT NULL, rel_path TEXT NOT NULL, real_path TEXT NOT NULL,
  PRIMARY KEY(instance_id, source_id, rel_path));
CREATE TABLE snapshot(
  id INTEGER PRIMARY KEY, instance_id TEXT NOT NULL, scan_id INTEGER NOT NULL, snapshot_hash BLOB NOT NULL,
  package_id TEXT NOT NULL DEFAULT '', package_id_raw TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
  authors TEXT NOT NULL DEFAULT '', workshop_id_declared TEXT NOT NULL DEFAULT '',
  supported_versions TEXT NOT NULL DEFAULT '', decl_blob TEXT NOT NULL DEFAULT '{}',
  acf_manifest TEXT NOT NULL DEFAULT '', acf_time_updated INTEGER NOT NULL DEFAULT 0, acf_size INTEGER NOT NULL DEFAULT 0,
  git_head TEXT NOT NULL DEFAULT '', origin TEXT NOT NULL DEFAULT '',
  file_count INTEGER NOT NULL DEFAULT 0, total_size INTEGER NOT NULL DEFAULT 0, semantic_count INTEGER NOT NULL DEFAULT 0);
CREATE INDEX snapshot_instance ON snapshot(instance_id);
CREATE TABLE snapshot_file(
  snapshot_id INTEGER NOT NULL, rel_path TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL, hash BLOB,
  PRIMARY KEY(snapshot_id, rel_path)) WITHOUT ROWID;
CREATE TABLE scan(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, started_at INTEGER NOT NULL, finished_at INTEGER,
  full INTEGER NOT NULL DEFAULT 0, stats TEXT NOT NULL DEFAULT '{}');
CREATE TABLE change_event(
  id INTEGER PRIMARY KEY, scan_id INTEGER NOT NULL, instance_id TEXT NOT NULL, kind TEXT NOT NULL,
  before_snapshot INTEGER, after_snapshot INTEGER, detail TEXT NOT NULL DEFAULT '');
CREATE INDEX change_event_instance ON change_event(instance_id);
CREATE INDEX change_event_scan ON change_event(scan_id);
CREATE TABLE identity_question(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, scan_id INTEGER NOT NULL, instance_id TEXT NOT NULL,
  observation_snapshot_id INTEGER NOT NULL, candidate_instance_id TEXT NOT NULL, score REAL NOT NULL,
  status TEXT NOT NULL DEFAULT 'open', resolution TEXT NOT NULL DEFAULT '', resolved_at INTEGER);
`

// schemaV2 (M2): unloadable instances, the workspace Loadout with undo/redo, saved
// versions and tags, ModsConfig.xml write tracking, user rules and launches (04).
const schemaV2 = `
ALTER TABLE instance ADD COLUMN unloadable TEXT NOT NULL DEFAULT '';
CREATE TABLE workspace(
  environment_id TEXT PRIMARY KEY, entries TEXT NOT NULL, origin_version INTEGER NOT NULL DEFAULT 0,
  origin_kind TEXT NOT NULL DEFAULT '', conflict_version INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL);
CREATE TABLE workspace_undo(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, stack TEXT NOT NULL, entries TEXT NOT NULL, op TEXT NOT NULL);
CREATE INDEX workspace_undo_env ON workspace_undo(environment_id, stack, id);
CREATE TABLE loadout_version(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, n INTEGER NOT NULL, created_at INTEGER NOT NULL,
  source TEXT NOT NULL, entries TEXT NOT NULL, game_version TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  UNIQUE(environment_id, n));
CREATE TABLE loadout_tag(environment_id TEXT NOT NULL, name TEXT NOT NULL, version_n INTEGER NOT NULL,
  PRIMARY KEY(environment_id, name));
CREATE TABLE modsconfig_state(environment_id TEXT PRIMARY KEY, written_hash BLOB, seen_hash BLOB, written_at INTEGER);
CREATE TABLE user_rule(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, kind TEXT NOT NULL, a_lineage TEXT NOT NULL DEFAULT '',
  b_lineage TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
CREATE TABLE launch(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, started_at INTEGER NOT NULL, method TEXT NOT NULL, command TEXT NOT NULL);
`

// schemaV3 (M3): effective content and Def facts per instance (cached by content key),
// analysis runs with their Patch facts and Def/Patch-layer findings, and problem ignores.
const schemaV3 = `
CREATE TABLE content_state(
  instance_id TEXT PRIMARY KEY, content_key TEXT NOT NULL, folders TEXT NOT NULL DEFAULT '',
  def_files INTEGER NOT NULL DEFAULT 0, patch_files INTEGER NOT NULL DEFAULT 0, parse_errors TEXT NOT NULL DEFAULT '');
CREATE TABLE def_fact(
  instance_id TEXT NOT NULL, def_type TEXT NOT NULL, def_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
  parent TEXT NOT NULL DEFAULT '', abstract INTEGER NOT NULL DEFAULT 0, file TEXT NOT NULL, line INTEGER NOT NULL);
CREATE INDEX def_fact_instance ON def_fact(instance_id);
CREATE INDEX def_fact_name ON def_fact(def_name);
CREATE TABLE analysis(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, run_key TEXT NOT NULL, game_mm TEXT NOT NULL,
  version INTEGER NOT NULL, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL, stats TEXT NOT NULL DEFAULT '{}');
CREATE INDEX analysis_env ON analysis(environment_id, id);
CREATE TABLE patch_fact(
  analysis_id INTEGER NOT NULL, instance_id TEXT NOT NULL, seq INTEGER NOT NULL, file TEXT NOT NULL, line INTEGER NOT NULL,
  class TEXT NOT NULL, xpath TEXT NOT NULL DEFAULT '', descr TEXT NOT NULL, result TEXT NOT NULL, hits INTEGER NOT NULL,
  reasons TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '', targets TEXT NOT NULL DEFAULT '', leaf TEXT NOT NULL DEFAULT '');
CREATE INDEX patch_fact_analysis ON patch_fact(analysis_id, instance_id);
CREATE TABLE finding(
  analysis_id INTEGER NOT NULL, layer TEXT NOT NULL, code TEXT NOT NULL, severity TEXT NOT NULL, key TEXT NOT NULL,
  subject_instance TEXT NOT NULL DEFAULT '', data TEXT NOT NULL);
CREATE INDEX finding_analysis ON finding(analysis_id);
CREATE TABLE finding_ignore(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL DEFAULT '', code TEXT NOT NULL,
  key TEXT NOT NULL, snapshot_id INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
  UNIQUE(environment_id, lineage_id, code, key));
`

// schemaV4: MayRequire attributes on Def facts (the game skips such defs unless active).
const schemaV4 = `
ALTER TABLE def_fact ADD COLUMN may_require TEXT NOT NULL DEFAULT '';
ALTER TABLE def_fact ADD COLUMN may_require_any TEXT NOT NULL DEFAULT '';
DELETE FROM content_state;
`

// schemaV5 (M4): the Harmony layer per active instance (03 §2): effective assemblies,
// TypeDef names (stack frame attribution), declared patches and Harmony ID candidates.
const schemaV5 = `
CREATE TABLE assembly_fact(
  instance_id TEXT NOT NULL, file TEXT NOT NULL, skipped INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0,
  uses_harmony INTEGER NOT NULL DEFAULT 0, types INTEGER NOT NULL DEFAULT 0, patches INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '');
CREATE INDEX assembly_fact_instance ON assembly_fact(instance_id);
CREATE TABLE type_fact(instance_id TEXT NOT NULL, file TEXT NOT NULL, type_name TEXT NOT NULL);
CREATE INDEX type_fact_name ON type_fact(type_name);
CREATE INDEX type_fact_instance ON type_fact(instance_id);
CREATE TABLE harmony_fact(
  instance_id TEXT NOT NULL, file TEXT NOT NULL, patch_type TEXT NOT NULL, patch_method TEXT NOT NULL, kind TEXT NOT NULL,
  target TEXT NOT NULL, target_type TEXT NOT NULL DEFAULT '', target_method TEXT NOT NULL DEFAULT '', args TEXT NOT NULL DEFAULT '',
  skips INTEGER NOT NULL DEFAULT 0, dynamic INTEGER NOT NULL DEFAULT 0);
CREATE INDEX harmony_fact_target ON harmony_fact(target);
CREATE INDEX harmony_fact_instance ON harmony_fact(instance_id);
CREATE TABLE harmony_id(instance_id TEXT NOT NULL, file TEXT NOT NULL, id TEXT NOT NULL COLLATE NOCASE);
CREATE INDEX harmony_id_id ON harmony_id(id);
`

// schemaV6 (M4): decompile history per DLL hash (survives a deleted source cache).
const schemaV6 = `
CREATE TABLE source_cache(
  dll_hash TEXT NOT NULL, instance_id TEXT NOT NULL, file TEXT NOT NULL, status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '', decompiled_at INTEGER NOT NULL, PRIMARY KEY(dll_hash, instance_id));
`

// schemaV7 (M5): player data on lineages (04 §8.1): alias, note, tags.
const schemaV7 = `
CREATE TABLE lineage_data(
  environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL, alias TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(environment_id, lineage_id));
CREATE TABLE lineage_tag(
  environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL, tag TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY(environment_id, lineage_id, tag));
`

// schemaV8 (M5): analysis runs and the per-instance facts are kept per game major.minor,
// so an analysis for the log's version does not evict the environment's (03 §3).
// The facts are a cache: they are dropped and rebuilt on the next analysis.
const schemaV8 = `
DROP TABLE content_state;
CREATE TABLE content_state(
  instance_id TEXT NOT NULL, game_mm TEXT NOT NULL DEFAULT '', content_key TEXT NOT NULL, folders TEXT NOT NULL DEFAULT '',
  def_files INTEGER NOT NULL DEFAULT 0, patch_files INTEGER NOT NULL DEFAULT 0, parse_errors TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(instance_id, game_mm));
ALTER TABLE def_fact ADD COLUMN game_mm TEXT NOT NULL DEFAULT '';
ALTER TABLE assembly_fact ADD COLUMN game_mm TEXT NOT NULL DEFAULT '';
ALTER TABLE type_fact ADD COLUMN game_mm TEXT NOT NULL DEFAULT '';
ALTER TABLE harmony_fact ADD COLUMN game_mm TEXT NOT NULL DEFAULT '';
ALTER TABLE harmony_id ADD COLUMN game_mm TEXT NOT NULL DEFAULT '';
DELETE FROM def_fact; DELETE FROM assembly_fact; DELETE FROM type_fact; DELETE FROM harmony_fact; DELETE FROM harmony_id;
DELETE FROM patch_fact; DELETE FROM finding; DELETE FROM analysis;
CREATE INDEX def_fact_mm ON def_fact(game_mm, instance_id);
CREATE INDEX def_fact_mm_name ON def_fact(game_mm, def_name);
CREATE INDEX type_fact_mm ON type_fact(game_mm, type_name);
CREATE INDEX harmony_fact_mm ON harmony_fact(game_mm, target);
`

// schemaV9: attributed Player.log reports, so a new process (an MCP session) does not
// parse and attribute an unchanged log again. One row per log path.
const schemaV9 = `
CREATE TABLE log_cache(
  path TEXT PRIMARY KEY, size INTEGER NOT NULL, mtime INTEGER NOT NULL, version TEXT NOT NULL DEFAULT '',
  input_key TEXT NOT NULL, report BLOB NOT NULL);
`

// schemaV10: saveHarmony deletes an instance's facts by (instance_id, game_mm) for every
// instance of an analysis. Without an index on both, SQLite took type_fact_mm (all rows share
// the game version) and scanned harmony_id: on the log corpus's 3,500 instances an analysis
// spent 60% of its time in these deletes (docs/research/00, spikes/corpusrun).
const schemaV10 = `
CREATE INDEX assembly_fact_inst_mm ON assembly_fact(instance_id, game_mm);
CREATE INDEX type_fact_inst_mm ON type_fact(instance_id, game_mm);
CREATE INDEX harmony_fact_inst_mm ON harmony_fact(instance_id, game_mm);
CREATE INDEX harmony_id_inst_mm ON harmony_id(instance_id, game_mm);
`

// schemaV11: several plans per environment (04 §1). The workspace, its undo stack and the
// versions belong to a plan; versions are numbered per plan. Each environment's existing
// data becomes its first plan ("default"); each loadout_tag name becomes a plan whose
// list is that version, with one import version (04 §1.4). ModsConfig.xml write records
// are kept per user data folder with the writer (plan, version); launches record the plan,
// the list fingerprint and the version (08 §1). Key normalisation of modsconfig_state is
// finished in Go (migrateV11).
const schemaV11 = `
CREATE TABLE plan(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, name TEXT NOT NULL,
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(environment_id, name));
ALTER TABLE environment ADD COLUMN current_plan INTEGER NOT NULL DEFAULT 0;

INSERT INTO plan(environment_id, name, created_at, updated_at)
  SELECT e.id, 'default',
    coalesce((SELECT min(v.created_at) FROM loadout_version v WHERE v.environment_id=e.id), e.created_at),
    coalesce((SELECT w.updated_at FROM workspace w WHERE w.environment_id=e.id), e.created_at)
  FROM environment e
  WHERE EXISTS(SELECT 1 FROM workspace w WHERE w.environment_id=e.id)
     OR EXISTS(SELECT 1 FROM loadout_version v WHERE v.environment_id=e.id)
  ORDER BY e.created_at, e.id;
UPDATE environment SET current_plan=coalesce((SELECT p.id FROM plan p WHERE p.environment_id=environment.id), 0);

CREATE TABLE workspace_v11(
  plan_id INTEGER PRIMARY KEY, entries TEXT NOT NULL, origin_version INTEGER NOT NULL DEFAULT 0,
  origin_kind TEXT NOT NULL DEFAULT '', conflict_version INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL);
INSERT INTO workspace_v11(plan_id, entries, origin_version, origin_kind, conflict_version, updated_at)
  SELECT p.id, w.entries, w.origin_version, w.origin_kind, w.conflict_version, w.updated_at
  FROM workspace w JOIN plan p ON p.environment_id=w.environment_id;

CREATE TABLE workspace_undo_v11(
  id INTEGER PRIMARY KEY, plan_id INTEGER NOT NULL, stack TEXT NOT NULL, entries TEXT NOT NULL, op TEXT NOT NULL);
INSERT INTO workspace_undo_v11(id, plan_id, stack, entries, op)
  SELECT u.id, p.id, u.stack, u.entries, u.op FROM workspace_undo u JOIN plan p ON p.environment_id=u.environment_id;

CREATE TABLE loadout_version_v11(
  id INTEGER PRIMARY KEY, plan_id INTEGER NOT NULL, n INTEGER NOT NULL, created_at INTEGER NOT NULL,
  source TEXT NOT NULL, entries TEXT NOT NULL, game_version TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  UNIQUE(plan_id, n));
INSERT INTO loadout_version_v11(plan_id, n, created_at, source, entries, game_version, note)
  SELECT p.id, v.n, v.created_at, v.source, v.entries, v.game_version, v.note
  FROM loadout_version v JOIN plan p ON p.environment_id=v.environment_id ORDER BY v.id;

UPDATE meta SET key='stage:' || (SELECT p.id FROM plan p WHERE 'stage:' || p.environment_id = meta.key)
  WHERE key IN (SELECT 'stage:' || environment_id FROM plan);

INSERT INTO plan(environment_id, name, created_at, updated_at)
  SELECT t.environment_id, CASE WHEN t.name='default' THEN 'default (tag)' ELSE t.name END, v.created_at, v.created_at
  FROM loadout_tag t JOIN loadout_version v ON v.environment_id=t.environment_id AND v.n=t.version_n
  ORDER BY t.environment_id, t.name;
INSERT INTO loadout_version_v11(plan_id, n, created_at, source, entries, game_version, note)
  SELECT p.id, 1, v.created_at, 'import', v.entries, v.game_version, 'copied from default v' || v.n
  FROM loadout_tag t JOIN loadout_version v ON v.environment_id=t.environment_id AND v.n=t.version_n
  JOIN plan p ON p.environment_id=t.environment_id AND p.name=CASE WHEN t.name='default' THEN 'default (tag)' ELSE t.name END;
INSERT INTO workspace_v11(plan_id, entries, origin_version, origin_kind, conflict_version, updated_at)
  SELECT v.plan_id, v.entries, 1, 'import', 0, v.created_at FROM loadout_version_v11 v
  WHERE v.plan_id NOT IN (SELECT plan_id FROM workspace_v11)
    AND v.plan_id NOT IN (SELECT min(id) FROM plan GROUP BY environment_id);

DROP TABLE workspace; ALTER TABLE workspace_v11 RENAME TO workspace;
DROP TABLE workspace_undo; ALTER TABLE workspace_undo_v11 RENAME TO workspace_undo;
CREATE INDEX workspace_undo_plan ON workspace_undo(plan_id, stack, id);
DROP TABLE loadout_version; ALTER TABLE loadout_version_v11 RENAME TO loadout_version;
DROP TABLE loadout_tag;

CREATE TABLE modsconfig_state_v11(
  user_data_root TEXT PRIMARY KEY, written_hash BLOB, seen_hash BLOB, written_at INTEGER,
  writer_plan INTEGER, writer_version INTEGER NOT NULL DEFAULT 0);
INSERT OR REPLACE INTO modsconfig_state_v11(user_data_root, written_hash, seen_hash, written_at, writer_plan, writer_version)
  SELECT e.user_data_root, m.written_hash, m.seen_hash, m.written_at,
    CASE WHEN m.written_hash IS NULL THEN NULL ELSE (SELECT min(p.id) FROM plan p WHERE p.environment_id=e.id) END,
    CASE WHEN m.written_hash IS NULL THEN 0 ELSE coalesce((SELECT max(v.n) FROM loadout_version v WHERE v.plan_id=
      (SELECT min(p.id) FROM plan p WHERE p.environment_id=e.id) AND v.source IN ('user', 'undo', 'launch')), 0) END
  FROM modsconfig_state m JOIN environment e ON e.id=m.environment_id
  WHERE e.user_data_root<>''
  ORDER BY coalesce(m.written_at, 0), e.created_at;
DROP TABLE modsconfig_state; ALTER TABLE modsconfig_state_v11 RENAME TO modsconfig_state;

ALTER TABLE launch ADD COLUMN plan_id INTEGER;
ALTER TABLE launch ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE launch ADD COLUMN version_n INTEGER NOT NULL DEFAULT 0;
UPDATE launch SET plan_id=(SELECT min(p.id) FROM plan p WHERE p.environment_id=launch.environment_id)
  WHERE method NOT LIKE 'pack:%';
CREATE INDEX launch_env ON launch(environment_id, started_at);
`

// schemaV12: analysis runs are kept per enabled set (03 §3 "按方案缓存多份"): the slot is
// (environment, game_mm, set_key), so switching between plans does not evict each other's
// runs. Existing runs get an empty key and are replaced on the next analysis.
const schemaV12 = `
ALTER TABLE analysis ADD COLUMN set_key TEXT NOT NULL DEFAULT '';
CREATE INDEX analysis_slot ON analysis(environment_id, game_mm, set_key, id);
`

// schemaV13: a note per Instance (06 §2.9): the player's (or an agent's) words about this
// copy, and what RimArcho recorded when it made the copy (made: JSON, see store.Made). The
// lineage's note (lineage_data, has:note) is about the mod and all its copies.
const schemaV13 = `
CREATE TABLE instance_note(
  instance_id TEXT PRIMARY KEY, text TEXT NOT NULL DEFAULT '', made TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0);
`

// schemaV15: log attribution looks defs up by their Name="…" attribute (a missing parent,
// 03 §6.2). Without an index each lookup scanned every Def fact of the game version (~70 ms
// on 83k rows); a log with 190 such clusters took 14 s on its first query (07 §6). Partial:
// only abstract and named defs have a Name (about 5%).
const schemaV15 = `
CREATE INDEX IF NOT EXISTS def_fact_mm_named ON def_fact(game_mm, name) WHERE name<>'';
`

// migrateV11 finishes schemaV11: modsconfig_state is keyed by the normalised user data
// folder (UserDataKey); rows that normalise to the same folder keep the newest write.
func migrateV11(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT user_data_root, coalesce(written_at, 0) FROM modsconfig_state`)
	if err != nil {
		return err
	}
	type row struct {
		root string
		at   int64
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.root, &r.at); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	newest := map[string]row{}
	for _, r := range all {
		k := UserDataKey(r.root)
		if o, ok := newest[k]; !ok || r.at > o.at {
			newest[k] = r
		}
	}
	for _, r := range all {
		k := UserDataKey(r.root)
		if newest[k].root != r.root {
			if _, err := tx.Exec(`DELETE FROM modsconfig_state WHERE user_data_root=?`, r.root); err != nil {
				return err
			}
		}
	}
	for k, r := range newest {
		if k != r.root {
			if _, err := tx.Exec(`UPDATE modsconfig_state SET user_data_root=? WHERE user_data_root=?`, k, r.root); err != nil {
				return err
			}
		}
	}
	return nil
}

// Shared is the environment ID of what every environment shares (02 §0): the mod folders
// and their instances, the lineages, the player's data on lineages and the user rules.
const Shared = ""

// Store wraps the database handle.
type Store struct {
	DB   *sql.DB
	Path string
	// MM is the game major.minor whose analysis facts the fact queries read (the app sets
	// it from the Loadout it works on).
	MM string
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	return open(path, true)
}

// OpenSession connects to a database already initialised by Open. Page/worker
// sessions do not run migrations or negotiate the journal mode while a writer runs.
func OpenSession(path string) (*Store, error) {
	return open(path, false)
}

func open(path string, initialise bool) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Per-connection settings go in the DSN so every connection the pool opens gets them.
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if initialise {
		for _, p := range []string{"PRAGMA journal_mode=WAL"} {
			if _, err := db.Exec(p); err != nil {
				db.Close()
				return nil, fmt.Errorf("store: %s: %w", p, err)
			}
		}
	}
	s := &Store{DB: db, Path: path}
	if initialise {
		if err := s.migrate(); err != nil {
			db.Close()
			return nil, err
		}
	}
	if initialise {
		if err := s.installIndexTriggers(); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: index generation: %w", err)
		}
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&n); err != nil {
		return err
	}
	steps := []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24}
	// post runs after a step's SQL, in the same transaction.
	post := map[int]func(*sql.Tx) error{11: migrateV11, 14: migrateV14, 19: migrateV19, 22: migrateV22, 24: migrateV24}
	if n == 0 {
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(strings.Join(steps, "")); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: create schema: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, fmt.Sprint(SchemaVersion)); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	v, _ := strconv.Atoi(s.Meta("schema_version"))
	if v < 1 || v > SchemaVersion {
		return fmt.Errorf("store: unsupported schema version %d (want %d)", v, SchemaVersion)
	}
	for ; v < SchemaVersion; v++ {
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(steps[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migrate to v%d: %w", v+1, err)
		}
		if f := post[v+1]; f != nil {
			if err := f(tx); err != nil {
				tx.Rollback()
				return fmt.Errorf("store: migrate to v%d: %w", v+1, err)
			}
		}
		if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key='schema_version'`, fmt.Sprint(v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Meta reads a meta value ("" if absent).
func (s *Store) Meta(key string) string {
	var v string
	s.DB.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	return v
}

// SetMeta writes a meta value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ChangeKey changes whenever the database content may have changed: this connection's
// writes (total_changes) or a commit by another connection or process (data_version).
func (s *Store) ChangeKey() string {
	var dv, tc int64
	s.DB.QueryRow(`PRAGMA data_version`).Scan(&dv)
	s.DB.QueryRow(`SELECT total_changes()`).Scan(&tc)
	return fmt.Sprintf("%d.%d", dv, tc)
}
