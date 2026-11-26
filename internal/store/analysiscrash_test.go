package store_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/testkit/hide"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/store"
	"modernc.org/sqlite"
)

func TestAnalysisReclaimsAbandonedStage(t *testing.T) {
	s, reader, game := analysisWriteFixture(t)
	if _, err := s.DB.Exec(`INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen)
		VALUES('mod','','l','synthetic','mod','live',1,1)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	keep := []string{"plan", "live"}
	original, err := s.SaveAnalysis(game, "plan", "original", "1.6", keep, &analyze.Output{}, now, now)
	if err != nil {
		t.Fatal(err)
	}
	dead := startAnalysisStage(t, reader, game, "plan", "abandoned")
	dead.kill(t)
	if err := dead.cmd.Wait(); err == nil {
		t.Fatal("the abandoned writer exited successfully")
	}
	assertPending := func(id int64, key string) {
		t.Helper()
		if n := factCount(t, reader, `SELECT count(*) FROM analysis WHERE id=? AND publication=0`, id); n != 1 {
			t.Fatalf("expected a pending run at a:%d", id)
		}
		if n := factCount(t, reader, `SELECT count(*) FROM patch_fact WHERE analysis_id=?`, id); n == 0 {
			t.Fatal("the child did not commit a patch chunk")
		}
		patches, err := reader.PatchFacts(id, store.PatchFilter{})
		if err != nil || len(patches) != 0 {
			t.Fatalf("staged patches visible for a:%d: %d, %v", id, len(patches), err)
		}
		if reader.ContentKey("mod") == key {
			t.Fatalf("staged content %q became current", key)
		}
	}
	assertPending(dead.id, "abandoned")
	latest, err := reader.LatestAnalysis(game, "plan")
	if err != nil || latest == nil || latest.ID != original {
		t.Fatalf("published plan after process kill = %+v, %v", latest, err)
	}

	// A save soon after a crash honors the grace period. Advance only the dead
	// writer's recorded heartbeat so the next save sees it past the cutoff.
	if _, err := s.SaveAnalysis(game, "plan", "grace", "1.6", keep, &analyze.Output{}, now, now); err != nil {
		t.Fatal(err)
	}
	assertPending(dead.id, "abandoned")
	if _, err := s.DB.Exec(`UPDATE analysis SET finished_at=? WHERE id=?`, time.Now().Add(-time.Hour-time.Minute).Unix(), dead.id); err != nil {
		t.Fatal(err)
	}
	live := startAnalysisStage(t, reader, game, "live", "live")
	assertPending(live.id, "live")
	start := time.Now()
	replacement, err := s.SaveAnalysis(game, "plan", "replacement", "1.6", keep, &analyze.Output{}, now, now)
	if elapsed := time.Since(start); err != nil || elapsed > timingLimit(500*time.Millisecond) {
		t.Fatalf("reclaim during the next save took %v: %v", elapsed, err)
	}
	if replacement <= dead.id {
		t.Fatalf("expected the published run to skip a:%d, got a:%d", dead.id, replacement)
	}
	for table, column := range map[string]string{
		"analysis": "id", "patch_fact": "analysis_id", "analysis_stage_content": "analysis_id",
	} {
		if n := factCount(t, reader, `SELECT count(*) FROM `+table+` WHERE `+column+`=?`, dead.id); n != 0 {
			t.Errorf("%s retained %d rows for abandoned a:%d", table, n, dead.id)
		}
	}
	if n := factCount(t, reader, `SELECT count(*) FROM fact_set WHERE content_key='abandoned'`); n != 0 {
		t.Errorf("retained %d abandoned fact sets", n)
	}
	assertPending(live.id, "live")
	live.finish(t)
	latest, err = reader.LatestAnalysis(game, "live")
	if err != nil || latest == nil || latest.ID != live.id || reader.ContentKey("mod") != "live" {
		t.Fatalf("overlapping writer did not publish: %+v, %v", latest, err)
	}
	newest, err := reader.NewestAnalysis(game)
	if err != nil || newest == nil || newest.ID != live.id {
		t.Fatalf("newest analysis = %+v, %v, want last publication a:%d", newest, err, live.id)
	}
	patches, err := reader.PatchFacts(live.id, store.PatchFilter{})
	if err != nil || len(patches) != 4001 {
		t.Fatalf("overlapping writer published %d patches: %v", len(patches), err)
	}
}

type stagedAnalysisProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr bytes.Buffer
	id     int64
}

func startAnalysisStage(t *testing.T, reader *store.Store, game, slot, key string) *stagedAnalysisProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	child := &stagedAnalysisProcess{cmd: hide.Cmd(exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAnalysisStageChild$"))}
	isolated := t.TempDir()
	child.cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+filepath.Dir(reader.Path), "APPDATA="+isolated,
		"LOCALAPPDATA="+isolated, "RIMARCHO_ANALYSIS_TEST_GAME="+game, "RIMARCHO_ANALYSIS_TEST_SLOT="+slot,
		"RIMARCHO_ANALYSIS_TEST_KEY="+key)
	child.cmd.Stderr = &child.stderr
	stdout, err := child.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.stdin, err = child.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.cmd.ProcessState == nil {
			_ = child.cmd.Process.Kill()
			_ = child.cmd.Wait()
		}
	})
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "staged" {
		output := scanner.Text()
		_ = child.cmd.Process.Kill()
		err := child.cmd.Wait()
		t.Fatalf("child did not reach the pause: %s %s %v", output, child.stderr.String(), err)
	}
	if err := reader.DB.QueryRow(`SELECT id FROM analysis WHERE set_key=? AND publication=0 ORDER BY id DESC LIMIT 1`, slot).Scan(&child.id); err != nil {
		t.Fatal(err)
	}
	return child
}

func (p *stagedAnalysisProcess) kill(t *testing.T) {
	t.Helper()
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
}

func (p *stagedAnalysisProcess) finish(t *testing.T) {
	t.Helper()
	if _, err := fmt.Fprintln(p.stdin, "publish"); err != nil {
		t.Fatal(err)
	}
	p.stdin.Close()
	if err := p.cmd.Wait(); err != nil {
		t.Fatalf("child failed: %v %s", err, p.stderr.String())
	}
}

func TestAnalysisStageChild(t *testing.T) {
	key := os.Getenv("RIMARCHO_ANALYSIS_TEST_KEY")
	if key == "" {
		return
	}
	path := filepath.Join(os.Getenv("RIMARCHO_HOME"), "archo.db")
	base, err := sqlite.NewConnector("file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: sql.OpenDB(analysisStageConnector{base}), Path: path}
	defer s.Close()
	s.DB.SetMaxOpenConns(1)
	out := &analyze.Output{
		Content: map[string]*analyze.ContentState{"mod": {Key: key}},
		NewDefs: map[string][]analyze.DefFact{"mod": {{Type: "ThingDef", DefName: key, File: "Defs/Things.xml"}}},
		Patches: make([]*analyze.PatchFact, 4001),
	}
	patch := &analyze.PatchFact{Instance: "mod", Class: "PatchOperationAdd"}
	for i := range out.Patches {
		out.Patches[i] = patch
	}
	now := time.Now()
	if _, err := s.SaveAnalysis(os.Getenv("RIMARCHO_ANALYSIS_TEST_GAME"), os.Getenv("RIMARCHO_ANALYSIS_TEST_SLOT"),
		key, "1.6", []string{"plan", "live"}, out, now, now); err != nil {
		t.Fatal(err)
	}
}

type analysisStageConnector struct{ driver.Connector }

func (c analysisStageConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &analysisStageConn{Conn: conn, ExecQuerierContext: conn.(sqlite.ExecQuerierContext)}, nil
}

type analysisStageConn struct {
	driver.Conn
	sqlite.ExecQuerierContext
	transactions int
}

func (c *analysisStageConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.transactions++
	// Pause before opening the next transaction, after the content and the
	// first patch chunk committed and the SQLite write lock was released.
	if c.transactions == 3 {
		fmt.Println("staged")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) != "publish" {
			return nil, fmt.Errorf("unexpected parent response %q", line)
		}
	}
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
