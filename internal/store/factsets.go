package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// ForeignAnalysisRetention is the idle lifetime of slots for a non-current version.
const ForeignAnalysisRetention = 30 * 24 * time.Hour

// PruneFacts removes unused version views and their unreferenced fact sets (03 §3).
func (s *Store) PruneFacts(now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error { return pruneFacts(tx, now) })
}

func pruneFacts(tx *sql.Tx, now time.Time) error {
	type game struct{ id, mm string }
	var games []game
	rows, err := tx.Query(`SELECT id, game_version FROM environment`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g game
		var v string
		if err := rows.Scan(&g.id, &v); err != nil {
			rows.Close()
			return err
		}
		g.mm = env.MajorMinor(v)
		games = append(games, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Unknown game versions are conservative: no time-based expiry until detected.
	for _, g := range games {
		if g.mm == "" {
			continue
		}
		for _, table := range []string{"patch_fact", "finding"} {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE analysis_id IN (SELECT id FROM analysis WHERE environment_id=? AND game_mm<>? AND finished_at<?)`, g.id, g.mm, now.Add(-ForeignAnalysisRetention).Unix()); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM analysis WHERE environment_id=? AND game_mm<>? AND finished_at<?`, g.id, g.mm, now.Add(-ForeignAnalysisRetention).Unix()); err != nil {
			return err
		}
	}
	// Materialize the small set of needed (visibility owner, mm) pairs once. Shared
	// instances can use any environment; private instances only their owning one.
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS needed_fact_version(environment_id TEXT NOT NULL, game_mm TEXT NOT NULL, PRIMARY KEY(environment_id,game_mm)) WITHOUT ROWID; DELETE FROM needed_fact_version`); err != nil {
		return err
	}
	for _, g := range games {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO needed_fact_version VALUES(?,?)`, g.id, g.mm); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO needed_fact_version SELECT environment_id,game_mm FROM analysis`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM content_state WHERE NOT EXISTS (
 SELECT 1 FROM instance i WHERE i.id=content_state.instance_id AND EXISTS (
 SELECT 1 FROM needed_fact_version n WHERE (i.environment_id='' OR n.environment_id=i.environment_id)
 AND (n.game_mm=content_state.game_mm OR n.game_mm='')))`); err != nil {
		return err
	}
	return pruneFactSets(tx)
}

func pruneFactSets(tx *sql.Tx) error {
	// The child tables have set_id indexes for these cascading deletes.
	_, err := tx.Exec(`DELETE FROM fact_set WHERE NOT EXISTS(SELECT 1 FROM content_state c WHERE c.def_set_id=fact_set.id)
 AND NOT EXISTS(SELECT 1 FROM content_state c WHERE c.dll_set_id=fact_set.id)`)
	return err
}

func ensureFactSet(tx *sql.Tx, instance, kind, key string) (id int64, fresh bool, err error) {
	res, err := tx.Exec(`INSERT INTO fact_set(instance_id,kind,content_key) VALUES(?,?,?) ON CONFLICT DO NOTHING`, instance, kind, key)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	err = tx.QueryRow(`SELECT id FROM fact_set WHERE instance_id=? AND kind=? AND content_key=?`, instance, kind, key).Scan(&id)
	return id, n > 0, err
}

// deleteInstanceFacts also supports migrations that predate fact sets.
func deleteInstanceFacts(tx *sql.Tx, instance string) error {
	var modern int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='fact_set'`).Scan(&modern); err != nil {
		return err
	}
	if modern == 0 {
		for _, table := range []string{"def_fact", "assembly_fact", "type_fact", "harmony_fact", "harmony_id"} {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE instance_id=?`, instance); err != nil {
				return fmt.Errorf("delete %s: %w", table, err)
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM content_state WHERE instance_id=?`, instance); err != nil {
		return err
	}
	if modern != 0 {
		return pruneFactSets(tx)
	}
	return nil
}
