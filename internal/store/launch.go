package store

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ModsConfigState is the write record of one user data folder's ModsConfig.xml (04 §4.3):
// what RimArcho last wrote and last saw there, and who wrote it. Several environments can
// share a folder (02 §2.4), so the record belongs to the folder, not to an environment.
type ModsConfigState struct {
	Written   []byte
	Seen      []byte
	WrittenAt time.Time
	// WriterPlan and WriterVersion name the plan and its version v<n> last written to the
	// file (0 when unknown).
	WriterPlan    int64
	WriterVersion int
}

// UserDataKey normalises a user data folder for the write record (case-insensitive on
// Windows).
func UserDataKey(root string) string {
	if root == "" {
		return ""
	}
	k := filepath.Clean(root)
	if runtime.GOOS == "windows" {
		k = strings.ToLower(filepath.FromSlash(k))
	}
	return k
}

// ModsConfigStateAt returns the write record of a user data folder (zero value if none).
func (s *Store) ModsConfigStateAt(userDataRoot string) (*ModsConfigState, error) {
	var st ModsConfigState
	var at, plan sql.NullInt64
	err := s.DB.QueryRow(`SELECT written_hash, seen_hash, written_at, writer_plan, writer_version FROM modsconfig_state WHERE user_data_root=?`,
		UserDataKey(userDataRoot)).Scan(&st.Written, &st.Seen, &at, &plan, &st.WriterVersion)
	if err == sql.ErrNoRows {
		return &st, nil
	}
	if at.Valid {
		st.WrittenAt = time.Unix(at.Int64, 0).UTC()
	}
	st.WriterPlan = plan.Int64
	return &st, err
}

// SetModsConfigStateAt upserts the write record of a user data folder. An empty folder
// (an environment without user data) has no record and is ignored.
func (s *Store) SetModsConfigStateAt(userDataRoot string, st *ModsConfigState) error {
	key := UserDataKey(userDataRoot)
	if key == "" {
		return nil
	}
	var at, plan any
	if !st.WrittenAt.IsZero() {
		at = st.WrittenAt.Unix()
	}
	if st.WriterPlan != 0 {
		plan = st.WriterPlan
	}
	_, err := s.DB.Exec(`INSERT INTO modsconfig_state(user_data_root, written_hash, seen_hash, written_at, writer_plan, writer_version) VALUES(?,?,?,?,?,?)
		ON CONFLICT(user_data_root) DO UPDATE SET written_hash=excluded.written_hash, seen_hash=excluded.seen_hash, written_at=excluded.written_at,
		writer_plan=excluded.writer_plan, writer_version=excluded.writer_version`,
		key, st.Written, st.Seen, at, plan, st.WriterVersion)
	return err
}

func (s *Store) envUserData(envID string) string {
	var root string
	s.DB.QueryRow(`SELECT user_data_root FROM environment WHERE id=?`, envID).Scan(&root)
	return root
}

// ModsConfigState returns the write record of the environment's user data folder.
//
// Deprecated: the record belongs to the folder (ModsConfigStateAt).
func (s *Store) ModsConfigState(envID string) (*ModsConfigState, error) {
	return s.ModsConfigStateAt(s.envUserData(envID))
}

// SetModsConfigState writes the record of the environment's user data folder.
//
// Deprecated: use SetModsConfigStateAt and set the writer.
func (s *Store) SetModsConfigState(envID string, st *ModsConfigState) error {
	return s.SetModsConfigStateAt(s.envUserData(envID), st)
}

// LaunchRecord is one game launch (08 §1): a plan's (PlanID set) or a pack's (method
// "pack:<name>", no plan).
type LaunchRecord struct {
	ID          int64
	EnvID       string
	PlanID      int64 // 0 for packs and launches recorded before plans existed
	Started     time.Time
	Method      string // steam | direct | pack:<name>
	Command     string
	Fingerprint string // the enabled list's fingerprint
	VersionN    int    // the plan's version v<n> that was launched (0 when unknown)
}

// AddLaunchRecord stores a launch and sets its ID.
func (s *Store) AddLaunchRecord(r *LaunchRecord) error {
	var plan any
	if r.PlanID != 0 {
		plan = r.PlanID
	}
	res, err := s.DB.Exec(`INSERT INTO launch(environment_id, started_at, method, command, plan_id, fingerprint, version_n) VALUES(?,?,?,?,?,?,?)`,
		r.EnvID, r.Started.Unix(), r.Method, r.Command, plan, r.Fingerprint, r.VersionN)
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	return err
}

// AddLaunch records a game launch (08 §2); a launch that is not a pack's is attributed to
// the environment's current plan.
//
// Deprecated: use AddLaunchRecord with the plan, fingerprint and version.
func (s *Store) AddLaunch(envID string, at time.Time, method, command string) error {
	r := &LaunchRecord{EnvID: envID, Started: at, Method: method, Command: command}
	if !strings.HasPrefix(method, "pack:") {
		r.PlanID = s.CurrentPlanID(envID)
	}
	return s.AddLaunchRecord(r)
}

// Launches lists an environment's launches, newest first (limit ≤ 0: all).
func (s *Store) Launches(envID string, limit int) ([]*LaunchRecord, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.DB.Query(`SELECT id, environment_id, coalesce(plan_id, 0), started_at, method, command, fingerprint, version_n FROM launch
		WHERE environment_id=? ORDER BY started_at DESC, id DESC LIMIT ?`, envID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*LaunchRecord
	for rows.Next() {
		var r LaunchRecord
		var at int64
		if err := rows.Scan(&r.ID, &r.EnvID, &r.PlanID, &at, &r.Method, &r.Command, &r.Fingerprint, &r.VersionN); err != nil {
			return nil, err
		}
		r.Started = time.Unix(at, 0).UTC()
		out = append(out, &r)
	}
	return out, rows.Err()
}

// LastLaunch returns the environment's newest launch time (zero if never).
func (s *Store) LastLaunch(envID string) time.Time {
	var t sql.NullInt64
	s.DB.QueryRow(`SELECT max(started_at) FROM launch WHERE environment_id=?`, envID).Scan(&t)
	if !t.Valid {
		return time.Time{}
	}
	return time.Unix(t.Int64, 0).UTC()
}

// LastPlanLaunch returns a plan's newest launch time (zero if never played).
func (s *Store) LastPlanLaunch(planID int64) time.Time {
	var t sql.NullInt64
	s.DB.QueryRow(`SELECT max(started_at) FROM launch WHERE plan_id=?`, planID).Scan(&t)
	if !t.Valid {
		return time.Time{}
	}
	return time.Unix(t.Int64, 0).UTC()
}
