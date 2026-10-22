package store

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// v10DB builds a schema v10 database the way an older RimArcho left it.
func v10DB(t *testing.T, path string, rows ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := strings.Join([]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10}, "")
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', '10')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range rows {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// 04 §1.4 (V10): each environment's workspace, undo stack, versions and staging become its
// first plan; each loadout_tag becomes a plan with one import version; modsconfig_state is
// keyed by user data folder with the environment's first plan as writer; launches get the plan.
func TestMigrateV10Plans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rimarcho.db")
	v10DB(t, path,
		`INSERT INTO environment(id, name, game_root, user_data_root, created_at) VALUES
			('e1', 'one', 'G1', 'C:/Users/x/RimWorld', 1), ('e2', 'two', 'G2', 'C:/Users/x/RimWorld', 2), ('e3', 'three', 'G3', '', 3)`,
		`INSERT INTO workspace(environment_id, entries, origin_version, origin_kind, conflict_version, updated_at) VALUES
			('e1', 'W1', 2, 'save', 3, 500), ('e2', 'W2', 1, 'import', 0, 600)`,
		`INSERT INTO workspace_undo(id, environment_id, stack, entries, op) VALUES
			(1, 'e1', 'undo', 'U1', 'enable'), (2, 'e2', 'undo', 'X1', 'sort'), (3, 'e1', 'undo', 'U2', 'move'), (4, 'e1', 'redo', 'R1', 'disable')`,
		`INSERT INTO loadout_version(environment_id, n, created_at, source, entries, game_version, note) VALUES
			('e1', 1, 100, 'import', 'V1', '1.6', 'ModsConfig.xml'), ('e1', 2, 200, 'user', 'V2', '1.6', ''),
			('e1', 3, 300, 'external', 'V3', '1.6', 'ModsConfig.xml changed outside RimArcho'),
			('e2', 1, 150, 'import', 'Y1', '1.6', 'ModsConfig.xml'), ('e2', 2, 250, 'user', 'Y2', '1.6', '')`,
		`INSERT INTO loadout_tag(environment_id, name, version_n) VALUES ('e1', 'raid', 2), ('e1', 'default', 1), ('e1', 'lost', 9)`,
		`INSERT INTO meta(key, value) VALUES ('stage:e1', '[{"i":"m1"}]'), ('current_env', 'e1')`,
		`INSERT INTO modsconfig_state(environment_id, written_hash, seen_hash, written_at) VALUES
			('e1', x'01', x'01', 210), ('e2', x'02', x'03', 260), ('e3', x'04', x'04', 900)`,
		`INSERT INTO launch(environment_id, started_at, method, command) VALUES ('e1', 400, 'steam', 'x'), ('e1', 450, 'pack:Big', 'y')`,
	)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := s.Meta("schema_version"); v != fmt.Sprint(SchemaVersion) {
		t.Fatalf("schema_version %s", v)
	}

	ps, _ := s.Plans("e1")
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "default,default (tag),raid" {
		t.Fatalf("e1 plans: %v (a tag of a missing version makes no plan)", names)
	}
	def, tagDef, raid := ps[0], ps[1], ps[2]
	if s.CurrentPlanID("e1") != def.ID {
		t.Fatal("the first plan is current")
	}
	if p, _ := s.Plans("e3"); len(p) != 0 || s.CurrentPlanID("e3") != 0 {
		t.Fatal("an environment without loadout data gets no plan (first use imports ModsConfig.xml)")
	}

	ws, _ := s.Workspace(def.ID)
	if ws == nil || ws.Entries != "W1" || ws.OriginVersion != 2 || ws.OriginKind != "save" || ws.Conflict != 3 {
		t.Fatalf("e1 workspace: %+v", ws)
	}
	undo, _ := s.Steps(def.ID, "undo")
	if len(undo) != 2 || undo[0].Entries != "U2" || undo[1].Op != "enable" {
		t.Fatalf("undo stack: %+v", undo)
	}
	if redo, _ := s.Steps(def.ID, "redo"); len(redo) != 1 || redo[0].Entries != "R1" {
		t.Fatalf("redo stack: %+v", redo)
	}
	vs, _ := s.Versions(def.ID)
	if len(vs) != 3 || vs[0].N != 3 || vs[0].Source != "external" || vs[2].Entries != "V1" {
		t.Fatalf("e1 versions: %+v", vs)
	}

	rv, _ := s.Versions(raid.ID)
	if len(rv) != 1 || rv[0].N != 1 || rv[0].Source != "import" || rv[0].Entries != "V2" || rv[0].Note != "copied from default v2" || rv[0].GameVersion != "1.6" {
		t.Fatalf("tag plan versions: %+v", rv[0])
	}
	if w, _ := s.Workspace(raid.ID); w == nil || w.Entries != "V2" || w.OriginVersion != 1 || w.OriginKind != "import" {
		t.Fatalf("tag plan workspace: %+v", w)
	}
	if w, _ := s.Workspace(tagDef.ID); w == nil || w.Entries != "V1" {
		t.Fatalf("tag 'default' plan workspace: %+v", w)
	}
	if n := s.StepCount(raid.ID, "undo"); n != 0 {
		t.Fatal("a tag plan starts without undo steps")
	}

	e2, _ := s.Plans("e2")
	if len(e2) != 1 {
		t.Fatalf("e2 plans: %d", len(e2))
	}
	if v, _ := s.Version(e2[0].ID, 2); v == nil || v.Entries != "Y2" {
		t.Fatal("versions are numbered per plan")
	}
	if u, _ := s.Steps(e2[0].ID, "undo"); len(u) != 1 || u[0].Entries != "X1" {
		t.Fatalf("e2 undo: %+v", u)
	}

	if s.Meta(StageKey(def.ID)) != `[{"i":"m1"}]` || s.Meta("stage:e1") != "" {
		t.Fatal("staging moves to stage:<plan ID>")
	}

	// Both environments share the folder: the newest write wins, attributed to e2's first plan.
	st, _ := s.ModsConfigStateAt("C:/Users/x/RimWorld")
	if !bytes.Equal(st.Written, []byte{2}) || !bytes.Equal(st.Seen, []byte{3}) || st.WriterPlan != e2[0].ID || st.WriterVersion != 2 || st.WrittenAt.Unix() != 260 {
		t.Fatalf("write record: %+v", st)
	}
	if old, _ := s.ModsConfigState("e1"); !bytes.Equal(old.Written, []byte{2}) {
		t.Fatal("the deprecated per-environment accessor reads the folder's record")
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM modsconfig_state`).Scan(&n)
	if n != 1 {
		t.Fatalf("modsconfig_state rows: %d (an environment without user data has none)", n)
	}

	ls, _ := s.Launches("e1", 0)
	if len(ls) != 2 || ls[0].Method != "pack:Big" || ls[0].PlanID != 0 || ls[1].PlanID != def.ID {
		t.Fatalf("launches: %+v %+v", ls[0], ls[1])
	}
	if !s.LastPlanLaunch(def.ID).Equal(ls[1].Started) {
		t.Fatal("last played")
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='loadout_tag'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("loadout_tag is dropped")
	}

	// Removing an environment removes its plans and their data.
	if err := s.DeleteEnv("e1"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`SELECT count(*) FROM plan WHERE environment_id='e1'`,
		`SELECT count(*) FROM workspace WHERE plan_id IN (` + ids(def.ID, tagDef.ID, raid.ID) + `)`,
		`SELECT count(*) FROM loadout_version WHERE plan_id IN (` + ids(def.ID, tagDef.ID, raid.ID) + `)`,
		`SELECT count(*) FROM workspace_undo WHERE plan_id IN (` + ids(def.ID, tagDef.ID, raid.ID) + `)`,
		`SELECT count(*) FROM meta WHERE key LIKE 'stage:%'`} {
		s.DB.QueryRow(q).Scan(&n)
		if n != 0 {
			t.Fatalf("%s: %d left", q, n)
		}
	}
}

func ids(xs ...int64) string {
	var out []string
	for _, x := range xs {
		out = append(out, strconv.FormatInt(x, 10))
	}
	return strings.Join(out, ",")
}

var fixed = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// A plan's write path: deleting it removes its data, and the current plan falls back.
func TestPlanStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rimarcho.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.DB.Exec(`INSERT INTO environment(id, name, game_root, user_data_root, created_at) VALUES ('e', 'e', 'g', 'u', 0)`)
	if p, _ := s.CurrentPlan("e"); p != nil {
		t.Fatal("no plan yet")
	}
	a, err := s.CreatePlan("e", "a", fixed)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreatePlan("e", "b", fixed)
	if _, err := s.CreatePlan("e", "a", fixed); err == nil {
		t.Fatal("names are unique per environment")
	}
	if p, _ := s.CurrentPlan("e"); p == nil || p.ID != a.ID {
		t.Fatal("without a current plan the first one is used")
	}
	s.SetCurrentPlan("e", b.ID)
	s.SetWorkspace(&Workspace{PlanID: b.ID, Entries: "x", Updated: fixed.Add(time.Hour)})
	s.PushStep(b.ID, "undo", "y", "enable")
	s.AddVersion(b.ID, &Version{Source: "import", Created: fixed})
	s.SetMeta(StageKey(b.ID), "[]")
	if p, _ := s.PlanByID(b.ID); !p.Updated.Equal(fixed.Add(time.Hour)) {
		t.Fatal("updated_at follows the workspace")
	}
	if p, _ := s.PlanByName("e", "B"); p == nil || p.ID != b.ID {
		t.Fatal("case-insensitive lookup")
	}
	if err := s.DeletePlan(b.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ := s.Workspace(b.ID); w != nil || s.StepCount(b.ID, "undo") != 0 || s.VersionCount(b.ID) != 0 || s.Meta(StageKey(b.ID)) != "" {
		t.Fatal("a removed plan's data is gone")
	}
	if p, _ := s.CurrentPlan("e"); p == nil || p.ID != a.ID {
		t.Fatal("the current plan falls back to the first")
	}
}
