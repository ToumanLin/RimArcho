package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// LastChanged maps each instance of an environment to the start of the newest scan that
// recorded a change for it (Mod library's "last changed" sort, 16 §2.9). Optional IDs restrict the lookup.
func (s *Store) LastChanged(envID string, instances ...string) (map[string]time.Time, error) {
	where, args := libraryInstances(envID, instances)
	rows, err := s.DB.Query(`SELECT c.instance_id, max(s.started_at) FROM change_event c JOIN scan s ON s.id=c.scan_id
		JOIN instance i ON i.id=c.instance_id WHERE `+where+` GROUP BY c.instance_id`, args...)
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

// RelocateInstance points an instance at a new folder (a move RimArcho made itself, 16 §2.9):
// the next scan finds it there by source and path (R1), so its ID, lineage, history and
// every plan's reference stay the same.
func (s *Store) RelocateInstance(id, sourceID, rel, realPath string) error {
	// It belongs where its new source does (a copy moved into the lock library is shared, 03 §2).
	_, err := s.DB.Exec(`UPDATE instance SET source_id=?, rel_path=?, real_path=?,
		environment_id=coalesce((SELECT environment_id FROM source WHERE id=?), environment_id) WHERE id=?`, sourceID, rel, realPath, sourceID, id)
	return err
}

// Made is what RimArcho recorded when it made a copy (16 §2.9): how ("pin", "pin-update",
// "move"), when, for which plans, and why when it was told.
type Made struct {
	Kind   string    `json:"kind"`
	At     time.Time `json:"at"`
	Plans  []string  `json:"plans,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

// CopyNote is an Instance's note.
type CopyNote struct {
	Text    string
	Made    *Made
	Updated time.Time
}

// CopyNotes are the notes of an environment's instances, optionally restricted to IDs.
func (s *Store) CopyNotes(envID string, instances ...string) (map[string]*CopyNote, error) {
	where, args := libraryInstances(envID, instances)
	rows, err := s.DB.Query(`SELECT n.instance_id, n.text, n.made, n.updated_at FROM instance_note n
		JOIN instance i ON i.id=n.instance_id WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*CopyNote{}
	for rows.Next() {
		var id, made string
		var at int64
		n := &CopyNote{}
		if err := rows.Scan(&id, &n.Text, &made, &at); err != nil {
			return nil, err
		}
		if made != "" {
			var m Made
			if json.Unmarshal([]byte(made), &m) == nil {
				n.Made = &m
			}
		}
		if at > 0 {
			n.Updated = time.Unix(at, 0).UTC()
		}
		out[id] = n
	}
	return out, rows.Err()
}

// SetCopyNote sets the text of an Instance's note ("" clears it; what RimArcho recorded stays).
func (s *Store) SetCopyNote(id, text string, at time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO instance_note(instance_id, text, updated_at) VALUES(?,?,?)
		ON CONFLICT(instance_id) DO UPDATE SET text=excluded.text, updated_at=excluded.updated_at`, id, text, at.Unix())
	return err
}

// SetMade records how RimArcho made a copy.
func (s *Store) SetMade(id string, m *Made) error {
	b, _ := json.Marshal(m)
	_, err := s.DB.Exec(`INSERT INTO instance_note(instance_id, made) VALUES(?,?)
		ON CONFLICT(instance_id) DO UPDATE SET made=excluded.made`, id, string(b))
	return err
}

// UpdatesSince counts an instance's "updated" change events in scans after t.
func (s *Store) UpdatesSince(id string, t time.Time) int {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM change_event c JOIN scan s ON s.id=c.scan_id
		WHERE c.instance_id=? AND c.kind='updated' AND s.started_at>?`, id, t.Unix()).Scan(&n)
	return n
}

// Languages are the language folders a snapshot has (Languages/<name>/…, also under a
// version folder such as 1.6/Languages/<name>/…), each once, in the order found.
func (s *Store) Languages(snapID int64) ([]string, error) {
	var langs string
	err := s.DB.QueryRow(`SELECT langs FROM snapshot WHERE id=?`, snapID).Scan(&langs)
	if err == sql.ErrNoRows || langs == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(langs, "/"), nil
}

// With no IDs the inventory reads all instances; copy details restrict indexed history
// and note lookups to their lineage.
func libraryInstances(envID string, instances []string) (string, []any) {
	where := "i.environment_id IN ('', ?)"
	args := []any{envID}
	if len(instances) > 0 {
		where += " AND i.id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(instances)), ",") + ")"
		for _, id := range instances {
			args = append(args, id)
		}
	}
	return where, args
}
