package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
)

// RecordRemoved publishes a completed file operation without walking every source.
// Filesystem work must finish before this short transaction begins.
func (s *Store) RecordRemoved(id, path string, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		var envID, lineage string
		var snapshot int64
		if err := tx.QueryRow(`SELECT environment_id,lineage_id,coalesce(current_snapshot_id,0) FROM instance WHERE id=? AND real_path=?`, id, path).Scan(&envID, &lineage, &snapshot); err != nil {
			return err
		}
		r, err := tx.Exec(`UPDATE instance SET status=?,tombstone_at=? WHERE id=? AND real_path=? AND status=?`, identity.Tombstone, now.Unix(), id, path, identity.Live)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("instance changed during removal: %s", id)
		}
		if _, err := tx.Exec(`DELETE FROM instance_alias WHERE instance_id=?`, id); err != nil {
			return err
		}
		scan, err := tx.Exec(`INSERT INTO scan(environment_id,started_at,finished_at,stats) VALUES(?,?,?,?)`, envID, now.Unix(), now.Unix(), `{"events":{"disappeared":1}}`)
		if err != nil {
			return err
		}
		scanID, err := scan.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO change_event(scan_id,instance_id,kind,before_snapshot) VALUES(?,?,?,?)`, scanID, id, identity.EvDisappeared, snapshot); err != nil {
			return err
		}
		return recomputePrimary(tx, lineage)
	})
}
