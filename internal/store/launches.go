package store

import (
	"database/sql"
	"time"
)

// LastListLaunch returns when the environment's own list was last launched: the newest
// launch that did not play a pack (zero if never). The home page sorts by it (06 §2.3).
func (s *Store) LastListLaunch(envID string) time.Time {
	var t sql.NullInt64
	s.DB.QueryRow(`SELECT max(started_at) FROM launch WHERE environment_id=? AND method NOT LIKE 'pack:%'`, envID).Scan(&t)
	if !t.Valid {
		return time.Time{}
	}
	return time.Unix(t.Int64, 0).UTC()
}
