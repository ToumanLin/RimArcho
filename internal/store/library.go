package store

import (
	"time"
)

// LastChanged maps each instance of an environment to the start of the newest scan that
// recorded a change for it (Mod 库存's "最近变化" sort, 06 §2.9).
func (s *Store) LastChanged(envID string) (map[string]time.Time, error) {
	rows, err := s.DB.Query(`SELECT c.instance_id, max(s.started_at) FROM change_event c JOIN scan s ON s.id=c.scan_id
		WHERE s.environment_id=? GROUP BY c.instance_id`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = time.Unix(at, 0).UTC()
	}
	return out, rows.Err()
}

// RelocateInstance points an instance at a new folder (a move RimArcho made itself, 06 §2.9):
// the next scan finds it there by source and path (R1), so its ID, lineage, history and
// every plan's reference stay the same.
func (s *Store) RelocateInstance(id, sourceID, rel, realPath string) error {
	_, err := s.DB.Exec(`UPDATE instance SET source_id=?, rel_path=?, real_path=? WHERE id=?`, sourceID, rel, realPath, id)
	return err
}
