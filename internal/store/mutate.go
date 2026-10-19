package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rimarcho/rimarcho/internal/identity"
)

func (s *Store) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ResolveSame merges the question's new Instance into the candidate (02 §5.4 "same"): the
// candidate takes over the location, snapshots and events, and comes back to life.
func (s *Store) ResolveSame(q *Question, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		var candStatus string
		var candSnap sql.NullInt64
		if err := tx.QueryRow(`SELECT status, current_snapshot_id FROM instance WHERE id=?`, q.CandidateID).Scan(&candStatus, &candSnap); err != nil {
			return fmt.Errorf("candidate #%s: %w", q.CandidateID, err)
		}
		if candStatus == identity.Live {
			return fmt.Errorf("candidate #%s is live again; answer copy or unrelated instead", q.CandidateID)
		}
		var src, rel, real string
		var residue int
		var lastSeen int64
		var newSnap sql.NullInt64
		if err := tx.QueryRow(`SELECT source_id, rel_path, real_path, residue, last_seen, current_snapshot_id, status FROM instance WHERE id=?`, q.InstanceID).
			Scan(&src, &rel, &real, &residue, &lastSeen, &newSnap, new(string)); err != nil {
			return fmt.Errorf("instance #%s: %w", q.InstanceID, err)
		}
		stmts := []struct {
			q    string
			args []any
		}{
			{`UPDATE instance SET source_id=?, rel_path=?, real_path=?, residue=?, last_seen=?, status=?, tombstone_at=NULL, current_snapshot_id=? WHERE id=?`,
				[]any{src, rel, real, residue, lastSeen, identity.Live, newSnap, q.CandidateID}},
			{`UPDATE snapshot SET instance_id=? WHERE instance_id=?`, []any{q.CandidateID, q.InstanceID}},
			{`DELETE FROM change_event WHERE instance_id=? AND kind=?`, []any{q.InstanceID, identity.EvDiscovered}},
			{`UPDATE change_event SET instance_id=? WHERE instance_id=?`, []any{q.CandidateID, q.InstanceID}},
			{`UPDATE OR REPLACE instance_alias SET instance_id=? WHERE instance_id=?`, []any{q.CandidateID, q.InstanceID}},
			{`INSERT INTO change_event(scan_id, instance_id, kind, before_snapshot, after_snapshot, detail) VALUES(?,?,?,?,?,?)`,
				[]any{q.ScanID, q.CandidateID, identity.EvReappeared, candSnap, newSnap, fmt.Sprintf("confirmed by player (q:%d)", q.ID)}},
			{`DELETE FROM instance WHERE id=?`, []any{q.InstanceID}},
			{`UPDATE identity_question SET status='resolved', resolution='same', resolved_at=? WHERE id=?`, []any{now.Unix(), q.ID}},
			{`UPDATE identity_question SET status='resolved', resolution='superseded', resolved_at=? WHERE status='open' AND id<>? AND (instance_id=? OR candidate_instance_id=?)`,
				[]any{now.Unix(), q.ID, q.InstanceID, q.CandidateID}},
		}
		for _, st := range stmts {
			if _, err := tx.Exec(st.q, st.args...); err != nil {
				return err
			}
		}
		return recomputePrimaries(tx, q.EnvID)
	})
}

// ResolveCopy puts the new Instance into the candidate's lineage, confirmed by the player.
func (s *Store) ResolveCopy(q *Question, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		var lin string
		if err := tx.QueryRow(`SELECT lineage_id FROM instance WHERE id=?`, q.CandidateID).Scan(&lin); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE instance SET lineage_id=?, lineage_basis=? WHERE id=?`, lin, identity.BasisConfirmed, q.InstanceID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE identity_question SET status='resolved', resolution='copy', resolved_at=? WHERE id=?`, now.Unix(), q.ID); err != nil {
			return err
		}
		return recomputePrimaries(tx, q.EnvID)
	})
}

// ResolveUnrelated closes the question without linking anything.
func (s *Store) ResolveUnrelated(q *Question, now time.Time) error {
	_, err := s.DB.Exec(`UPDATE identity_question SET status='resolved', resolution='unrelated', resolved_at=? WHERE id=?`, now.Unix(), q.ID)
	return err
}

// SplitLineage moves an Instance into a new lineage of its own.
func (s *Store) SplitLineage(envID, instanceID, newLineageID string, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) VALUES(?,?,?,?)`, newLineageID, envID, instanceID, now.Unix()); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE instance SET lineage_id=?, lineage_basis=? WHERE id=?`, newLineageID, identity.BasisManual, instanceID); err != nil {
			return err
		}
		return recomputePrimaries(tx, envID)
	})
}

// MergeLineages moves every Instance of `from` into `into`.
func (s *Store) MergeLineages(envID, into, from string) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE instance SET lineage_id=?, lineage_basis=? WHERE lineage_id=?`, into, identity.BasisManual, from); err != nil {
			return err
		}
		return recomputePrimaries(tx, envID)
	})
}

// GCResult counts what a garbage collection removed.
type GCResult struct {
	Instances []string
	Kept      int // expired tombstones kept because an open question references them
	Snapshots int
	Files     int64
}

// GC removes tombstones that died before cutoff (02 §5.5) unless an open question refers to
// them, and drops superseded snapshots no event refers to. dryRun only counts.
func (s *Store) GC(envID string, cutoff time.Time, dryRun bool) (*GCResult, error) {
	r := &GCResult{}
	err := s.inTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id, EXISTS(SELECT 1 FROM identity_question q WHERE q.status='open' AND (q.instance_id=i.id OR q.candidate_instance_id=i.id))
			FROM instance i WHERE environment_id=? AND status=? AND tombstone_at < ? ORDER BY id`, envID, identity.Tombstone, cutoff.Unix())
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var held bool
			rows.Scan(&id, &held)
			if held {
				r.Kept++
			} else {
				r.Instances = append(r.Instances, id)
			}
		}
		rows.Close()
		for _, id := range r.Instances {
			for _, q := range []string{
				`DELETE FROM snapshot_file WHERE snapshot_id IN (SELECT id FROM snapshot WHERE instance_id=?)`,
				`DELETE FROM snapshot WHERE instance_id=?`,
				`DELETE FROM instance_alias WHERE instance_id=?`,
				`DELETE FROM change_event WHERE instance_id=?`,
				`DELETE FROM identity_question WHERE instance_id=? OR candidate_instance_id=?`,
				`DELETE FROM instance WHERE id=?`,
			} {
				args := []any{id}
				if n := countQ(q); n == 2 {
					args = append(args, id)
				}
				if _, err := tx.Exec(q, args...); err != nil {
					return err
				}
			}
		}
		// Superseded snapshots: not current, not referenced by any event or question.
		orphan := `SELECT n.id FROM snapshot n JOIN instance i ON i.id=n.instance_id WHERE i.environment_id=? AND n.id <> coalesce(i.current_snapshot_id, -1)
			AND NOT EXISTS(SELECT 1 FROM change_event e WHERE e.before_snapshot=n.id OR e.after_snapshot=n.id)
			AND NOT EXISTS(SELECT 1 FROM identity_question q WHERE q.observation_snapshot_id=n.id)`
		rows, err = tx.Query(orphan, envID)
		if err != nil {
			return err
		}
		var snaps []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			snaps = append(snaps, id)
		}
		rows.Close()
		r.Snapshots = len(snaps)
		for _, id := range snaps {
			var n int64
			tx.QueryRow(`SELECT count(*) FROM snapshot_file WHERE snapshot_id=?`, id).Scan(&n)
			r.Files += n
			if _, err := tx.Exec(`DELETE FROM snapshot_file WHERE snapshot_id=?`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM snapshot WHERE id=?`, id); err != nil {
				return err
			}
		}
		if err := recomputePrimaries(tx, envID); err != nil {
			return err
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if err == errDryRun {
		err = nil
	}
	return r, err
}

var errDryRun = fmt.Errorf("dry run")

func countQ(q string) int {
	n := 0
	for _, c := range q {
		if c == '?' {
			n++
		}
	}
	return n
}
