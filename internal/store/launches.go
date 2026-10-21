package store

import (
	"database/sql"
	"strings"
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

// LaunchRecordBefore returns the newest launch of any of the environments that started at
// or before t (nil if none). Logs map to it by their modification time (03 §6.4), and a
// running game by its start (08 §4).
func (s *Store) LaunchRecordBefore(envIDs []string, t time.Time) (*LaunchRecord, error) {
	if len(envIDs) == 0 {
		return nil, nil
	}
	q := `SELECT id, environment_id, coalesce(plan_id, 0), started_at, method, command, fingerprint, version_n FROM launch
		WHERE started_at<=? AND environment_id IN (?` + strings.Repeat(",?", len(envIDs)-1) + `) ORDER BY started_at DESC, id DESC LIMIT 1`
	args := []any{t.Unix()}
	for _, id := range envIDs {
		args = append(args, id)
	}
	var r LaunchRecord
	var at int64
	err := s.DB.QueryRow(q, args...).Scan(&r.ID, &r.EnvID, &r.PlanID, &at, &r.Method, &r.Command, &r.Fingerprint, &r.VersionN)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Started = time.Unix(at, 0).UTC()
	return &r, nil
}

// Pack is the pack a launch played ("" for a plan's launch).
func (r *LaunchRecord) Pack() string {
	name, _ := strings.CutPrefix(r.Method, "pack:")
	if name == r.Method {
		return ""
	}
	return name
}
