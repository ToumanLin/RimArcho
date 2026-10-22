package store

import (
	"encoding/json"
	"time"
)

// LastChanged maps each instance of an environment to the start of the newest scan that
// recorded a change for it (Mod 库存's "最近变化" sort, 06 §2.9).
func (s *Store) LastChanged(envID string) (map[string]time.Time, error) {
	rows, err := s.DB.Query(`SELECT c.instance_id, max(s.started_at) FROM change_event c JOIN scan s ON s.id=c.scan_id
		WHERE s.environment_id=? GROUP BY c.instance_id`, envID)
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

// RelocateInstance points an instance at a new folder (a move RimArcho made itself, 06 §2.9):
// the next scan finds it there by source and path (R1), so its ID, lineage, history and
// every plan's reference stay the same.
func (s *Store) RelocateInstance(id, sourceID, rel, realPath string) error {
	_, err := s.DB.Exec(`UPDATE instance SET source_id=?, rel_path=?, real_path=? WHERE id=?`, sourceID, rel, realPath, id)
	return err
}

// Made is what RimArcho recorded when it made a copy (06 §2.9): how ("pin", "pin-update",
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

// CopyNotes are the notes of an environment's instances.
func (s *Store) CopyNotes(envID string) (map[string]*CopyNote, error) {
	rows, err := s.DB.Query(`SELECT n.instance_id, n.text, n.made, n.updated_at FROM instance_note n
		JOIN instance i ON i.id=n.instance_id WHERE i.environment_id=?`, envID)
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
