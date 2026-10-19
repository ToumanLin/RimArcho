package store

import (
	"database/sql"
	"strings"
)

// PlayerData is what the player attached to a lineage (04 §7.1).
type PlayerData struct {
	Alias, Note string
	Tags        []string
}

// PlayerDataAll returns every lineage's player data in the environment.
func (s *Store) PlayerDataAll(envID string) (map[string]*PlayerData, error) {
	out := map[string]*PlayerData{}
	get := func(lin string) *PlayerData {
		if out[lin] == nil {
			out[lin] = &PlayerData{}
		}
		return out[lin]
	}
	rows, err := s.DB.Query(`SELECT lineage_id, alias, note FROM lineage_data WHERE environment_id=?`, envID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var lin, alias, note string
		if err := rows.Scan(&lin, &alias, &note); err != nil {
			rows.Close()
			return nil, err
		}
		if alias != "" || note != "" {
			p := get(lin)
			p.Alias, p.Note = alias, note
		}
	}
	rows.Close()
	rows, err = s.DB.Query(`SELECT lineage_id, tag FROM lineage_tag WHERE environment_id=? ORDER BY tag`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var lin, tag string
		if err := rows.Scan(&lin, &tag); err != nil {
			return nil, err
		}
		p := get(lin)
		p.Tags = append(p.Tags, tag)
	}
	return out, rows.Err()
}

// SetAlias sets (or with "" clears) a lineage's alias.
func (s *Store) SetAlias(envID, lineage, alias string) error {
	return s.setData(envID, lineage, "alias", strings.TrimSpace(alias))
}

// SetNote sets (or with "" clears) a lineage's note.
func (s *Store) SetNote(envID, lineage, note string) error {
	return s.setData(envID, lineage, "note", strings.TrimSpace(note))
}

func (s *Store) setData(envID, lineage, col, v string) error {
	_, err := s.DB.Exec(`INSERT INTO lineage_data(environment_id, lineage_id, `+col+`) VALUES(?,?,?)
		ON CONFLICT(environment_id, lineage_id) DO UPDATE SET `+col+`=excluded.`+col, envID, lineage, v)
	return err
}

// AddTag tags a lineage; it reports whether the tag is new.
func (s *Store) AddTag(envID, lineage, tag string) (bool, error) {
	r, err := s.DB.Exec(`INSERT OR IGNORE INTO lineage_tag(environment_id, lineage_id, tag) VALUES(?,?,?)`, envID, lineage, strings.TrimSpace(tag))
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// RemoveTag removes a tag; it reports whether it was there.
func (s *Store) RemoveTag(envID, lineage, tag string) (bool, error) {
	r, err := s.DB.Exec(`DELETE FROM lineage_tag WHERE environment_id=? AND lineage_id=? AND tag=?`, envID, lineage, strings.TrimSpace(tag))
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// TagCounts lists the environment's tags with how many lineages carry each.
func (s *Store) TagCounts(envID string) ([][2]string, error) {
	rows, err := s.DB.Query(`SELECT tag, count(*) FROM lineage_tag WHERE environment_id=? GROUP BY tag ORDER BY tag COLLATE NOCASE`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var t, n string
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out = append(out, [2]string{t, n})
	}
	return out, rows.Err()
}

var _ = sql.ErrNoRows
