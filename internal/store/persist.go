package store

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
)

// BeginScan records the start of a scan and returns its ID.
func (s *Store) BeginScan(envID string, started time.Time, full bool) (int64, error) {
	f := 0
	if full {
		f = 1
	}
	res, err := s.DB.Exec(`INSERT INTO scan(environment_id, started_at, full) VALUES(?,?,?)`, envID, started.Unix(), f)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishScan records the end of a scan with its statistics (JSON).
func (s *Store) FinishScan(scanID int64, finished time.Time, stats any) error {
	b, _ := json.Marshal(stats)
	_, err := s.DB.Exec(`UPDATE scan SET finished_at=?, stats=? WHERE id=?`, finished.Unix(), string(b), scanID)
	return err
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// insertSnapshot writes a snapshot and its manifest; returns the snapshot ID. A released
// manifest (unchanged content, new external state) is copied from prevSnap.
func insertSnapshot(tx *sql.Tx, instanceID string, scanID int64, ob *scan.Observation, prevSnap int64) (int64, error) {
	d := ob.Decl
	blob, _ := json.Marshal(d)
	origin := ""
	if ob.Origin != nil {
		b, _ := json.Marshal(ob.Origin)
		origin = string(b)
	}
	var manifest string
	var acfTime, acfSize int64
	if ob.ACF != nil {
		manifest, acfTime, acfSize = ob.ACF.Manifest, ob.ACF.TimeUpdated, ob.ACF.Size
	}
	h := ob.FP.SnapshotHash
	res, err := tx.Exec(`INSERT INTO snapshot(instance_id, scan_id, snapshot_hash, package_id, package_id_raw, name, authors,
		workshop_id_declared, supported_versions, decl_blob, acf_manifest, acf_time_updated, acf_size, git_head, origin,
		file_count, total_size, semantic_count) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		instanceID, scanID, h[:], d.PackageID, d.PackageIDRaw, d.Name, strings.Join(d.Authors, ", "), d.PublishedFileID,
		strings.Join(d.SupportedVersions, " "), string(blob), manifest, acfTime, acfSize, ob.GitHead, origin,
		ob.FP.Count, ob.FP.TotalSize, len(ob.FP.Semantic))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if ob.FP.Files == nil && ob.FP.Count > 0 {
		_, err = tx.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash)
			SELECT ?, rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=?`, id, prevSnap)
		return id, err
	}
	return id, insertFiles(tx, id, ob.FP.Files)
}

func insertFiles(tx *sql.Tx, snapID int64, files []scan.File) error {
	st, err := tx.Prepare(`INSERT OR REPLACE INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, f := range files {
		var h any
		if f.HasHash {
			h = f.Hash[:]
		}
		if _, err := st.Exec(snapID, f.Rel, f.Size, f.MTime, h); err != nil {
			return err
		}
	}
	return nil
}

// ApplyScan persists one reconciliation result in a single transaction.
func (s *Store) ApplyScan(envID string, scanID int64, now time.Time, res *identity.Result) (err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	ts := now.Unix()
	snapOf := map[*identity.Assignment]int64{}
	for _, a := range res.Assignments {
		ob := a.Obs
		if a.NewLineage {
			if _, err = tx.Exec(`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) VALUES(?,?,?,?)`,
				a.LineageID, envID, a.InstanceID, ts); err != nil {
				return err
			}
		}
		residue := 0
		if ob.Residue {
			residue = 1
		}
		unloadable := ob.Unloadable
		if a.Known == nil {
			if _, err = tx.Exec(`INSERT INTO instance(id, environment_id, lineage_id, lineage_basis, source_id, rel_path, real_path, status,
				residue, unloadable, first_seen, last_seen) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				a.InstanceID, envID, a.LineageID, a.LineageBasis, ob.Loc.SourceID, ob.Loc.Rel, ob.RealPath, identity.Live, residue, unloadable, ts, ts); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(`UPDATE instance SET source_id=?, rel_path=?, real_path=?, status=?, residue=?, unloadable=?, last_seen=?, tombstone_at=NULL WHERE id=?`,
				ob.Loc.SourceID, ob.Loc.Rel, ob.RealPath, identity.Live, residue, unloadable, ts, a.InstanceID); err != nil {
				return err
			}
		}
		var snapID int64
		switch {
		case a.NeedSnapshot:
			var prevSnap int64
			if a.Prev != nil {
				prevSnap = a.Prev.ID
			}
			if snapID, err = insertSnapshot(tx, a.InstanceID, scanID, ob, prevSnap); err != nil {
				return err
			}
			if _, err = tx.Exec(`UPDATE instance SET current_snapshot_id=? WHERE id=?`, snapID, a.InstanceID); err != nil {
				return err
			}
		default:
			snapID = a.Prev.ID
			if ob.FP.Hashed > 0 && ob.FP.Files != nil {
				// Same content, newer mtimes: refresh the cache so the next scan can skip hashing.
				if _, err = tx.Exec(`DELETE FROM snapshot_file WHERE snapshot_id=?`, snapID); err != nil {
					return err
				}
				if err = insertFiles(tx, snapID, ob.FP.Files); err != nil {
					return err
				}
			}
		}
		snapOf[a] = snapID
		if _, err = tx.Exec(`DELETE FROM instance_alias WHERE instance_id=?`, a.InstanceID); err != nil {
			return err
		}
		for _, al := range ob.Aliases {
			if _, err = tx.Exec(`INSERT OR REPLACE INTO instance_alias(instance_id, source_id, rel_path, real_path) VALUES(?,?,?,?)`,
				a.InstanceID, al.SourceID, al.Rel, ob.RealPath); err != nil {
				return err
			}
		}
	}
	for _, k := range res.Disappeared {
		if _, err = tx.Exec(`UPDATE instance SET status=?, tombstone_at=? WHERE id=?`, identity.Tombstone, ts, k.ID); err != nil {
			return err
		}
	}
	for _, e := range res.Events {
		var before, after any
		if e.Before != 0 {
			before = e.Before
		}
		if e.After != nil {
			after = snapOf[e.After]
		}
		if _, err = tx.Exec(`INSERT INTO change_event(scan_id, instance_id, kind, before_snapshot, after_snapshot, detail) VALUES(?,?,?,?,?,?)`,
			scanID, e.InstanceID, e.Kind, before, after, e.Detail); err != nil {
			return err
		}
	}
	for _, q := range res.Questions {
		if _, err = tx.Exec(`INSERT INTO identity_question(environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id, score)
			VALUES(?,?,?,?,?,?)`, envID, scanID, q.New.InstanceID, snapOf[q.New], q.Candidate.ID, q.Score); err != nil {
			return err
		}
	}
	if err = recomputePrimaries(tx, envID); err != nil {
		return err
	}
	return tx.Commit()
}

// recomputePrimaries sets each lineage's primary instance (02 §6): a live instance before a
// tombstone, then a workshop-like source, then the earliest first_seen. Empty lineages are
// deleted.
func recomputePrimaries(tx *sql.Tx, envID string) error {
	rows, err := tx.Query(`SELECT i.id, i.lineage_id, i.status, s.kind, i.first_seen FROM instance i JOIN source s ON s.id=i.source_id
		WHERE i.environment_id=?`, envID)
	if err != nil {
		return err
	}
	type row struct {
		id, lineage, status string
		kind            source.Kind
		first           int64
	}
	byLineage := map[string][]row{}
	for rows.Next() {
		var r row
		var kind string
		if err := rows.Scan(&r.id, &r.lineage, &r.status, &kind, &r.first); err != nil {
			rows.Close()
			return err
		}
		r.kind = source.Kind(kind)
		byLineage[r.lineage] = append(byLineage[r.lineage], r)
	}
	rows.Close()
	for lin, rs := range byLineage {
		sort.Slice(rs, func(i, j int) bool {
			a, b := rs[i], rs[j]
			if (a.status == identity.Live) != (b.status == identity.Live) {
				return a.status == identity.Live
			}
			if a.kind.WorkshopLike() != b.kind.WorkshopLike() {
				return a.kind.WorkshopLike()
			}
			if a.first != b.first {
				return a.first < b.first
			}
			return a.id < b.id
		})
		if _, err := tx.Exec(`UPDATE lineage SET primary_instance_id=? WHERE id=?`, rs[0].id, lin); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM lineage WHERE environment_id=? AND id NOT IN (SELECT lineage_id FROM instance WHERE environment_id=?)`, envID, envID)
	return err
}

// RecomputePrimaries is recomputePrimaries in its own transaction.
func (s *Store) RecomputePrimaries(envID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if err := recomputePrimaries(tx, envID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
