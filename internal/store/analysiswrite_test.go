package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"modernc.org/sqlite"
)

func analysisWriteFixture(t *testing.T) (*store.Store, *store.Store, string) {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(false)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	s := e.App.Store
	reader, err := store.OpenSession(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	s.MM, reader.MM = "1.6", "1.6"
	return s, reader, st.Ix.Env.GameID
}

func watchAnalysisWrites(t *testing.T, s *store.Store, fn sqlite.PreUpdateHookFn) {
	t.Helper()
	c, err := s.DB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Raw(func(raw any) error {
		raw.(sqlite.HookRegisterer).RegisterPreUpdateHook(fn)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAnalysisYieldsToSmallWrite(t *testing.T) {
	s, other, game := analysisWriteFixture(t)
	const mods, defsPerMod = 64, 4000
	if _, err := s.DB.Exec(`WITH RECURSIVE nums(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM nums WHERE n+1<?)
		INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen)
		SELECT 'mod'||n,'','l','synthetic','mod'||n,'live',1,1 FROM nums`, mods); err != nil {
		t.Fatal(err)
	}
	out := &analyze.Output{Content: map[string]*analyze.ContentState{}, NewDefs: map[string][]analyze.DefFact{}}
	defs := make([]analyze.DefFact, defsPerMod)
	for i := range defs {
		defs[i] = analyze.DefFact{Type: "ThingDef", DefName: fmt.Sprintf("Thing_%05d", i), File: "Defs/Things.xml", Line: i + 1}
	}
	for i := range mods {
		inst := fmt.Sprintf("mod%d", i)
		out.Content[inst] = &analyze.ContentState{Key: inst}
		out.NewDefs[inst] = defs
	}
	started := make(chan struct{})
	var once sync.Once
	watchAnalysisWrites(t, s, func(d sqlite.SQLitePreUpdateData) {
		if d.TableName == "def_fact" {
			once.Do(func() { close(started) })
		}
	})
	done := make(chan error, 1)
	now := time.Now()
	go func() {
		_, err := s.SaveAnalysis(game, "plan", "large", "1.6", []string{"plan"}, out, now, now)
		done <- err
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("analysis finished before the first Def write: %v", err)
	}
	start := time.Now()
	err := other.SetMeta("test-edit", "committed")
	elapsed := time.Since(start)
	t.Logf("independent write during %d Def inserts took %v (%v)", mods*defsPerMod, elapsed, err)
	if err != nil {
		t.Errorf("independent write failed: %v", err)
	}
	if !raceBuild && elapsed > 500*time.Millisecond {
		t.Error("independent write exceeded 500 ms")
	}
	select {
	case err := <-done:
		t.Fatalf("analysis finished before the independent write returned: %v", err)
	default:
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	run, err := other.LatestAnalysis(game, "plan")
	if err != nil || run == nil || run.RunKey != "large" {
		t.Fatalf("completed analysis = %+v, %v", run, err)
	}
	if n := factCount(t, other, `SELECT count(*) FROM def_fact`); n != mods*defsPerMod {
		t.Fatalf("saved %d Defs, want %d", n, mods*defsPerMod)
	}
}

func TestAnalysisCancelKeepsPublishedRun(t *testing.T) {
	s, reader, game := analysisWriteFixture(t)
	if _, err := s.DB.Exec(`INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen)
		VALUES('mod','','l','synthetic','mod','live',1,1)`); err != nil {
		t.Fatal(err)
	}
	output := func(key string) *analyze.Output {
		return &analyze.Output{
			Content:  map[string]*analyze.ContentState{"mod": {Key: key}},
			NewDefs:  map[string][]analyze.DefFact{"mod": {{Type: "ThingDef", DefName: key, File: "Defs/Things.xml"}}},
			Patches:  []*analyze.PatchFact{{Instance: "mod", Class: key}},
			Findings: []*analyze.Finding{{Layer: "def", Code: "synthetic", Key: key}},
		}
	}
	now := time.Now()
	first, err := s.SaveAnalysis(game, "plan", "old", "1.6", []string{"plan"}, output("old"), now, now)
	if err != nil {
		t.Fatal(err)
	}
	pause, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var releaseOnce sync.Once
	unpause := func() { releaseOnce.Do(func() { close(release) }) }
	defer unpause()
	next := output("new")
	for len(next.NewDefs["mod"]) < 9000 {
		next.NewDefs["mod"] = append(next.NewDefs["mod"], next.NewDefs["mod"][0])
	}
	for len(next.Patches) < 9000 {
		next.Patches = append(next.Patches, next.Patches[0])
	}
	var patches int
	watchAnalysisWrites(t, s, func(d sqlite.SQLitePreUpdateData) {
		if d.TableName == "patch_fact" {
			patches++
			if patches == 5000 {
				close(pause)
				<-release
			}
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := s.SaveAnalysisContext(ctx, game, "plan", "new", "1.6", []string{"plan"}, next, now, now)
		done <- err
	}()
	select {
	case <-pause:
	case err := <-done:
		t.Fatalf("analysis finished before staging its patch rows: %v", err)
	}
	assertOld := func() {
		t.Helper()
		run, err := reader.LatestAnalysis(game, "plan")
		if err != nil || run == nil || run.ID != first || run.RunKey != "old" {
			t.Errorf("current analysis = %+v, %v, want run %d", run, err, first)
		}
		defs, err := reader.DefsOf("mod")
		if err != nil || len(defs) != 1 || defs[0].DefName != "old" || defs[0].File != "Defs/Things.xml" {
			t.Errorf("current Defs = %+v, %v", defs, err)
		}
		if _, ok := reader.CachedDefs("mod", "new"); ok {
			t.Error("staged Defs are visible through the cache")
		}
		findings, err := reader.Findings(first)
		if err != nil || len(findings) != 1 || findings[0].Key != "old" {
			t.Errorf("current findings = %+v, %v", findings, err)
		}
	}
	assertOld()
	var stagedID int64
	if err := reader.DB.QueryRow(`SELECT id FROM analysis WHERE id<>?`, first).Scan(&stagedID); err != nil {
		t.Error(err)
	}
	if n := factCount(t, reader, `SELECT count(*) FROM patch_fact WHERE analysis_id=?`, stagedID); n == 0 {
		t.Error("expected a committed patch chunk before cancellation")
	}
	if ps, err := reader.PatchFacts(stagedID, store.PatchFilter{}); err != nil || len(ps) != 0 {
		t.Errorf("staged patches are visible: %d, %v", len(ps), err)
	}
	cancel()
	unpause()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("save after cancellation: %v", err)
	}
	assertOld()
	for _, table := range []string{"analysis", "fact_set", "def_fact", "patch_fact", "finding"} {
		if n := factCount(t, reader, `SELECT count(*) FROM `+table); n != 1 {
			t.Errorf("%s has %d rows after cancellation, want 1", table, n)
		}
	}
	if _, err := s.SaveAnalysis(game, "plan", "new", "1.6", []string{"plan"}, next, now, now); err != nil {
		t.Fatal(err)
	}
	defs, err := reader.DefsOf("mod")
	if err != nil || len(defs) != 9000 || defs[8999].File != "Defs/Things.xml" {
		t.Fatalf("retry saved %d Defs with error %v", len(defs), err)
	}
	if n := factCount(t, reader, `SELECT count(*) FROM fact_set`); n != 1 {
		t.Errorf("retry retained %d fact sets, want 1", n)
	}
}
