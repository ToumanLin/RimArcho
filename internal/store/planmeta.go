package store

import (
	"encoding/json"
	"strings"
	"time"
)

// SetPlanVersions stores the game versions a player chose for a plan ("" = the default).
func (s *Store) SetPlanVersions(id int64, versions []string) error {
	_, err := s.DB.Exec(`UPDATE plan SET versions=? WHERE id=?`, strings.Join(versions, " "), id)
	return err
}

// CollectionItem is one item of a followed collection as RimArcho saw it.
type CollectionItem struct {
	WorkshopID string `json:"id"`
	Title      string `json:"title,omitempty"`
	Sub        bool   `json:"sub,omitempty"` // a nested collection
}

// PlanCollection is a Workshop collection a plan follows (04 §9.6).
type PlanCollection struct {
	PlanID     int64
	WorkshopID string
	Title      string
	Mode       string // "add" or "follow"
	Linked     time.Time
	Checked    time.Time // zero: never checked since it was linked
	// Snapshot: the items when it was linked or last caught up with; Seen: at the last check.
	Snapshot []CollectionItem
	Seen     []CollectionItem
}

// PlanCollections lists the collections a plan follows, in the order they were linked.
func (s *Store) PlanCollections(planID int64) ([]*PlanCollection, error) {
	rows, err := s.DB.Query(`SELECT plan_id, workshop_id, title, mode, linked_at, checked_at, snapshot, seen
		FROM plan_collection WHERE plan_id=? ORDER BY linked_at, workshop_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PlanCollection
	for rows.Next() {
		var c PlanCollection
		var linked, checked int64
		var snap, seen string
		if err := rows.Scan(&c.PlanID, &c.WorkshopID, &c.Title, &c.Mode, &linked, &checked, &snap, &seen); err != nil {
			return nil, err
		}
		c.Linked = time.Unix(linked, 0).UTC()
		if checked > 0 {
			c.Checked = time.Unix(checked, 0).UTC()
		}
		json.Unmarshal([]byte(snap), &c.Snapshot)
		json.Unmarshal([]byte(seen), &c.Seen)
		out = append(out, &c)
	}
	return out, rows.Err()
}

// SetPlanCollection inserts or replaces a followed collection.
func (s *Store) SetPlanCollection(c *PlanCollection) error {
	snap, _ := json.Marshal(nonNil(c.Snapshot))
	seen, _ := json.Marshal(nonNil(c.Seen))
	var checked int64
	if !c.Checked.IsZero() {
		checked = c.Checked.Unix()
	}
	_, err := s.DB.Exec(`INSERT INTO plan_collection(plan_id, workshop_id, title, mode, linked_at, checked_at, snapshot, seen)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(plan_id, workshop_id) DO UPDATE SET title=excluded.title, mode=excluded.mode,
		linked_at=excluded.linked_at, checked_at=excluded.checked_at, snapshot=excluded.snapshot, seen=excluded.seen`,
		c.PlanID, c.WorkshopID, c.Title, c.Mode, c.Linked.Unix(), checked, string(snap), string(seen))
	return err
}

// DeletePlanCollection unlinks a collection from a plan; the plan is unchanged.
func (s *Store) DeletePlanCollection(planID int64, workshopID string) (bool, error) {
	r, err := s.DB.Exec(`DELETE FROM plan_collection WHERE plan_id=? AND workshop_id=?`, planID, workshopID)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

func nonNil(v []CollectionItem) []CollectionItem {
	if v == nil {
		return []CollectionItem{}
	}
	return v
}
