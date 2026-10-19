package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Scan is one recorded scan.
type Scan struct {
	ID       int64
	EnvID    string
	Started  time.Time
	Finished time.Time
	Full     bool
	Stats    string
}

// Scans lists an environment's scans, newest first.
func (s *Store) Scans(envID string, limit int) ([]*Scan, error) {
	rows, err := s.DB.Query(`SELECT id, environment_id, started_at, coalesce(finished_at, 0), full, stats FROM scan
		WHERE environment_id=? ORDER BY id DESC LIMIT ?`, envID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Scan
	for rows.Next() {
		var sc Scan
		var st, fin int64
		var full int
		if err := rows.Scan(&sc.ID, &sc.EnvID, &st, &fin, &full, &sc.Stats); err != nil {
			return nil, err
		}
		sc.Started, sc.Full = time.Unix(st, 0).UTC(), full != 0
		if fin != 0 {
			sc.Finished = time.Unix(fin, 0).UTC()
		}
		out = append(out, &sc)
	}
	return out, rows.Err()
}

// ScanByID returns a scan, or nil.
func (s *Store) ScanByID(id int64) (*Scan, error) {
	var sc Scan
	var st, fin int64
	var full int
	err := s.DB.QueryRow(`SELECT id, environment_id, started_at, coalesce(finished_at, 0), full, stats FROM scan WHERE id=?`, id).
		Scan(&sc.ID, &sc.EnvID, &st, &fin, &full, &sc.Stats)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sc.Started, sc.Full = time.Unix(st, 0).UTC(), full != 0
	if fin != 0 {
		sc.Finished = time.Unix(fin, 0).UTC()
	}
	return &sc, nil
}

// DecodeStats unmarshals a scan's stats JSON into v.
func (sc *Scan) DecodeStats(v any) { json.Unmarshal([]byte(sc.Stats), v) }

// Event is a stored change event.
type Event struct {
	ID         int64
	ScanID     int64
	ScanTime   time.Time
	InstanceID string
	Kind       string
	Before     int64
	After      int64
	Detail     string
}

// EventFilter selects change events.
type EventFilter struct {
	EnvID       string
	InstanceIDs []string // empty = all
	MinScan     int64    // inclusive
	MaxScan     int64    // inclusive, 0 = no bound
	SinceTime   time.Time
}

// Events returns matching change events, oldest scan first, in insertion order.
func (s *Store) Events(f EventFilter) ([]*Event, error) {
	q := `SELECT e.id, e.scan_id, sc.started_at, e.instance_id, e.kind, coalesce(e.before_snapshot, 0), coalesce(e.after_snapshot, 0), e.detail
		FROM change_event e JOIN scan sc ON sc.id = e.scan_id WHERE sc.environment_id=? AND e.scan_id >= ?`
	args := []any{f.EnvID, f.MinScan}
	if f.MaxScan != 0 {
		q += ` AND e.scan_id <= ?`
		args = append(args, f.MaxScan)
	}
	if !f.SinceTime.IsZero() {
		q += ` AND sc.started_at >= ?`
		args = append(args, f.SinceTime.Unix())
	}
	if len(f.InstanceIDs) > 0 {
		q += ` AND e.instance_id IN (` + placeholders(len(f.InstanceIDs)) + `)`
		for _, id := range f.InstanceIDs {
			args = append(args, id)
		}
	}
	q += ` ORDER BY e.scan_id, e.id`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Event
	for rows.Next() {
		var e Event
		var t int64
		if err := rows.Scan(&e.ID, &e.ScanID, &t, &e.InstanceID, &e.Kind, &e.Before, &e.After, &e.Detail); err != nil {
			return nil, err
		}
		e.ScanTime = time.Unix(t, 0).UTC()
		out = append(out, &e)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	b := make([]byte, 0, 2*n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}

// Question is a stored identity question.
type Question struct {
	ID          int64
	EnvID       string
	ScanID      int64
	InstanceID  string
	SnapshotID  int64
	CandidateID string
	Score       float64
	Status      string // open | resolved
	Resolution  string // same | copy | unrelated
	ResolvedAt  time.Time
}

// Questions lists an environment's identity questions (open first unless all is false).
func (s *Store) Questions(envID string, all bool) ([]*Question, error) {
	q := `SELECT id, environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id, score, status, resolution,
		coalesce(resolved_at, 0) FROM identity_question WHERE environment_id=?`
	if !all {
		q += ` AND status='open'`
	}
	q += ` ORDER BY id`
	rows, err := s.DB.Query(q, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Question
	for rows.Next() {
		var x Question
		var ra int64
		if err := rows.Scan(&x.ID, &x.EnvID, &x.ScanID, &x.InstanceID, &x.SnapshotID, &x.CandidateID, &x.Score, &x.Status, &x.Resolution, &ra); err != nil {
			return nil, err
		}
		if ra != 0 {
			x.ResolvedAt = time.Unix(ra, 0).UTC()
		}
		out = append(out, &x)
	}
	return out, rows.Err()
}

// QuestionByID returns a question, or nil.
func (s *Store) QuestionByID(id int64) (*Question, error) {
	var x Question
	var ra int64
	err := s.DB.QueryRow(`SELECT id, environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id, score, status, resolution,
		coalesce(resolved_at, 0) FROM identity_question WHERE id=?`, id).
		Scan(&x.ID, &x.EnvID, &x.ScanID, &x.InstanceID, &x.SnapshotID, &x.CandidateID, &x.Score, &x.Status, &x.Resolution, &ra)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if ra != 0 {
		x.ResolvedAt = time.Unix(ra, 0).UTC()
	}
	return &x, err
}

// Lineage is a stored lineage.
type Lineage struct {
	ID        string
	PrimaryID string
	CreatedAt time.Time
}

// Lineages returns an environment's lineages keyed by ID.
func (s *Store) Lineages(envID string) (map[string]*Lineage, error) {
	rows, err := s.DB.Query(`SELECT id, coalesce(primary_instance_id, ''), created_at FROM lineage WHERE environment_id=?`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*Lineage{}
	for rows.Next() {
		var l Lineage
		var c int64
		if err := rows.Scan(&l.ID, &l.PrimaryID, &c); err != nil {
			return nil, err
		}
		l.CreatedAt = time.Unix(c, 0).UTC()
		out[l.ID] = &l
	}
	return out, rows.Err()
}
