package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Plan is one named, ordered enabled list of an environment (04 §1): it owns a workspace,
// an undo stack, versions and a staging list. Names are unique within an environment.
type Plan struct {
	ID      int64
	EnvID   string
	Name    string
	Created time.Time
	Updated time.Time // the workspace's last change
}

// DefaultPlanName names an environment's first plan: the one imported from ModsConfig.xml
// on first use, or the environment's data before plans existed (04 §1.4).
const DefaultPlanName = "default"

// StageKey is the meta key of a plan's staging list (04 §2.2).
func StageKey(planID int64) string { return fmt.Sprintf("stage:%d", planID) }

const planCols = `id, environment_id, name, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	var c, u int64
	if err := row.Scan(&p.ID, &p.EnvID, &p.Name, &c, &u); err != nil {
		return nil, err
	}
	p.Created, p.Updated = time.Unix(c, 0).UTC(), time.Unix(u, 0).UTC()
	return &p, nil
}

// Plans lists an environment's plans in creation order.
func (s *Store) Plans(envID string) ([]*Plan, error) {
	rows, err := s.DB.Query(`SELECT `+planCols+` FROM plan WHERE environment_id=? ORDER BY id`, envID)
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

// PlanByName returns an environment's plan by exact name, else by a unique
// case-insensitive match; nil if none.
func (s *Store) PlanByName(envID, name string) (*Plan, error) {
	ps, err := s.Plans(envID)
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

// CreatePlan stores a new, empty plan (no workspace, no versions).
func (s *Store) CreatePlan(envID, name string, at time.Time) (*Plan, error) {
	res, err := s.DB.Exec(`INSERT INTO plan(environment_id, name, created_at, updated_at) VALUES(?,?,?,?)`, envID, name, at.Unix(), at.Unix())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Plan{ID: id, EnvID: envID, Name: name, Created: at.UTC(), Updated: at.UTC()}, nil
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
			`UPDATE environment SET current_plan=0 WHERE current_plan=?`,
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

// SetCurrentPlan selects an environment's current plan. It writes no game file.
func (s *Store) SetCurrentPlan(envID string, planID int64) error {
	_, err := s.DB.Exec(`UPDATE environment SET current_plan=? WHERE id=?`, planID, envID)
	return err
}

// CurrentPlan returns the environment's current plan; when the stored one is gone it falls
// back to the first plan (and records that). nil when the environment has no plan yet.
func (s *Store) CurrentPlan(envID string) (*Plan, error) {
	if id := s.CurrentPlanID(envID); id != 0 {
		p, err := s.PlanByID(id)
		if err != nil {
			return nil, err
		}
		if p != nil && p.EnvID == envID {
			return p, nil
		}
	}
	ps, err := s.Plans(envID)
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	return ps[0], s.SetCurrentPlan(envID, ps[0].ID)
}
