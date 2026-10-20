package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Workspace is the Loadout being edited (04 §1).
type Workspace struct {
	EnvID   string
	Entries string // loadout.List encoded
	// OriginVersion/OriginKind: where the workspace was last loaded from ("import",
	// "external", "checkout", "save"); a save right after a checkout is recorded as "undo".
	OriginVersion int
	OriginKind    string
	// Conflict is an external version that was not loaded because the workspace had
	// unsaved edits (04 §2.3); 0 when none.
	Conflict int
	Updated  time.Time
}

// Workspace returns the environment's workspace, or nil.
func (s *Store) Workspace(envID string) (*Workspace, error) {
	var w Workspace
	var up int64
	err := s.DB.QueryRow(`SELECT environment_id, entries, origin_version, origin_kind, conflict_version, updated_at FROM workspace WHERE environment_id=?`, envID).
		Scan(&w.EnvID, &w.Entries, &w.OriginVersion, &w.OriginKind, &w.Conflict, &up)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	w.Updated = time.Unix(up, 0).UTC()
	return &w, err
}

// SetWorkspace upserts the workspace.
func (s *Store) SetWorkspace(w *Workspace) error {
	_, err := s.DB.Exec(`INSERT INTO workspace(environment_id, entries, origin_version, origin_kind, conflict_version, updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(environment_id) DO UPDATE SET entries=excluded.entries, origin_version=excluded.origin_version,
		origin_kind=excluded.origin_kind, conflict_version=excluded.conflict_version, updated_at=excluded.updated_at`,
		w.EnvID, w.Entries, w.OriginVersion, w.OriginKind, w.Conflict, w.Updated.Unix())
	return err
}

// UndoLimit bounds each undo/redo stack.
const UndoLimit = 100

// PushStep pushes a workspace state onto the "undo" or "redo" stack.
func (s *Store) PushStep(envID, stack, entries, op string) error {
	if _, err := s.DB.Exec(`INSERT INTO workspace_undo(environment_id, stack, entries, op) VALUES(?,?,?,?)`, envID, stack, entries, op); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE environment_id=? AND stack=? AND id NOT IN
		(SELECT id FROM workspace_undo WHERE environment_id=? AND stack=? ORDER BY id DESC LIMIT ?)`, envID, stack, envID, stack, UndoLimit)
	return err
}

// PopStep pops the newest state of a stack; ok is false when the stack is empty.
func (s *Store) PopStep(envID, stack string) (entries, op string, ok bool, err error) {
	var id int64
	err = s.DB.QueryRow(`SELECT id, entries, op FROM workspace_undo WHERE environment_id=? AND stack=? ORDER BY id DESC LIMIT 1`, envID, stack).Scan(&id, &entries, &op)
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
func (s *Store) Steps(envID, stack string) ([]Step, error) {
	rows, err := s.DB.Query(`SELECT id, entries, op FROM workspace_undo WHERE environment_id=? AND stack=? ORDER BY id DESC`, envID, stack)
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
func (s *Store) LastStepID(envID, stack string) int64 {
	var id int64
	s.DB.QueryRow(`SELECT coalesce(max(id), 0) FROM workspace_undo WHERE environment_id=? AND stack=?`, envID, stack).Scan(&id)
	return id
}

// DeleteStepsAfter removes the states of a stack newer than id.
func (s *Store) DeleteStepsAfter(envID, stack string, id int64) error {
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE environment_id=? AND stack=? AND id > ?`, envID, stack, id)
	return err
}

// StepCount returns the size of a stack.
func (s *Store) StepCount(envID, stack string) int {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM workspace_undo WHERE environment_id=? AND stack=?`, envID, stack).Scan(&n)
	return n
}

// ClearSteps empties a stack.
func (s *Store) ClearSteps(envID, stack string) error {
	_, err := s.DB.Exec(`DELETE FROM workspace_undo WHERE environment_id=? AND stack=?`, envID, stack)
	return err
}

// Version is a saved Loadout version v<n> (04 §3).
type Version struct {
	N           int
	Created     time.Time
	Source      string // user | external | import | undo
	Entries     string
	GameVersion string // the version string the list was written with / read from
	Note        string
	Tags        []string
}

// AddVersion appends a version and returns its number.
func (s *Store) AddVersion(envID string, v *Version) (int, error) {
	var n int
	s.DB.QueryRow(`SELECT coalesce(max(n), 0) FROM loadout_version WHERE environment_id=?`, envID).Scan(&n)
	n++
	_, err := s.DB.Exec(`INSERT INTO loadout_version(environment_id, n, created_at, source, entries, game_version, note) VALUES(?,?,?,?,?,?,?)`,
		envID, n, v.Created.Unix(), v.Source, v.Entries, v.GameVersion, v.Note)
	v.N = n
	return n, err
}

// Versions lists all versions, newest first, with their tags.
func (s *Store) Versions(envID string) ([]*Version, error) {
	rows, err := s.DB.Query(`SELECT n, created_at, source, entries, game_version, note FROM loadout_version WHERE environment_id=? ORDER BY n DESC`, envID)
	if err != nil {
		return nil, err
	}
	var out []*Version
	byN := map[int]*Version{}
	for rows.Next() {
		var v Version
		var c int64
		if err := rows.Scan(&v.N, &c, &v.Source, &v.Entries, &v.GameVersion, &v.Note); err != nil {
			rows.Close()
			return nil, err
		}
		v.Created = time.Unix(c, 0).UTC()
		out = append(out, &v)
		byN[v.N] = &v
	}
	rows.Close()
	tags, err := s.DB.Query(`SELECT name, version_n FROM loadout_tag WHERE environment_id=? ORDER BY name`, envID)
	if err != nil {
		return nil, err
	}
	defer tags.Close()
	for tags.Next() {
		var name string
		var n int
		tags.Scan(&name, &n)
		if v := byN[n]; v != nil {
			v.Tags = append(v.Tags, name)
		}
	}
	return out, tags.Err()
}

// Version returns v<n>, or nil.
func (s *Store) Version(envID string, n int) (*Version, error) {
	vs, err := s.Versions(envID)
	if err != nil {
		return nil, err
	}
	for _, v := range vs {
		if v.N == n {
			return v, nil
		}
	}
	return nil, nil
}

// TagVersion names a version; an existing tag of the same name moves.
func (s *Store) TagVersion(envID, name string, n int) error {
	_, err := s.DB.Exec(`INSERT INTO loadout_tag(environment_id, name, version_n) VALUES(?,?,?)
		ON CONFLICT(environment_id, name) DO UPDATE SET version_n=excluded.version_n`, envID, name, n)
	return err
}

// TagVersionN resolves a tag name to a version number (0 if unknown).
func (s *Store) TagVersionN(envID, name string) int {
	var n int
	s.DB.QueryRow(`SELECT version_n FROM loadout_tag WHERE environment_id=? AND name=?`, envID, name).Scan(&n)
	return n
}

// ModsConfigState tracks what RimArcho last wrote and last saw in ModsConfig.xml.
type ModsConfigState struct {
	Written   []byte
	Seen      []byte
	WrittenAt time.Time
}

// ModsConfigState returns the tracking row (zero value if none).
func (s *Store) ModsConfigState(envID string) (*ModsConfigState, error) {
	var st ModsConfigState
	var at sql.NullInt64
	err := s.DB.QueryRow(`SELECT written_hash, seen_hash, written_at FROM modsconfig_state WHERE environment_id=?`, envID).Scan(&st.Written, &st.Seen, &at)
	if err == sql.ErrNoRows {
		return &st, nil
	}
	if at.Valid {
		st.WrittenAt = time.Unix(at.Int64, 0).UTC()
	}
	return &st, err
}

// SetModsConfigState upserts the tracking row.
func (s *Store) SetModsConfigState(envID string, st *ModsConfigState) error {
	var at any
	if !st.WrittenAt.IsZero() {
		at = st.WrittenAt.Unix()
	}
	_, err := s.DB.Exec(`INSERT INTO modsconfig_state(environment_id, written_hash, seen_hash, written_at) VALUES(?,?,?,?)
		ON CONFLICT(environment_id) DO UPDATE SET written_hash=excluded.written_hash, seen_hash=excluded.seen_hash, written_at=excluded.written_at`,
		envID, st.Written, st.Seen, at)
	return err
}

// UserRule is a stored user rule (04 §5.1); A and B are lineage IDs.
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
		r.ID, envID, r.Kind, r.A, r.B, r.Target, r.Note, r.Created.Unix()); err != nil {
		return 0, err
	}
	return r.ID, s.SetMeta("rule_seq", fmt.Sprint(r.ID))
}

// Rules lists an environment's user rules.
func (s *Store) Rules(envID string) ([]*UserRule, error) {
	rows, err := s.DB.Query(`SELECT id, kind, a_lineage, b_lineage, target, note, created_at FROM user_rule WHERE environment_id=? ORDER BY id`, envID)
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
	res, err := s.DB.Exec(`DELETE FROM user_rule WHERE environment_id=? AND id=?`, envID, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddLaunch records a game launch (04 §8).
func (s *Store) AddLaunch(envID string, at time.Time, method, command string) error {
	_, err := s.DB.Exec(`INSERT INTO launch(environment_id, started_at, method, command) VALUES(?,?,?,?)`, envID, at.Unix(), method, command)
	return err
}

// LastLaunch returns the newest launch time (zero if never).
func (s *Store) LastLaunch(envID string) time.Time {
	var t sql.NullInt64
	s.DB.QueryRow(`SELECT max(started_at) FROM launch WHERE environment_id=?`, envID).Scan(&t)
	if !t.Valid {
		return time.Time{}
	}
	return time.Unix(t.Int64, 0).UTC()
}

// AllAliases maps instance ID → alias locations for an environment.
func (s *Store) AllAliases(envID string) (map[string][]Alias, error) {
	rows, err := s.DB.Query(`SELECT a.instance_id, a.source_id, s.label, a.rel_path, a.real_path FROM instance_alias a
		JOIN source s ON s.id=a.source_id JOIN instance i ON i.id=a.instance_id WHERE i.environment_id=? ORDER BY s.label, a.rel_path`, envID)
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
