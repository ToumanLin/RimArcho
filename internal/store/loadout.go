package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Workspace is the list of a plan being edited (04 §1.2).
type Workspace struct {
	PlanID  int64
	Entries string // loadout.List encoded
	// OriginVersion/OriginKind: where the workspace was last loaded from ("import",
	// "external", "checkout", "edited", "save"); a save right after a checkout is recorded
	// as "undo".
	OriginVersion int
	OriginKind    string
	// Conflict is an external version that was not loaded because the workspace had
	// unsaved edits (04 §4.3); 0 when none.
	Conflict int
	Updated  time.Time
}

// Workspace returns a plan's workspace, or nil.
func (s *Store) Workspace(planID int64) (*Workspace, error) {
	var w Workspace
	var up int64
	err := s.DB.QueryRow(`SELECT plan_id, entries, origin_version, origin_kind, conflict_version, updated_at FROM workspace WHERE plan_id=?`, planID).
		Scan(&w.PlanID, &w.Entries, &w.OriginVersion, &w.OriginKind, &w.Conflict, &up)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	w.Updated = time.Unix(up, 0).UTC()
	return &w, err
}

// SetWorkspace upserts the workspace; the plan's "last changed" time follows it.
func (s *Store) SetWorkspace(w *Workspace) error {
	if w.PlanID == 0 {
		return fmt.Errorf("store: workspace without a plan")
	}
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspace(plan_id, entries, origin_version, origin_kind, conflict_version, updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(plan_id) DO UPDATE SET entries=excluded.entries, origin_version=excluded.origin_version,
		origin_kind=excluded.origin_kind, conflict_version=excluded.conflict_version, updated_at=excluded.updated_at`,
			w.PlanID, w.Entries, w.OriginVersion, w.OriginKind, w.Conflict, w.Updated.Unix()); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE plan SET updated_at=max(updated_at, ?) WHERE id=?`, w.Updated.Unix(), w.PlanID)
		return err
	})
}

// UndoLimit bounds each undo/redo stack.
const UndoLimit = 100

// PushStep pushes a workspace state onto a plan's "undo" or "redo" stack.
func (s *Store) PushStep(planID int64, stack, entries, op string) error {
	if _, err := s.DB.Exec(`INSERT INTO workspace_undo(plan_id, stack, entries, op) VALUES(?,?,?,?)`, planID, stack, entries, op); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE plan_id=? AND stack=? AND id NOT IN
		(SELECT id FROM workspace_undo WHERE plan_id=? AND stack=? ORDER BY id DESC LIMIT ?)`, planID, stack, planID, stack, UndoLimit)
	return err
}

// PopStep pops the newest state of a stack; ok is false when the stack is empty.
func (s *Store) PopStep(planID int64, stack string) (entries, op string, ok bool, err error) {
	var id int64
	err = s.DB.QueryRow(`SELECT id, entries, op FROM workspace_undo WHERE plan_id=? AND stack=? ORDER BY id DESC LIMIT 1`, planID, stack).Scan(&id, &entries, &op)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	_, err = s.DB.Exec(`DELETE FROM workspace_undo WHERE id=?`, id)
	return entries, op, true, err
}

// Step is one stored workspace state of an undo or redo stack.
type Step struct {
	ID      int64
	Entries string
	Op      string
}

// Steps lists a stack, newest first.
func (s *Store) Steps(planID int64, stack string) ([]Step, error) {
	rows, err := s.DB.Query(`SELECT id, entries, op FROM workspace_undo WHERE plan_id=? AND stack=? ORDER BY id DESC`, planID, stack)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Step
	for rows.Next() {
		var x Step
		if err := rows.Scan(&x.ID, &x.Entries, &x.Op); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LastStepID is the id of the newest state of a stack (0 when empty).
func (s *Store) LastStepID(planID int64, stack string) int64 {
	var id int64
	s.DB.QueryRow(`SELECT coalesce(max(id), 0) FROM workspace_undo WHERE plan_id=? AND stack=?`, planID, stack).Scan(&id)
	return id
}

// DeleteStepsAfter removes the states of a stack newer than id.
func (s *Store) DeleteStepsAfter(planID int64, stack string, id int64) error {
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE plan_id=? AND stack=? AND id > ?`, planID, stack, id)
	return err
}

// StepCount returns the size of a stack.
func (s *Store) StepCount(planID int64, stack string) int {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM workspace_undo WHERE plan_id=? AND stack=?`, planID, stack).Scan(&n)
	return n
}

// ClearSteps empties a stack.
func (s *Store) ClearSteps(planID int64, stack string) error {
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE plan_id=? AND stack=?`, planID, stack)
	return err
}

// Version is a saved version v<n> of a plan (04 §3); numbers count per plan.
type Version struct {
	N           int
	Created     time.Time
	Source      string // import | external | user | undo | launch
	Entries     string
	GameVersion string // the version string the list was written with / read from
	Note        string
}

// AddVersion appends a version to a plan and returns its number.
func (s *Store) AddVersion(planID int64, v *Version) (int, error) {
	var n int
	s.DB.QueryRow(`SELECT coalesce(max(n), 0) FROM loadout_version WHERE plan_id=?`, planID).Scan(&n)
	n++
	_, err := s.DB.Exec(`INSERT INTO loadout_version(plan_id, n, created_at, source, entries, game_version, note) VALUES(?,?,?,?,?,?,?)`,
		planID, n, v.Created.Unix(), v.Source, v.Entries, v.GameVersion, v.Note)
	v.N = n
	return n, err
}

const versionCols = `n, created_at, source, entries, game_version, note`

func scanVersion(row interface{ Scan(...any) error }) (*Version, error) {
	var v Version
	var c int64
	if err := row.Scan(&v.N, &c, &v.Source, &v.Entries, &v.GameVersion, &v.Note); err != nil {
		return nil, err
	}
	v.Created = time.Unix(c, 0).UTC()
	return &v, nil
}

// Versions lists a plan's versions, newest first.
func (s *Store) Versions(planID int64) ([]*Version, error) {
	rows, err := s.DB.Query(`SELECT `+versionCols+` FROM loadout_version WHERE plan_id=? ORDER BY n DESC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Version returns v<n> of a plan, or nil.
func (s *Store) Version(planID int64, n int) (*Version, error) {
	v, err := scanVersion(s.DB.QueryRow(`SELECT `+versionCols+` FROM loadout_version WHERE plan_id=? AND n=?`, planID, n))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

// LatestVersion returns a plan's newest version, or nil.
func (s *Store) LatestVersion(planID int64) (*Version, error) {
	v, err := scanVersion(s.DB.QueryRow(`SELECT `+versionCols+` FROM loadout_version WHERE plan_id=? ORDER BY n DESC LIMIT 1`, planID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

// VersionCount is the number of versions of a plan.
func (s *Store) VersionCount(planID int64) int {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM loadout_version WHERE plan_id=?`, planID).Scan(&n)
	return n
}

// UserRule is a stored user rule (04 §6.1); A and B are lineage IDs.
type UserRule struct {
	ID      int64
	Kind    string
	A, B    string
	Target  string
	Note    string
	Created time.Time
}

// AddRule stores a rule and returns its ID. IDs are never reused (r:<n> stays unambiguous
// in notes and scripts after a rule is removed).
func (s *Store) AddRule(envID string, r *UserRule) (int64, error) {
	var seq, maxID int64
	fmt.Sscan(s.Meta("rule_seq"), &seq)
	s.DB.QueryRow(`SELECT coalesce(max(id), 0) FROM user_rule`).Scan(&maxID)
	r.ID = max(seq, maxID) + 1
	if _, err := s.DB.Exec(`INSERT INTO user_rule(id, environment_id, kind, a_lineage, b_lineage, target, note, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		r.ID, Shared, r.Kind, r.A, r.B, r.Target, r.Note, r.Created.Unix()); err != nil {
		return 0, err
	}
	return r.ID, s.SetMeta("rule_seq", fmt.Sprint(r.ID))
}

// Rules lists the user rules. They are global (02 §0 item 6): envID is kept for the callers.
func (s *Store) Rules(envID string) ([]*UserRule, error) {
	rows, err := s.DB.Query(`SELECT id, kind, a_lineage, b_lineage, target, note, created_at FROM user_rule ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserRule
	for rows.Next() {
		var r UserRule
		var c int64
		if err := rows.Scan(&r.ID, &r.Kind, &r.A, &r.B, &r.Target, &r.Note, &c); err != nil {
			return nil, err
		}
		r.Created = time.Unix(c, 0).UTC()
		out = append(out, &r)
	}
	return out, rows.Err()
}

// RemoveRule deletes a rule; it reports whether one was deleted.
func (s *Store) RemoveRule(envID string, id int64) (bool, error) {
	res, err := s.DB.Exec(`DELETE FROM user_rule WHERE id=?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AllAliases maps instance ID → alias locations for an environment.
func (s *Store) AllAliases(envID string) (map[string][]Alias, error) {
	rows, err := s.DB.Query(`SELECT a.instance_id, a.source_id, s.label, a.rel_path, a.real_path FROM instance_alias a
		JOIN source s ON s.id=a.source_id JOIN instance i ON i.id=a.instance_id WHERE i.environment_id IN ('', ?) ORDER BY s.label, a.rel_path`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Alias{}
	for rows.Next() {
		var id string
		var a Alias
		if err := rows.Scan(&id, &a.SourceID, &a.SourceLabel, &a.Rel, &a.RealPath); err != nil {
			return nil, err
		}
		out[id] = append(out[id], a)
	}
	return out, rows.Err()
}

// LaunchBefore returns the newest launch at or before t: its method ("steam", "direct",
// "pack:<name>") and time; "" when there is none.
func (s *Store) LaunchBefore(envID string, t time.Time) (string, time.Time) {
	var m sql.NullString
	var at sql.NullInt64
	s.DB.QueryRow(`SELECT method, started_at FROM launch WHERE environment_id=? AND started_at<=? ORDER BY started_at DESC, rowid DESC LIMIT 1`, envID, t.Unix()).Scan(&m, &at)
	if !m.Valid || !at.Valid {
		return "", time.Time{}
	}
	return m.String, time.Unix(at.Int64, 0).UTC()
}
