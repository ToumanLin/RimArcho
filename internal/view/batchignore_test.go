package view_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
	"modernc.org/sqlite"
)

type ignoreProbe struct {
	driver.Connector
	problems, commits int
}

func (p *ignoreProbe) Connect(ctx context.Context) (driver.Conn, error) {
	c, err := p.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	c.(sqlite.HookRegisterer).RegisterCommitHook(func() int32 {
		p.commits++
		return 0
	})
	return &ignoreProbeConn{Conn: c, ExecQuerierContext: c.(sqlite.ExecQuerierContext), probe: p}, nil
}

type ignoreProbeConn struct {
	driver.Conn
	sqlite.ExecQuerierContext
	probe *ignoreProbe
}

func (c *ignoreProbeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// The declaration and stored findings are collected together with the ignore rows.
	if strings.Contains(query, "FROM finding_ignore WHERE environment_id=? ORDER BY id") {
		c.probe.problems++
	}
	return c.ExecQuerierContext.QueryContext(ctx, query, args)
}

func TestIgnoreMergedDependency(t *testing.T) {
	e := testkit.New(t)
	const count = 200
	active := []string{"ludeon.rimworld"}
	for i := range count {
		m := testkit.Sample(fmt.Sprintf("test.dependent%d", i), fmt.Sprintf("Dependent %d", i))
		m.Deps = []string{"brrainz.harmony"}
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
		active = append(active, m.PackageID)
	}
	e.WriteModsConfig("1.6.4633", active...)
	e.AddEnv(false)
	e.MustRun("scan")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var action view.Action
	for _, item := range view.BuildNeeds(e.App, ps.State, ps).Items {
		for _, candidate := range item.Actions {
			if candidate.Kind == view.ActIgnore && len(candidate.Refs) > len(action.Refs) {
				action = candidate
			}
		}
	}
	if len(action.Refs) != count {
		t.Fatalf("merged dependency covers %d findings, want %d", len(action.Refs), count)
	}
	s := e.App.Store
	base, err := sqlite.NewConnector("file:" + filepath.ToSlash(s.Path) + "?_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	probe := &ignoreProbe{Connector: base}
	original := s.DB
	s.DB = sql.OpenDB(probe)
	s.DB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		s.DB.Close()
		s.DB = original
	})
	start := time.Now()
	if _, err := view.Do(e.App, action, 0); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	t.Logf("%d refs in %v, %d problem computations, %d write commits", count, elapsed, probe.problems, probe.commits)
	// The counts are the guard; the time catches a return to per-Mod work (2.25 s on the
	// base before grouping, 18) and leaves room for CI (526 ms there; 24–29 ms locally).
	if probe.problems != 1 || probe.commits != 1 || elapsed > timingLimit(1500*time.Millisecond) {
		t.Error("expected one problem computation and one write transaction within 1.5 s")
	}
	rows, err := s.Ignores(ps.State.Ix.Env.ID)
	if err != nil || len(rows) != count {
		t.Fatalf("stored %d ignores, want %d: %v", len(rows), count, err)
	}
	for _, row := range rows {
		if row.LineageID == "" || row.SnapshotID == 0 || row.Code != "dep.missing" || !row.CreatedAt.Equal(e.Now()) {
			t.Errorf("incorrect ignore: %+v", row)
		}
	}
}
