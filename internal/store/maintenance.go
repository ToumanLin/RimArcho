package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	MaintenanceInterval = 7 * 24 * time.Hour
	MaintenanceUpdates  = 3
	MaintenanceLastKey  = "maintenance:last_completed"
	maintenanceLeaseKey = "maintenance:lease"
)

var ErrMaintenanceBusy = errors.New("database is busy; try compacting again later")

// NeedSpaceError: the volume has less free space than a VACUUM may need (Need bytes).
type NeedSpaceError struct{ Need int64 }

func (e *NeedSpaceError) Error() string {
	return fmt.Sprintf("cannot compact database: it needs %d bytes of free disk space", e.Need)
}

type MaintenanceOptions struct {
	Now       time.Time
	Automatic bool
	DryRun    bool
	FreeSpace func(string) int64
}

type MaintenanceResult struct {
	Before, After     int64
	Files             int64
	Vacuumed, Skipped bool
}

// Maintain owns the ordered maintenance pipeline (02 §7). It always uses a private
// connection: page connections, their busy timeouts and their caches are untouched.
func (s *Store) Maintain(ctx context.Context, o MaintenanceOptions) (r MaintenanceResult, err error) {
	w, err := OpenSession(s.Path)
	if err != nil {
		return r, err
	}
	defer w.Close()
	c, err := w.DB.Conn(ctx)
	if err != nil {
		return r, err
	}
	defer c.Close()
	defer func() {
		var code interface{ Code() int }
		if errors.As(err, &code) && (code.Code()&255 == 5 || code.Code()&255 == 6) {
			err = ErrMaintenanceBusy
		}
	}()
	for _, q := range []string{"PRAGMA busy_timeout=0", "PRAGMA cache_size=-2048", "PRAGMA temp_store=FILE"} {
		if _, err = c.ExecContext(ctx, q); err != nil {
			return r, err
		}
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	// A write reservation makes concurrent GUI windows recheck the interval after
	// the preceding window's commit, rather than both claiming the same week.
	if _, err = c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return r, err
	}
	defer c.ExecContext(context.Background(), "ROLLBACK")
	var last string
	err = c.QueryRowContext(ctx, "SELECT value FROM meta WHERE key=?", MaintenanceLastKey).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return r, err
	}
	if o.Automatic {
		if t, e := time.Parse(time.RFC3339, last); e == nil && o.Now.Before(t.Add(MaintenanceInterval)) {
			r.Skipped = true
			return r, nil
		}
	}
	err = nil
	var lease string
	if err = c.QueryRowContext(ctx, "SELECT value FROM meta WHERE key=?", maintenanceLeaseKey).Scan(&lease); err != nil && err != sql.ErrNoRows {
		return r, err
	}
	if t, e := time.Parse(time.RFC3339, lease); e == nil && o.Now.Before(t.Add(15*time.Minute)) {
		return r, ErrMaintenanceBusy
	}
	err = nil
	r.Before = databaseBytes(s.Path)
	// 1. Prune file lists, retaining whole snapshots used for identity and recent
	// updates. The candidate selector must match questionEvidence exactly.
	_, err = c.ExecContext(ctx, `CREATE TEMP TABLE maintenance_keep(id INTEGER PRIMARY KEY);
		INSERT OR IGNORE INTO maintenance_keep SELECT current_snapshot_id FROM instance WHERE current_snapshot_id IS NOT NULL;
		INSERT OR IGNORE INTO maintenance_keep SELECT observation_snapshot_id FROM identity_question WHERE status='open';
		INSERT OR IGNORE INTO maintenance_keep SELECT
		 (SELECT n.id FROM snapshot n WHERE n.instance_id=q.candidate_instance_id AND n.scan_id<=q.scan_id AND n.id<>q.observation_snapshot_id ORDER BY n.scan_id DESC,n.id DESC LIMIT 1)
		 FROM identity_question q WHERE q.status='open' AND EXISTS(SELECT 1 FROM snapshot n WHERE n.instance_id=q.candidate_instance_id AND n.scan_id<=q.scan_id AND n.id<>q.observation_snapshot_id);`)
	if err != nil {
		return r, err
	}
	_, err = c.ExecContext(ctx, `INSERT OR IGNORE INTO maintenance_keep
		SELECT before_snapshot FROM (SELECT before_snapshot, row_number() OVER(PARTITION BY instance_id ORDER BY scan_id DESC,id DESC) AS n FROM change_event WHERE kind='updated') WHERE n<=? AND before_snapshot IS NOT NULL
		UNION SELECT after_snapshot FROM (SELECT after_snapshot, row_number() OVER(PARTITION BY instance_id ORDER BY scan_id DESC,id DESC) AS n FROM change_event WHERE kind='updated') WHERE n<=? AND after_snapshot IS NOT NULL`, MaintenanceUpdates, MaintenanceUpdates)
	if err != nil {
		return r, err
	}
	const obsolete = `snapshot_id NOT IN (SELECT id FROM maintenance_keep)`
	if o.DryRun {
		err = c.QueryRowContext(ctx, `SELECT count(*) FROM snapshot_file WHERE `+obsolete).Scan(&r.Files)
		r.After = r.Before
		return r, err
	}
	res, err := c.ExecContext(ctx, `DELETE FROM snapshot_file WHERE `+obsolete)
	if err != nil {
		return r, err
	}
	r.Files, err = res.RowsAffected()
	if err != nil {
		return r, err
	}
	// 2. Version views no game needs and the fact sets nothing references (03 §3).
	if err = pruneFactsOn(ctx, c, o.Now); err != nil {
		return r, err
	}
	if _, err = c.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, maintenanceLeaseKey, o.Now.UTC().Format(time.RFC3339)); err != nil {
		return r, err
	}
	// 3. Commit pruning before VACUUM, which cannot run inside a transaction.
	if _, err = c.ExecContext(ctx, "COMMIT"); err != nil {
		return r, err
	}
	completionWritten := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err != nil && completionWritten {
			// A reader can arrive between the data checkpoint and the marker's
			// checkpoint. Preserve eligibility for retry when that last step fails.
			if last == "" {
				c.ExecContext(cleanup, "DELETE FROM meta WHERE key=?", MaintenanceLastKey)
			} else {
				c.ExecContext(cleanup, "UPDATE meta SET value=? WHERE key=?", last, MaintenanceLastKey)
			}
		}
		c.ExecContext(cleanup, "DELETE FROM meta WHERE key=?", maintenanceLeaseKey)
		// VACUUM can be cancelled after generating WAL frames. Always attempt to
		// release them, including when the caller cancelled foreground work.
		_ = maintenanceCheckpoint(cleanup, c)
		r.After = databaseBytes(s.Path)
	}()
	var pages, free, pageSize int64
	for _, x := range []struct {
		q string
		p *int64
	}{{"PRAGMA page_count", &pages}, {"PRAGMA freelist_count", &free}, {"PRAGMA page_size", &pageSize}} {
		if err = c.QueryRowContext(ctx, x.q).Scan(x.p); err != nil {
			return r, err
		}
	}
	// 4. Rebuild only when worthwhile automatically; explicit compact always does.
	if !o.Automatic || free*pageSize >= 16<<20 || (free*pageSize >= 1<<20 && free*5 >= pages) {
		// SQLite may need a temporary database AND a full WAL concurrently.
		need := 2*pages*pageSize + 64<<20
		if o.FreeSpace == nil || o.FreeSpace(s.Path) < need {
			return r, &NeedSpaceError{Need: need}
		}
		if err = maintenanceCheckpoint(ctx, c); err != nil {
			return r, err
		}
		if _, err = c.ExecContext(ctx, "VACUUM"); err != nil {
			return r, err
		}
		r.Vacuumed = true
	}
	// 5. Checkpoint reports busy as a result column, not as an SQLite error.
	if err = maintenanceCheckpoint(ctx, c); err != nil {
		return r, err
	}
	r.After = databaseBytes(s.Path)
	_, err = c.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, MaintenanceLastKey, o.Now.UTC().Format(time.RFC3339))
	if err != nil {
		return r, err
	}
	completionWritten = true
	// Include the completion marker in the final checkpoint and reported size.
	err = maintenanceCheckpoint(ctx, c)
	r.After = databaseBytes(s.Path)
	return r, err
}

func maintenanceCheckpoint(ctx context.Context, c *sql.Conn) error {
	var busy, log, done int
	if err := c.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &done); err != nil {
		return err
	}
	if busy != 0 {
		return ErrMaintenanceBusy
	}
	return nil
}

func databaseBytes(path string) int64 {
	var n int64
	for _, suffix := range []string{"", "-wal"} {
		if f, err := os.Stat(path + suffix); err == nil {
			n += f.Size()
		}
	}
	return n
}
