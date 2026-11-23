package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
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

// ResolveSame merges the question's new Instance into the candidate (04 §2.4 "same"): the
// candidate takes over the location, snapshots and events, and comes back to life. Every
// stored reference follows through the shared rewrite of 03 §2.1 (rewriteInstances,
// rewriteOrigins and, when the new Instance's Lineage is left empty, rewriteLineages,
// mergePlayerData and mergeRules), so notes, tags, rules and ignores move with it and the
// emptied Lineage is deleted.
func (s *Store) ResolveSame(q *Question, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		if err := lockHistory(tx); err != nil {
			return err
		}
		var candStatus, keptLineage string
		var candSnap sql.NullInt64
		if err := tx.QueryRow(`SELECT status, current_snapshot_id, lineage_id FROM instance WHERE id=?`, q.CandidateID).Scan(&candStatus, &candSnap, &keptLineage); err != nil {
			return fmt.Errorf("candidate #%s: %w", q.CandidateID, err)
		}
		if candStatus == identity.Live {
			return fmt.Errorf("candidate #%s is live again; answer copy or unrelated instead", q.CandidateID)
		}
		var src, rel, real, oldLineage string
		var residue int
		var lastSeen int64
		var newSnap sql.NullInt64
		if err := tx.QueryRow(`SELECT source_id, rel_path, real_path, residue, last_seen, current_snapshot_id, lineage_id FROM instance WHERE id=?`, q.InstanceID).
			Scan(&src, &rel, &real, &residue, &lastSeen, &newSnap, &oldLineage); err != nil {
			return fmt.Errorf("instance #%s: %w", q.InstanceID, err)
		}
		var remaining int
		if err := tx.QueryRow(`SELECT count(*) FROM instance WHERE lineage_id=? AND id<>?`, oldLineage, q.InstanceID).Scan(&remaining); err != nil {
			return err
		}
		instances := map[string]string{q.InstanceID: q.CandidateID}
		lineages := map[string]string{}
		if remaining == 0 && oldLineage != keptLineage {
			lineages[oldLineage] = keptLineage
		}
		// Aliases that pointed at the merged-away IDs follow them (pack.json and
		// origin.json written before an earlier merge).
		for _, a := range []struct {
			m    map[string]string
			kind string
		}{{instances, "instance"}, {lineages, "lineage"}} {
			for old, kept := range a.m {
				if _, err := tx.Exec(`UPDATE id_alias SET new_id=? WHERE new_id=? AND kind=?`, kept, old, a.kind); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(`UPDATE instance SET source_id=?, rel_path=?, real_path=?, residue=?, last_seen=?, status=?, tombstone_at=NULL, current_snapshot_id=?,
				game_id=coalesce((SELECT game_id FROM source WHERE id=?), game_id) WHERE id=?`,
			src, rel, real, residue, lastSeen, identity.Live, newSnap, src, q.CandidateID); err != nil {
			return err
		}
		// The answered question is resolved before the rewrite, which supersedes the
		// other open questions whose two sides become the same Instance.
		if _, err := tx.Exec(`UPDATE identity_question SET status='resolved', resolution='same', resolved_at=? WHERE id=?`, now.Unix(), q.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE identity_question SET status='resolved', resolution='superseded', resolved_at=? WHERE status='open' AND id<>? AND (instance_id=? OR candidate_instance_id=?)`,
			now.Unix(), q.ID, q.InstanceID, q.CandidateID); err != nil {
			return err
		}
		if err := remapHistoryStages(tx, instances, nil); err != nil {
			return err
		}
		if err := rewriteInstances(tx, instances); err != nil {
			return err
		}
		// The answered question keeps naming the Instance it asked about: its record shows
		// that Instance as gone, as before the shared rewrite.
		if _, err := tx.Exec(`UPDATE identity_question SET instance_id=? WHERE id=?`, q.InstanceID, q.ID); err != nil {
			return err
		}
		if err := rewriteOrigins(tx, instances, lineages); err != nil {
			return err
		}
		if len(lineages) > 0 {
			if err := rewriteLineages(tx, lineages); err != nil {
				return err
			}
			if err := mergePlayerData(tx, lineages); err != nil {
				return err
			}
			if err := mergeRules(tx, lineages); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO change_event(scan_id, instance_id, kind, before_snapshot, after_snapshot, detail) VALUES(?,?,?,?,?,?)`,
			q.ScanID, q.CandidateID, identity.EvReappeared, candSnap, newSnap, fmt.Sprintf("confirmed by player (q:%d)", q.ID)); err != nil {
			return err
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

// SplitLineage moves an Instance into a new lineage of its own, inheriting the player's
// alias, note and tags. Rules and finding ignores remain attached to the original lineage.
func (s *Store) SplitLineage(envID, instanceID, newLineageID string, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		var oldLineage string
		if err := tx.QueryRow(`SELECT lineage_id FROM instance WHERE id=?`, instanceID).Scan(&oldLineage); err != nil {
			return err
		}
		var members int
		if err := tx.QueryRow(`SELECT count(*) FROM instance WHERE lineage_id=?`, oldLineage).Scan(&members); err != nil {
			return err
		}
		if members < 2 {
			return fmt.Errorf("instance #%s is already alone in its lineage", instanceID)
		}
		if _, err := tx.Exec(`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) VALUES(?,?,?,?)`, newLineageID, Shared, instanceID, now.Unix()); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO lineage_data(environment_id, lineage_id, alias, note)
			SELECT environment_id, ?, alias, note FROM lineage_data WHERE lineage_id=?`, newLineageID, oldLineage); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO lineage_tag(environment_id, lineage_id, tag, pos)
			SELECT environment_id, ?, tag, pos FROM lineage_tag WHERE lineage_id=?`, newLineageID, oldLineage); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO lineage_functional(lineage_id, functional)
			SELECT ?, functional FROM lineage_functional WHERE lineage_id=?`, newLineageID, oldLineage); err != nil {
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

// GC removes tombstones that died before cutoff (04 §2.5) unless an open question refers to
// them, and drops superseded snapshots no event refers to. dryRun only counts.
func (s *Store) GC(gameID string, cutoff time.Time, dryRun bool) (*GCResult, error) {
	r := &GCResult{}
	err := s.inTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id, EXISTS(SELECT 1 FROM identity_question q WHERE q.status='open' AND (q.instance_id=i.id OR q.candidate_instance_id=i.id))
			FROM instance i WHERE game_id IN ('', ?) AND status=? AND tombstone_at < ? ORDER BY id`, gameID, identity.Tombstone, cutoff.Unix())
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
				`DELETE FROM snapshot_list WHERE snapshot_id IN (SELECT id FROM snapshot WHERE instance_id=?)`,
				`DELETE FROM snapshot WHERE instance_id=?`,
				`DELETE FROM instance_alias WHERE instance_id=?`,
				`DELETE FROM content_state WHERE instance_id=?`,
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
		orphan := `SELECT n.id FROM snapshot n JOIN instance i ON i.id=n.instance_id WHERE i.game_id IN ('', ?) AND n.id <> coalesce(i.current_snapshot_id, -1)
			AND NOT EXISTS(SELECT 1 FROM change_event e WHERE e.before_snapshot=n.id OR e.after_snapshot=n.id)
			AND NOT EXISTS(SELECT 1 FROM identity_question q WHERE q.observation_snapshot_id=n.id)`
		rows, err = tx.Query(orphan, gameID)
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
			n, err := deleteFiles(tx, `=?`, id)
			if err != nil {
				return err
			}
			r.Files += n
			if _, err := tx.Exec(`DELETE FROM snapshot WHERE id=?`, id); err != nil {
				return err
			}
		}
		if err := recomputePrimaries(tx, gameID); err != nil {
			return err
		}
		if err := pruneFacts(tx, time.Now()); err != nil {
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
