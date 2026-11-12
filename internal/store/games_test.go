package store_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// legacyGameSchema makes historical upgrade fixtures start before the game split.
func legacyGameSchema(t *testing.T, s *store.Store) {
	t.Helper()
	for _, q := range []string{
		`ALTER TABLE launch DROP COLUMN game_id`, `ALTER TABLE launch DROP COLUMN user_data_root`, `ALTER TABLE launch DROP COLUMN env_name`,
		`CREATE TABLE environment_legacy(
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, game_root TEXT NOT NULL,
  user_data_root TEXT NOT NULL DEFAULT '', game_version TEXT NOT NULL DEFAULT '',
  launch TEXT NOT NULL DEFAULT 'direct', created_at INTEGER NOT NULL, current_plan INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO environment_legacy SELECT e.id,e.name,g.game_root,e.user_data_root,g.game_version,e.launch,e.created_at,e.current_plan FROM environment e JOIN game g ON g.id=e.game_id`,
		`DROP TABLE environment`, `ALTER TABLE environment_legacy RENAME TO environment`, `DROP TABLE game`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"source", "instance", "scan", "identity_question", "analysis", "game_assembly"} {
		if _, err := s.DB.Exec(`ALTER TABLE ` + table + ` RENAME COLUMN game_id TO environment_id`); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetMeta("schema_version", "37"); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeGameEnvironment(t *testing.T) {
	s, path := openTemp(t)
	e := &env.Environment{ID: "old", Name: "Old game", GameRoot: filepath.Join(t.TempDir(), "game"), UserDataRoot: "synthetic-data", GameVersion: "1.6.1234", Launch: "steam", CreatedAt: time.Unix(123, 0).UTC()}
	if err := s.InsertEnv(e); err != nil {
		t.Fatal(err)
	}
	alias := *e
	alias.ID, alias.Name, alias.GameID = "legacy-alias", "Legacy alias", ""
	alias.GameRoot += string(filepath.Separator) + "."
	if err := s.InsertEnv(&alias); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentEnv(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("env:old:launch_args", "-popupwindow"); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlan(e.ID, "Keep list", e.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentPlan(e.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSource(&source.Source{ID: "local", EnvID: e.GameID, Kind: source.Local, Label: "local", Root: "synthetic-mods", Status: source.StatusOK}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES('mod','old','lineage','local','mod','live',1,2);
INSERT INTO lineage(id,environment_id,created_at) VALUES('lineage','',1);
INSERT INTO scan(id,game_id,started_at,finished_at) VALUES(1,'old',1,2);
INSERT INTO identity_question(game_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score) VALUES('old',1,'mod',1,'candidate',0.8);
INSERT INTO analysis(game_id,run_key,game_mm,version,started_at,finished_at) VALUES('old','run','1.6',1,1,2);
INSERT INTO finding_ignore(environment_id,code,key,created_at) VALUES('old','code','key',1);
INSERT INTO launch(environment_id,started_at,method,command) VALUES('old',1,'steam','unchanged');
INSERT INTO game_assembly(game_id,file,game_mm,path,size,mtime,reader) VALUES('old','Assembly-CSharp.dll','1.6','synthetic',1,1,1);
INSERT INTO game_type(assembly_id,type_name,ns,name) VALUES(1,'Verse.Synthetic','Verse','Synthetic');
INSERT INTO snapshot(id,instance_id,scan_id,snapshot_hash,package_id,decl_blob) VALUES(1,'mod',1,x'01020304','synthetic.mod','{"name":"Synthetic"}');
INSERT INTO snapshot_file(snapshot_id,rel_path,size,mtime,hash) VALUES(1,'Defs/Synthetic.xml',4,123,x'01020304');
INSERT INTO snapshot_list(snapshot_id,n,data) VALUES(1,1,x'01020304');
UPDATE instance SET current_snapshot_id=1 WHERE id='mod';
INSERT INTO lineage_data(environment_id,lineage_id,alias,note) VALUES('','lineage','Player alias','Player note');
INSERT INTO lineage_tag(environment_id,lineage_id,tag,pos) VALUES('','lineage','Player tag',0);
INSERT INTO user_rule(environment_id,kind,a_lineage,created_at) VALUES('','after','lineage',123);`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkspace(&store.Workspace{PlanID: p.ID, Entries: `[{"i":"mod"}]`, Updated: e.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	legacyGameSchema(t, s)
	before := storedRows(t, s)
	beforeKey := s.IndexKey()
	s.Close()
	u, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if u.IndexKey() == beforeKey {
		t.Fatal("migration did not invalidate existing indexes")
	}
	got, err := u.CurrentEnv()
	if err != nil || !reflect.DeepEqual(got, e) {
		t.Fatalf("environment changed: %+v, %v; want %+v", got, err, e)
	}
	if u.CurrentPlanID(e.ID) != p.ID || u.Meta("env:old:launch_args") != "-popupwindow" {
		t.Fatal("environment settings changed")
	}
	if after := storedRows(t, u); !reflect.DeepEqual(before, after) {
		t.Fatalf("rows changed:\n%v\n%v", before, after)
	}
	if u.Meta("schema_version") != fmt.Sprint(store.SchemaVersion) {
		t.Fatal("version not advanced")
	}
	var key string
	if err := u.DB.QueryRow(`SELECT game_root_key FROM game WHERE id='old'`).Scan(&key); err != nil || key == "" {
		t.Fatalf("root key %q: %v", key, err)
	}
	var aliases int
	if err := u.DB.QueryRow(`SELECT count(*) FROM game WHERE game_root_key=?`, key).Scan(&aliases); err != nil || aliases != 2 {
		t.Fatalf("legacy alias identities merged: %d %v", aliases, err)
	}
	if got, err := u.EnvByID(alias.ID); err != nil || !reflect.DeepEqual(got, &alias) {
		t.Fatalf("legacy alias changed: %+v %v", got, err)
	}
	fresh, _ := openTemp(t)
	if got, want := schemaShape(t, u), schemaShape(t, fresh); !reflect.DeepEqual(got, want) {
		t.Fatalf("schema differs:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	oldKey := u.IndexKey()
	if err := u.UpdateGame(&store.Game{ID: e.GameID, Name: e.Name, GameRoot: e.GameRoot, GameVersion: "1.7"}); err != nil {
		t.Fatal(err)
	}
	if oldKey == u.IndexKey() {
		t.Fatal("game update did not invalidate index")
	}
}

func storedRows(t *testing.T, s *store.Store) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []string{"source", "instance", "lineage", "scan", "identity_question", "analysis", "game_assembly", "game_type", "finding_ignore", "launch", "plan", "workspace", "snapshot", "snapshot_file", "snapshot_list", "lineage_data", "lineage_tag", "user_rule"} {
		colsSQL := "*"
		if table == "launch" {
			colsSQL = "id,environment_id,started_at,method,command,plan_id,fingerprint,version_n"
		}
		rows, err := s.DB.Query(`SELECT ` + colsSQL + ` FROM ` + table + ` ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			v := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range v {
				ptrs[i] = &v[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			out[table] += fmt.Sprint(v)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return out
}

// SQLite rewrites quoting and whitespace during ALTER TABLE. Compare schema
// definitions after normalizing quoting and whitespace.
func schemaShape(t *testing.T, s *store.Store) []string {
	t.Helper()
	rows, err := s.DB.Query(`SELECT type,name,sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind, name, q string
		if err := rows.Scan(&kind, &name, &q); err != nil {
			t.Fatal(err)
		}
		q = strings.ReplaceAll(q, `"`, "")
		q = strings.Join(strings.Fields(q), "")
		out = append(out, kind+":"+name+":"+q)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUpgradeGameEnvironmentRollback(t *testing.T) {
	for _, stage := range []string{"before-environment", "after-triggers"} {
		t.Run(stage, func(t *testing.T) {
			s, path := openTemp(t)
			if err := s.InsertEnv(&env.Environment{ID: "keep", Name: "Keep", GameRoot: t.TempDir(), CreatedAt: time.Unix(1, 0)}); err != nil {
				t.Fatal(err)
			}
			legacyGameSchema(t, s)
			var q string
			if stage == "before-environment" {
				q = `CREATE TABLE environment_v38(block INTEGER)`
			} else {
				q = `CREATE TRIGGER fail_upgrade BEFORE UPDATE OF value ON meta WHEN new.key='schema_version' AND new.value='38' BEGIN SELECT RAISE(ABORT,'injected failure'); END`
			}
			if _, err := s.DB.Exec(q); err != nil {
				t.Fatal(err)
			}
			before := schemaShape(t, s)
			s.Close()
			if u, err := store.Open(path); err == nil {
				u.Close()
				t.Fatal("expected injected failure")
			}
			u, err := store.OpenSession(path)
			if err != nil {
				t.Fatal(err)
			}
			defer u.Close()
			if u.Meta("schema_version") != "37" || !reflect.DeepEqual(before, schemaShape(t, u)) {
				t.Fatal("schema/version did not roll back")
			}
			var name string
			if err := u.DB.QueryRow(`SELECT name FROM environment WHERE id='keep'`).Scan(&name); err != nil || name != "Keep" {
				t.Fatalf("lost environment: %q %v", name, err)
			}
		})
	}
}

func TestDeleteEnvironmentKeepsGameUntilLast(t *testing.T) {
	s, _ := openTemp(t)
	first := &env.Environment{ID: "first", Name: "First", GameRoot: t.TempDir(), CreatedAt: time.Unix(1, 0)}
	if err := s.InsertEnv(first); err != nil {
		t.Fatal(err)
	}
	second := &env.Environment{ID: "second", GameID: first.GameID, Name: "Second", UserDataRoot: "other-data", CreatedAt: time.Unix(2, 0)}
	if err := s.InsertEnv(second); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentEnv(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("env:first:launch_args", "-popupwindow"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO source(id,game_id,kind,label,root,status) VALUES('local','first','local','local','synthetic','ok'),('shared','','extra','shared','synthetic-shared','ok');
INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES('own','first','lineage','local','mod','live',1,1),('shared','','shared-lineage','shared','mod','live',1,1);
INSERT INTO lineage(id,environment_id,created_at) VALUES('lineage','',1),('shared-lineage','',1);
INSERT INTO scan(id,game_id,started_at,finished_at) VALUES(1,'first',1,2);
INSERT INTO change_event(scan_id,instance_id,kind) VALUES(1,'shared','discovered');
INSERT INTO analysis(game_id,run_key,game_mm,version,started_at,finished_at) VALUES('first','run','1.6',1,1,2);
INSERT INTO identity_question(game_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score) VALUES('first',1,'own',1,'candidate',0.8);
INSERT INTO launch(environment_id,started_at,method,command) VALUES('first',1,'direct','one'),('second',1,'direct','two');
INSERT INTO finding_ignore(environment_id,code,key,created_at) VALUES('first','code','key',1),('second','code','key',1);
INSERT INTO game_assembly(game_id,file,game_mm,path,size,mtime,reader) VALUES('first','Assembly-CSharp.dll','1.6','synthetic',1,1,1);`); err != nil {
		t.Fatal(err)
	}
	before := storedRows(t, s)
	if err := s.DeleteEnv(first.ID); err != nil {
		t.Fatal(err)
	}
	cur, err := s.CurrentEnv()
	if err != nil || cur == nil || cur.ID != second.ID {
		t.Fatalf("current environment: %+v %v", cur, err)
	}
	after := storedRows(t, s)
	for _, table := range []string{"source", "instance", "lineage", "scan", "identity_question", "analysis", "game_assembly", "plan"} {
		if before[table] != after[table] {
			t.Fatalf("%s changed before last environment removal", table)
		}
	}
	if s.Meta("env:first:launch_args") != "" {
		t.Fatal("removed environment settings retained")
	}
	if launches, err := s.Launches(second.ID, 10); err != nil || len(launches) != 1 {
		t.Fatalf("sibling launch changed: %v %v", launches, err)
	}
	// A late failure must also roll back the environment-scoped cleanup.
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_delete BEFORE DELETE ON game BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEnv(second.ID); err == nil {
		t.Fatal("expected rollback")
	}
	if !reflect.DeepEqual(after, storedRows(t, s)) {
		t.Fatal("partial removal after failure")
	}
	if _, err := s.DB.Exec(`DROP TRIGGER fail_delete`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEnv(second.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"game", "environment", "analysis", "identity_question", "game_assembly", "launch", "finding_ignore"} {
		var n int
		if err := s.DB.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: %d rows, %v", table, n, err)
		}
	}
	var scope string
	if err := s.DB.QueryRow(`SELECT game_id FROM scan WHERE id=1`).Scan(&scope); err != nil || scope != "" {
		t.Fatalf("shared event scan: %q %v", scope, err)
	}
	mods, err := s.Mods("")
	if err != nil || len(mods) != 1 || mods[0].ID != "shared" {
		t.Fatalf("shared inventory: %v %v", mods, err)
	}
}
