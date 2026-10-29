package app_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMaintenanceRetentionAndHistory(t *testing.T) {
	e := testkit.New(t)
	s := e.App.Store
	ids := testkit.MaintenanceHistory(t, s, "history", 10, 4, 20)
	other := testkit.MaintenanceHistory(t, s, "other", 6, 4, 20)
	current := testkit.MaintenanceHistory(t, s, "current-only", 1, 4, 20)
	tombstone := testkit.MaintenanceHistory(t, s, "tombstone-only", 1, 4, 20)
	var scan int64
	if err := s.DB.QueryRow(`SELECT scan_id FROM snapshot WHERE id=?`, ids[2]).Scan(&scan); err != nil {
		t.Fatal(err)
	}
	// The observation is historical and the candidate's current snapshot is newer
	// than its question-time evidence. All three must survive.
	if _, err := s.DB.Exec(`INSERT INTO identity_question(environment_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score) VALUES('',?,'other',?,'history',0.9)`, scan, other[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE instance SET status='tombstone',tombstone_at=1 WHERE id='history'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE instance SET status='tombstone',tombstone_at=1 WHERE id='tombstone-only'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO identity_question(environment_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score,status) VALUES('',?,'other',?,'history',0.9,'resolved')`, scan, ids[1]); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.Maintain(context.Background(), false, true)
	if err != nil || r.Files != 24 {
		t.Fatalf("preview %+v %v", r, err)
	}
	if f, _ := s.SnapshotFiles(ids[0]); len(f) != 4 {
		t.Fatal("preview deleted data")
	}
	r, err = e.App.Maintain(context.Background(), false, false)
	if err != nil || r.Files != 24 || !r.Vacuumed {
		t.Fatalf("maintain %+v %v", r, err)
	}
	for _, id := range append(append([]int64{ids[2], other[0], current[0], tombstone[0]}, ids[6:]...), other[2:]...) {
		if f, err := s.SnapshotFiles(id); err != nil || len(f) != 4 {
			t.Fatalf("protected %d: %v %v", id, f, err)
		}
	}
	q := &store.Question{InstanceID: "other", CandidateID: "history", SnapshotID: other[0], ScanID: scan}
	if ev, err := s.QuestionEvidence(q); err != nil || len(ev.New.Files) == 0 || len(ev.Candidate.Files) == 0 {
		t.Fatalf("question lost evidence %+v %v", ev, err)
	}
	for _, id := range []int64{ids[0], ids[1], ids[3], ids[4], ids[5], other[1]} {
		if f, _ := s.SnapshotFiles(id); f != nil {
			t.Fatalf("old list %d retained", id)
		}
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM snapshot`).Scan(&n)
	if n != 18 {
		t.Fatalf("snapshots removed: %d", n)
	}
	s.DB.QueryRow(`SELECT count(*) FROM change_event`).Scan(&n)
	if n != 14 {
		t.Fatalf("events removed: %d", n)
	}
	s.DB.QueryRow(`SELECT count(*) FROM instance WHERE status='tombstone'`).Scan(&n)
	if n != 2 {
		t.Fatal("automatic tombstone GC")
	}
	lines := modchanges.Files(e.App, modchanges.English(time.UTC), ids[0], ids[1])
	if len(lines) != 1 || lines[0] != "(file lists no longer stored)" {
		t.Fatalf("fallback %v", lines)
	}
	r, err = e.App.Maintain(context.Background(), true, false)
	if err != nil || !r.Skipped {
		t.Fatalf("interval %+v %v", r, err)
	}
	e.Advance(store.MaintenanceInterval)
	r, err = e.App.Maintain(context.Background(), true, false)
	if err != nil || r.Skipped || r.Vacuumed {
		t.Fatalf("due small database %+v %v", r, err)
	}
}

func TestMaintenanceBusySpaceAndCancellation(t *testing.T) {
	for _, mode := range []string{"writer", "reader", "space", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			s := e.App.Store
			testkit.MaintenanceHistory(t, s, "history", 8, 200, 80)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "writer" || mode == "reader" {
				external, err := store.OpenSession(s.Path)
				if err != nil {
					t.Fatal(err)
				}
				defer external.Close()
				tx, err := external.DB.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if mode == "writer" {
					_, err = tx.Exec(`UPDATE meta SET value=value WHERE key='schema_version'`)
				} else {
					var n int
					err = tx.QueryRow(`SELECT count(*) FROM snapshot_file`).Scan(&n)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "space" {
				e.App.FreeSpace = func(string) int64 { return 0 }
			}
			if mode == "cancel" {
				cancel()
			}
			start := time.Now()
			r, err := e.App.Maintain(ctx, false, false)
			if err == nil {
				t.Fatalf("expected refusal %+v", r)
			}
			if (mode == "writer" || mode == "reader") && !errors.Is(err, store.ErrMaintenanceBusy) {
				t.Fatal(err)
			}
			if mode == "space" && (!strings.Contains(err.Error(), "free disk space") || r.Vacuumed) {
				t.Fatalf("space %+v %v", r, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("busy/cancel waited")
			}
			// Release the held connection before reading completion metadata.
			if mode != "writer" && mode != "reader" && s.Meta(store.MaintenanceLastKey) != "" {
				t.Fatal("failed run marked complete")
			}
		})
	}
}

func TestMaintenanceGameRefusalAndReclaimsSpace(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 123} }
	if _, _, err := e.App.DBVacuum(); err == nil || !strings.Contains(err.Error(), "close the game first") {
		t.Fatalf("game refusal: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	testkit.MaintenanceHistory(t, e.App.Store, "history", 20, 1000, 220)
	e.App.Store.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	r, err := e.App.Maintain(context.Background(), true, false)
	if err != nil || !r.Vacuumed || r.Before-r.After < 3<<20 {
		t.Fatalf("space not reclaimed %+v %v", r, err)
	}
	if f, err := os.Stat(e.App.Store.Path + "-wal"); err == nil && f.Size() != 0 {
		t.Fatalf("WAL not truncated: %d", f.Size())
	}
	var integrity string
	e.App.Store.DB.QueryRow(`PRAGMA integrity_check`).Scan(&integrity)
	if integrity != "ok" {
		t.Fatal(integrity)
	}
}

// Opt in separately so ordinary regressions stay small and parallel-test disks
// cannot accidentally accumulate medium databases.
func TestMaintenanceMedium(t *testing.T) {
	if os.Getenv("RIMARCHO_MAINTENANCE_BENCH") != "1" {
		t.Skip("opt-in synthetic disk benchmark")
	}
	e := testkit.New(t)
	testkit.MaintenanceHistory(t, e.App.Store, "medium", 40, 15000, 512)
	e.App.Store.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	start := time.Now()
	r, err := e.App.Maintain(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before=%d after=%d reclaimed=%d files=%d elapsed=%s", r.Before, r.After, r.Before-r.After, r.Files, time.Since(start))
	if r.Before < 300<<20 || r.Before > 400<<20 || !r.Vacuumed || r.Before-r.After < 250<<20 {
		t.Fatalf("unexpected medium result %+v", r)
	}
}
