package store_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type legacyAnalysis struct {
	name, game, mm, slot, key, problemRef, envID string
	id                                           int64
	finding                                      *finding.Finding
	ignore                                       *store.Ignore
}

func analysisAt44(t *testing.T) (*testkit.Env, []legacyAnalysis) {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(false)
	secondGame := e.OtherGame("Second game", "1.7.4985 rev1269")
	e.MustRun("env", "add", "second", "--game", secondGame, "--user-data", filepath.Join(e.Root, "second-data"))
	for _, name := range []string{"main", "second"} {
		e.MustRun("env", "use", name)
		e.MustRun("scan", "--game-only")
	}

	s := e.App.Store
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DROP TABLE analysis_stage_content; ALTER TABLE analysis DROP COLUMN publication;
		UPDATE meta SET value='44' WHERE key='schema_version'`)
	var runs []legacyAnalysis
	for i, name := range []string{"main", "second"} {
		e.App.EnvName = name
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		core, err := st.Ix.ResolveOne("pkg:ludeon.rimworld")
		if err != nil {
			t.Fatal(err)
		}
		run := legacyAnalysis{
			name: name, game: st.Ix.Env.GameID, mm: st.GameMM(), envID: st.Ix.Env.ID,
			slot: app.SetFingerprint(st.List), key: e.App.AnalysisKey(st, ""), id: int64(41 + i),
			finding: &finding.Finding{Layer: "def", Code: "def.synthetic-upgrade", Key: name,
				Claim: "Stored finding for " + name, Severity: finding.Warning, Confidence: finding.Certain,
				Subjects: []string{finding.Inst(core.ID)}},
		}
		run.problemRef = app.ProblemRef(run.finding, core.LineageID)
		run.ignore = &store.Ignore{ID: int64(51 + i), LineageID: core.LineageID, SnapshotID: core.SnapID,
			Code: run.finding.Code, Key: run.finding.Key, CreatedAt: e.Now()}
		data, err := json.Marshal(run.finding)
		if err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO analysis(id,game_id,run_key,game_mm,version,started_at,finished_at,set_key)
			VALUES(?,?,?,?,?,?,?,?)`, run.id, run.game, run.key, run.mm, analyze.Version, e.Now().Unix(), e.Now().Unix(), run.slot)
		exec(`INSERT INTO finding(analysis_id,layer,code,severity,key,subject_instance,data) VALUES(?,?,?,?,?,?,?)`,
			run.id, run.finding.Layer, run.finding.Code, run.finding.Severity, run.finding.Key, core.ID, string(data))
		exec(`INSERT INTO patch_fact(analysis_id,instance_id,seq,file,line,class,descr,result,hits,reasons,targets)
			VALUES(?,?,1,?,9,'PatchOperationAdd','Stored patch','ok',1,'synthetic','ThingDef/Item')`,
			run.id, core.ID, name+".xml")
		exec(`INSERT INTO finding_ignore(id,environment_id,lineage_id,code,key,snapshot_id,created_at) VALUES(?,?,?,?,?,?,?)`,
			run.ignore.ID, run.envID, run.ignore.LineageID, run.ignore.Code, run.ignore.Key, run.ignore.SnapshotID, run.ignore.CreatedAt.Unix())
		runs = append(runs, run)
	}
	if runs[0].mm != "1.6" || runs[1].mm != "1.7" || runs[0].game == runs[1].game {
		t.Fatalf("expected two installations with different versions: %+v", runs)
	}
	return e, runs
}

func assertLegacyAnalysis(t *testing.T, e *testkit.Env, run legacyAnalysis) {
	t.Helper()
	e.App.EnvName = run.name
	s := e.App.Store
	s.MM = run.mm
	latest, err := s.LatestAnalysis(run.game, run.slot)
	if err != nil || latest == nil || latest.ID != run.id || latest.RunKey != run.key {
		t.Fatalf("%s latest analysis = %+v, %v", run.name, latest, err)
	}
	newest, err := s.NewestAnalysis(run.game)
	if err != nil || !reflect.DeepEqual(newest, latest) {
		t.Fatalf("%s newest analysis = %+v, %v", run.name, newest, err)
	}
	findings, err := s.Findings(run.id)
	if err != nil || !reflect.DeepEqual(findings, []*finding.Finding{run.finding}) {
		t.Fatalf("%s stored findings = %+v, %v", run.name, findings, err)
	}
	patches, err := s.PatchFacts(run.id, store.PatchFilter{})
	want := &analyze.PatchFact{Instance: run.finding.SubjectInstance(), Seq: 1, File: run.name + ".xml",
		Line: 9, Class: "PatchOperationAdd", Desc: "Stored patch", Result: "ok", Hits: 1,
		Reasons: []string{"synthetic"}, Targets: []string{"ThingDef/Item"}}
	if err != nil || len(patches) != 1 {
		t.Fatalf("%s has %d stored patches, %v", run.name, len(patches), err)
	}
	if !reflect.DeepEqual(patches[0], want) {
		t.Fatalf("%s stored patch = %+v, want %+v", run.name, patches[0], want)
	}
	ignores, err := s.Ignores(run.envID)
	if err != nil || len(ignores) != 1 || *ignores[0] != *run.ignore {
		t.Fatalf("%s stored ignores = %+v, %v", run.name, ignores, err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ps.Analysis.Fresh || len(ps.Ignored) != 1 || ps.Ignored[0].Ref != run.problemRef {
		t.Fatalf("%s analysis = %+v, ignored = %+v, want %s", run.name, ps.Analysis, ps.Ignored, run.problemRef)
	}
	out := e.MustRun("problems", "--no-analyze", "--ignored")
	if !strings.Contains(out, fmt.Sprintf("a:%d ", run.id)) || !strings.Contains(out, run.problemRef) {
		t.Fatalf("%s lost stored references: %s", run.name, out)
	}
}

func TestUpgradeAnalysisStagePreservesPublishedRuns(t *testing.T) {
	e, runs := analysisAt44(t)
	e.Open()
	s := e.App.Store
	if got := s.Meta("schema_version"); got != "45" {
		t.Fatalf("schema = %s, want 45", got)
	}
	for _, run := range runs {
		assertLegacyAnalysis(t, e, run)
	}
	first := runs[0]
	s.MM = first.mm
	id, err := s.SaveAnalysis(first.game, first.slot, first.key, first.mm, []string{first.slot},
		&analyze.Output{Findings: []*analyze.Finding{first.finding}}, e.Now(), e.Now())
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestAnalysis(first.game, first.slot)
	if err != nil || latest == nil || latest.ID != id || id <= first.id {
		t.Fatalf("replacement analysis = %+v, %v", latest, err)
	}
	for table, column := range map[string]string{"analysis": "id", "finding": "analysis_id", "patch_fact": "analysis_id"} {
		if count := factCount(t, s, `SELECT count(*) FROM `+table+` WHERE `+column+`=?`, first.id); count != 0 {
			t.Errorf("%s retained %d rows for superseded a:%d", table, count, first.id)
		}
	}
	ignores, err := s.Ignores(first.envID)
	if err != nil || len(ignores) != 1 || *ignores[0] != *first.ignore {
		t.Fatalf("replacement changed the original ignore: %+v, %v", ignores, err)
	}
	assertLegacyAnalysis(t, e, runs[1])
}

func TestUpgradeAnalysisStageRollback(t *testing.T) {
	e, runs := analysisAt44(t)
	s := e.App.Store
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_analysis_upgrade BEFORE UPDATE OF value ON meta
		WHEN new.key='schema_version' AND new.value='45'
		BEGIN SELECT RAISE(ABORT,'injected analysis migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	before := schemaShape(t, s)
	path := s.Path
	s.Close()
	if upgraded, err := store.Open(path); err == nil {
		upgraded.Close()
		t.Fatal("expected injected upgrade failure")
	} else if !strings.Contains(err.Error(), "injected analysis migration failure") {
		t.Fatal(err)
	}
	legacy, err := store.OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if legacy.Meta("schema_version") != "44" || !reflect.DeepEqual(schemaShape(t, legacy), before) {
		t.Fatal("schema 45 changes survived rollback")
	}
	for _, run := range runs {
		if n := factCount(t, legacy, `SELECT count(*) FROM analysis WHERE id=? AND run_key=?`, run.id, run.key); n != 1 {
			t.Fatalf("legacy run a:%d changed during rollback", run.id)
		}
	}
	if _, err := legacy.DB.Exec(`DROP TRIGGER fail_analysis_upgrade`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	e.Open()
	for _, run := range runs {
		assertLegacyAnalysis(t, e, run)
	}
}
