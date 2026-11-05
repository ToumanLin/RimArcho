-- The archo.db schema at version SchemaVersion (store.go), created in one transaction for a new
-- database. A schema change edits this file and adds an upgrade step in store.go (02 §7).
-- Index-generation triggers are installed separately on each open (indexkey.go).

CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE environment(
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, game_root TEXT NOT NULL,
  user_data_root TEXT NOT NULL DEFAULT '', game_version TEXT NOT NULL DEFAULT '',
  launch TEXT NOT NULL DEFAULT 'direct', created_at INTEGER NOT NULL, current_plan INTEGER NOT NULL DEFAULT 0);
CREATE TABLE lineage(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, primary_instance_id TEXT, created_at INTEGER NOT NULL);
CREATE TABLE instance(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL,
  lineage_basis TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL, rel_path TEXT NOT NULL,
  real_path TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, residue INTEGER NOT NULL DEFAULT 0,
  current_snapshot_id INTEGER, first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL, tombstone_at INTEGER, unloadable TEXT NOT NULL DEFAULT '');
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
  file_count INTEGER NOT NULL DEFAULT 0, total_size INTEGER NOT NULL DEFAULT 0, semantic_count INTEGER NOT NULL DEFAULT 0, kinds INTEGER NOT NULL DEFAULT 0, langs TEXT NOT NULL DEFAULT '');
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
  status TEXT NOT NULL DEFAULT 'open', resolution TEXT NOT NULL DEFAULT '', resolved_at INTEGER, summary TEXT NOT NULL DEFAULT '');
CREATE TABLE user_rule(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, kind TEXT NOT NULL, a_lineage TEXT NOT NULL DEFAULT '',
  b_lineage TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
CREATE TABLE launch(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, started_at INTEGER NOT NULL, method TEXT NOT NULL, command TEXT NOT NULL, plan_id INTEGER, fingerprint TEXT NOT NULL DEFAULT '', version_n INTEGER NOT NULL DEFAULT 0);
CREATE TABLE analysis(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, run_key TEXT NOT NULL, game_mm TEXT NOT NULL,
  version INTEGER NOT NULL, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL, stats TEXT NOT NULL DEFAULT '{}', set_key TEXT NOT NULL DEFAULT '');
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
CREATE TABLE "source_use"(
  dll_hash TEXT NOT NULL, instance_id TEXT NOT NULL, file TEXT NOT NULL, status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '', decompiled_at INTEGER NOT NULL, mode TEXT NOT NULL DEFAULT 'project', PRIMARY KEY(dll_hash, instance_id));
CREATE TABLE lineage_data(
  environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL, alias TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(environment_id, lineage_id));
CREATE TABLE lineage_tag(
  environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL, tag TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY(environment_id, lineage_id, tag));
CREATE TABLE log_cache(
  path TEXT PRIMARY KEY, size INTEGER NOT NULL, mtime INTEGER NOT NULL, version TEXT NOT NULL DEFAULT '',
  input_key TEXT NOT NULL, report BLOB NOT NULL);
CREATE TABLE plan(
  id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, name TEXT NOT NULL,
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, created_game TEXT NOT NULL DEFAULT '', versions TEXT NOT NULL DEFAULT '', UNIQUE(environment_id, name));
CREATE TABLE "workspace"(
  plan_id INTEGER PRIMARY KEY, entries TEXT NOT NULL, origin_version INTEGER NOT NULL DEFAULT 0,
  origin_kind TEXT NOT NULL DEFAULT '', conflict_version INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL);
CREATE TABLE "workspace_undo"(
  id INTEGER PRIMARY KEY, plan_id INTEGER NOT NULL, stack TEXT NOT NULL, entries TEXT NOT NULL, op TEXT NOT NULL, staging TEXT);
CREATE TABLE "loadout_version"(
  id INTEGER PRIMARY KEY, plan_id INTEGER NOT NULL, n INTEGER NOT NULL, created_at INTEGER NOT NULL,
  source TEXT NOT NULL, entries TEXT NOT NULL, game_version TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  UNIQUE(plan_id, n));
CREATE INDEX workspace_undo_plan ON workspace_undo(plan_id, stack, id);
CREATE TABLE "modsconfig_state"(
  user_data_root TEXT PRIMARY KEY, written_hash BLOB, seen_hash BLOB, written_at INTEGER,
  writer_plan INTEGER, writer_version INTEGER NOT NULL DEFAULT 0, writer_env TEXT NOT NULL DEFAULT '');
CREATE INDEX launch_env ON launch(environment_id, started_at);
CREATE INDEX analysis_slot ON analysis(environment_id, game_mm, set_key, id);
CREATE TABLE instance_note(
  instance_id TEXT PRIMARY KEY, text TEXT NOT NULL DEFAULT '', made TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE id_alias(old_id TEXT PRIMARY KEY, new_id TEXT NOT NULL, kind TEXT NOT NULL);
CREATE TABLE "source"(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL,
  kind TEXT NOT NULL, label TEXT NOT NULL, root TEXT NOT NULL, status TEXT NOT NULL,
  pending_root TEXT NOT NULL DEFAULT '', UNIQUE(environment_id, label));
CREATE TABLE texture_file(
  path_key TEXT PRIMARY KEY, path TEXT NOT NULL, instance_id TEXT NOT NULL, mod_dir TEXT NOT NULL,
  rel TEXT NOT NULL, src_size INTEGER NOT NULL DEFAULT 0, src_mtime INTEGER NOT NULL DEFAULT 0,
  dds_size INTEGER NOT NULL DEFAULT 0, dds_mtime INTEGER NOT NULL DEFAULT 0,
  scale INTEGER NOT NULL DEFAULT 100, mips INTEGER NOT NULL DEFAULT 1,
  state TEXT NOT NULL, made_at INTEGER NOT NULL);
CREATE INDEX texture_file_inst ON texture_file(instance_id);
CREATE INDEX texture_file_dir ON texture_file(mod_dir);
CREATE TABLE texture_mod(
  instance_id TEXT PRIMARY KEY, snap_hash BLOB, scale INTEGER NOT NULL DEFAULT 100,
  mips INTEGER NOT NULL DEFAULT 1, files INTEGER NOT NULL DEFAULT 0, bytes INTEGER NOT NULL DEFAULT 0,
  made_at INTEGER NOT NULL);
CREATE TABLE plan_collection(
  plan_id INTEGER NOT NULL, workshop_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'add', linked_at INTEGER NOT NULL DEFAULT 0, checked_at INTEGER NOT NULL DEFAULT 0,
  snapshot TEXT NOT NULL DEFAULT '[]', seen TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY(plan_id, workshop_id));
CREATE TABLE managed_link(
 id INTEGER PRIMARY KEY, operation_id TEXT NOT NULL, phase TEXT NOT NULL,
 mods_path TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL,
 instance_id TEXT NOT NULL DEFAULT '', pack_id TEXT NOT NULL DEFAULT '',
 target TEXT NOT NULL, next_path TEXT NOT NULL, next_target TEXT NOT NULL,
 from_path TEXT NOT NULL DEFAULT '', to_path TEXT NOT NULL DEFAULT '', directory_id TEXT NOT NULL DEFAULT '',
 UNIQUE(operation_id, mods_path, name));
CREATE INDEX managed_link_phase ON managed_link(phase);
CREATE INDEX managed_link_path ON managed_link(mods_path, name, id);
CREATE TABLE credit_batch(
 id INTEGER PRIMARY KEY, created_at INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'counting' CHECK(state IN ('counting','prompt','replacing','done','dismissed')),
 sessions INTEGER NOT NULL DEFAULT 0);
CREATE TABLE credit_item(
 batch_id INTEGER NOT NULL REFERENCES credit_batch(id), workshop_id TEXT NOT NULL,
 instance_id TEXT NOT NULL, name TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending',
 target_id TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt INTEGER NOT NULL DEFAULT 0, result TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(batch_id, workshop_id));
CREATE TABLE credit_session(
 batch_id INTEGER NOT NULL REFERENCES credit_batch(id), launch_id INTEGER NOT NULL,
 environment_id TEXT NOT NULL, started_at INTEGER NOT NULL,
 PRIMARY KEY(batch_id, environment_id, launch_id, started_at));
CREATE TABLE credit_deployment(
 batch_id INTEGER NOT NULL, workshop_id TEXT NOT NULL, environment_id TEXT NOT NULL,
 plan_id INTEGER NOT NULL, done INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(batch_id, workshop_id, environment_id),
 FOREIGN KEY(batch_id, workshop_id) REFERENCES credit_item(batch_id, workshop_id));
CREATE TABLE version_mute(
  package_id TEXT NOT NULL, game_mm TEXT NOT NULL,
  PRIMARY KEY(package_id, game_mm));
CREATE INDEX identity_question_open ON identity_question(status, environment_id);
CREATE TABLE fact_set(
 id INTEGER PRIMARY KEY, instance_id TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('def','dll')),
 content_key TEXT NOT NULL, files TEXT NOT NULL DEFAULT '[]', UNIQUE(instance_id,kind,content_key));
CREATE TABLE content_state(
 instance_id TEXT NOT NULL, game_mm TEXT NOT NULL, content_key TEXT NOT NULL, folders TEXT NOT NULL DEFAULT '',
 def_files INTEGER NOT NULL DEFAULT 0, patch_files INTEGER NOT NULL DEFAULT 0, parse_errors TEXT NOT NULL DEFAULT '',
 def_set_id INTEGER NOT NULL REFERENCES fact_set(id), dll_set_id INTEGER REFERENCES fact_set(id),
 PRIMARY KEY(instance_id,game_mm));
CREATE INDEX content_state_def ON content_state(def_set_id,game_mm);
CREATE INDEX content_state_dll ON content_state(dll_set_id,game_mm);
CREATE INDEX content_state_mm ON content_state(game_mm,instance_id);
CREATE TABLE snapshot_list(snapshot_id INTEGER PRIMARY KEY, n INTEGER NOT NULL, data BLOB NOT NULL);
CREATE TABLE def_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 def_type TEXT NOT NULL, def_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
 parent TEXT NOT NULL DEFAULT '', abstract INTEGER NOT NULL DEFAULT 0, file_ix INTEGER NOT NULL, line INTEGER NOT NULL,
 may_require TEXT NOT NULL DEFAULT '', may_require_any TEXT NOT NULL DEFAULT '');
CREATE TABLE assembly_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_ix INTEGER NOT NULL, skipped INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0,
 uses_harmony INTEGER NOT NULL DEFAULT 0, types INTEGER NOT NULL DEFAULT 0, patches INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '');
CREATE TABLE type_fact(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file_ix INTEGER NOT NULL, type_name TEXT NOT NULL);
CREATE TABLE harmony_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_ix INTEGER NOT NULL, patch_type TEXT NOT NULL, patch_method TEXT NOT NULL, kind TEXT NOT NULL,
 target TEXT NOT NULL, target_type TEXT NOT NULL DEFAULT '', target_method TEXT NOT NULL DEFAULT '', args TEXT NOT NULL DEFAULT '',
 skips INTEGER NOT NULL DEFAULT 0, dynamic INTEGER NOT NULL DEFAULT 0);
CREATE TABLE harmony_id(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file_ix INTEGER NOT NULL, id TEXT NOT NULL COLLATE NOCASE);
CREATE TABLE source_library(
  dll_hash TEXT NOT NULL, game_build TEXT NOT NULL DEFAULT '', folder TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL, package_id TEXT NOT NULL DEFAULT '', dll_name TEXT NOT NULL,
  ilspy_version TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, bytes INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(dll_hash, game_build));
CREATE INDEX def_fact_set ON def_fact(set_id);
CREATE INDEX def_fact_lookup ON def_fact(def_name,set_id);
CREATE INDEX def_fact_named ON def_fact(name,set_id) WHERE name<>'';
CREATE INDEX assembly_fact_set ON assembly_fact(set_id);
CREATE INDEX type_fact_set ON type_fact(set_id);
CREATE INDEX type_fact_lookup ON type_fact(type_name,set_id);
CREATE INDEX harmony_fact_set ON harmony_fact(set_id);
CREATE INDEX harmony_id_set ON harmony_id(set_id);
CREATE INDEX harmony_id_lookup ON harmony_id(id,set_id);
-- The game's own managed assemblies per environment and the type names they define (03 §5.3).
CREATE TABLE game_assembly(
 id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, file TEXT NOT NULL, game_mm TEXT NOT NULL,
 path TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL, reader INTEGER NOT NULL,
 types INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', UNIQUE(environment_id,file));
CREATE TABLE game_type(
 assembly_id INTEGER NOT NULL REFERENCES game_assembly(id) ON DELETE CASCADE,
 type_name TEXT NOT NULL COLLATE NOCASE, ns TEXT NOT NULL, name TEXT NOT NULL);
CREATE INDEX game_type_assembly ON game_type(assembly_id);
CREATE INDEX game_type_lookup ON game_type(type_name,assembly_id);
CREATE INDEX game_type_named ON game_type(name,assembly_id);
-- The full read of each save (10 §1.7 L1): the facts as gzip JSON, plus the summary lists need.
CREATE TABLE save_cache(
 path TEXT PRIMARY KEY, size INTEGER NOT NULL, mtime INTEGER NOT NULL, parser INTEGER NOT NULL,
 ticks INTEGER NOT NULL DEFAULT 0, identity TEXT NOT NULL DEFAULT '', world TEXT NOT NULL DEFAULT '',
 faction TEXT NOT NULL DEFAULT '', malformed INTEGER NOT NULL DEFAULT 0, facts BLOB NOT NULL);
