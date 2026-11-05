package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Plan is one named, ordered enabled list (07 §1): it owns a workspace, an undo stack,
// versions and a staging list. Lists are shared by every game (2026-10-25); names are
// unique across them.
type Plan struct {
	ID int64
	// EnvID is the game the list was made in. It does not limit where the list is used.
	EnvID   string
	Name    string
	Created time.Time
	Updated time.Time // the workspace's last change
	// CreatedGame is the environment's game version when the plan was made; Versions are
	// the game versions the player chose for it, empty for the default (07 §1.6).
	CreatedGame string
	Versions    []string
}

// DefaultPlanName names an environment's first plan: the one imported from ModsConfig.xml
// on first use, or the environment's data before plans existed (07 §1.4).
const DefaultPlanName = "default"

// StageKey is the meta key of a plan's staging list (07 §2.2).
func StageKey(planID int64) string { return fmt.Sprintf("stage:%d", planID) }

const planCols = `id, environment_id, name, created_at, updated_at, created_game, versions`

func scanPlan(row interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	var c, u int64
	var vs string
	if err := row.Scan(&p.ID, &p.EnvID, &p.Name, &c, &u, &p.CreatedGame, &vs); err != nil {
		return nil, err
	}
	p.Versions = strings.Fields(vs)
	p.Created, p.Updated = time.Unix(c, 0).UTC(), time.Unix(u, 0).UTC()
	return &p, nil
}

// Plans lists every plan in creation order.
func (s *Store) Plans() ([]*Plan, error) {
	rows, err := s.DB.Query(`SELECT ` + planCols + ` FROM plan ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlanByID returns a plan, or nil.
func (s *Store) PlanByID(id int64) (*Plan, error) {
	p, err := scanPlan(s.DB.QueryRow(`SELECT `+planCols+` FROM plan WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PlanByName returns a plan by exact name, else by a unique case-insensitive match; nil
// if none.
func (s *Store) PlanByName(name string) (*Plan, error) {
	ps, err := s.Plans()
	if err != nil {
		return nil, err
	}
	var fold []*Plan
	for _, p := range ps {
		if p.Name == name {
			return p, nil
		}
		if strings.EqualFold(p.Name, name) {
			fold = append(fold, p)
		}
	}
	if len(fold) == 1 {
		return fold[0], nil
	}
	return nil, nil
}

// CreatePlan stores a new, empty plan (no workspace, no versions). Its created_game is the
// environment's game version now (07 §1.6).
func (s *Store) CreatePlan(envID, name string, at time.Time) (*Plan, error) {
	res, err := s.DB.Exec(`INSERT INTO plan(environment_id, name, created_at, updated_at, created_game)
		VALUES(?,?,?,?, coalesce((SELECT game_version FROM environment WHERE id=?), ''))`, envID, name, at.Unix(), at.Unix(), envID)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.PlanByID(id)
}

// RenamePlan changes a plan's name; its history is unchanged.
func (s *Store) RenamePlan(id int64, name string) error {
	_, err := s.DB.Exec(`UPDATE plan SET name=? WHERE id=?`, name, id)
	return err
}

// DeletePlan removes a plan with its workspace, undo stack, versions and staging list.
// Launch records and the ModsConfig.xml writer keep pointing at its (now unknown) ID.
func (s *Store) DeletePlan(id int64) error {
	return s.inTx(func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM workspace WHERE plan_id=?`,
			`DELETE FROM workspace_undo WHERE plan_id=?`,
			`DELETE FROM loadout_version WHERE plan_id=?`,
			// Games using it fall back to the oldest other list.
			`UPDATE environment SET current_plan=coalesce((SELECT min(p.id) FROM plan p WHERE p.id<>?1), 0) WHERE current_plan=?1`,
			`DELETE FROM plan_collection WHERE plan_id=?`,
			`DELETE FROM plan WHERE id=?`,
		} {
			if _, err := tx.Exec(q, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`DELETE FROM meta WHERE key=?`, StageKey(id))
		return err
	})
}

// CurrentPlanID is the plan the environment's commands act on (0 when none is set).
func (s *Store) CurrentPlanID(envID string) int64 {
	var id int64
	s.DB.QueryRow(`SELECT current_plan FROM environment WHERE id=?`, envID).Scan(&id)
	return id
}

// SetCurrentPlan selects an environment's current plan and moves it to the front of the
// enable history (PlansEnabled). It writes no game file.
func (s *Store) SetCurrentPlan(envID string, planID int64) error {
	if _, err := s.DB.Exec(`UPDATE environment SET current_plan=? WHERE id=?`, planID, envID); err != nil {
		return err
	}
	ids := []int64{planID}
	for _, id := range s.PlansEnabled() {
		if id != planID {
			ids = append(ids, id)
		}
	}
	return s.SetMeta(metaPlansEnabled, joinIDs(ids))
}

// metaPlansEnabled holds the enable history: list IDs, the most recently enabled first,
// in any game (lists are shared, 07 §1.7). The sidebar orders lists by it (16 §2.1).
const metaPlansEnabled = "plans-enabled"

// PlansEnabled is the enable history, the most recently enabled list first. Lists never
// enabled since it was kept are not in it; deleted ones may be.
func (s *Store) PlansEnabled() []int64 {
	var out []int64
	for _, f := range strings.Fields(s.Meta(metaPlansEnabled)) {
		if id, err := strconv.ParseInt(f, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func joinIDs(ids []int64) string {
	fs := make([]string, len(ids))
	for i, id := range ids {
		fs[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(fs, " ")
}

// CurrentPlan returns the environment's current plan — any list, lists being shared by
// every game; when the stored one is gone it falls back to the first plan (and records
// that). nil when the game has never had a current list: its first use imports its
// ModsConfig.xml (07 §1.4).
func (s *Store) CurrentPlan(envID string) (*Plan, error) {
	id := s.CurrentPlanID(envID)
	if id == 0 {
		return nil, nil
	}
	p, err := s.PlanByID(id)
	if err != nil || p != nil {
		return p, err
	}
	ps, err := s.Plans()
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	return ps[0], s.SetCurrentPlan(envID, ps[0].ID)
}
