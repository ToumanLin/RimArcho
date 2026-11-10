package store

import (
	"database/sql"
	"strings"
)

// PlayerData is what the player attached to a lineage (07 §6.1).
type PlayerData struct {
	Alias, Note string
	Tags        []string
}

// PlayerDataAll returns every lineage's player data. It is global, like the lineages
// (03 §2); envID is kept for the callers.
func (s *Store) PlayerDataAll(envID string) (map[string]*PlayerData, error) {
	out := map[string]*PlayerData{}
	get := func(lin string) *PlayerData {
		if out[lin] == nil {
			out[lin] = &PlayerData{}
		}
		return out[lin]
	}
	rows, err := s.DB.Query(`SELECT lineage_id, alias, note FROM lineage_data`)
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
	rows, err = s.DB.Query(`SELECT lineage_id, tag FROM lineage_tag ORDER BY lineage_id, pos, tag`)
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
		ON CONFLICT(environment_id, lineage_id) DO UPDATE SET `+col+`=excluded.`+col, Shared, lineage, v)
	return err
}

// AddTag tags a lineage, after its other tags; it reports whether the tag is new.
func (s *Store) AddTag(envID, lineage, tag string) (bool, error) {
	r, err := s.DB.Exec(`INSERT OR IGNORE INTO lineage_tag(environment_id, lineage_id, tag, pos)
		VALUES(?,?,?,(SELECT coalesce(max(pos)+1, 0) FROM lineage_tag WHERE lineage_id=?))`, Shared, lineage, strings.TrimSpace(tag), lineage)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// RemoveTag removes a tag; it reports whether it was there.
func (s *Store) RemoveTag(envID, lineage, tag string) (bool, error) {
	r, err := s.DB.Exec(`DELETE FROM lineage_tag WHERE lineage_id=? AND tag=?`, lineage, strings.TrimSpace(tag))
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// TagCounts lists the environment's tags with how many lineages carry each.
func (s *Store) TagCounts(envID string) ([][2]string, error) {
	rows, err := s.DB.Query(`SELECT tag, count(*) FROM lineage_tag GROUP BY tag ORDER BY tag COLLATE NOCASE`)
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

// Tag notes and whole-tag edits (16 §2.9, Tag manager). Tags are global, like the
// lineages; a tag's note is kept in tag_note, case-insensitive as lineage_tag is.

func upgradeTagNotes(tx *sql.Tx, _ func(done, total int64)) error {
	_, err := tx.Exec(`CREATE TABLE tag_note(tag TEXT PRIMARY KEY COLLATE NOCASE, note TEXT NOT NULL)`)
	return err
}

// TagNotes returns every tag's note, by lower-cased tag.
func (s *Store) TagNotes() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT tag, note FROM tag_note`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var t, n string
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[strings.ToLower(t)] = n
	}
	return out, rows.Err()
}

// SetTagNote sets (or with "" clears) a tag's note.
func (s *Store) SetTagNote(tag, note string) error {
	tag, note = strings.TrimSpace(tag), strings.TrimSpace(note)
	if note == "" {
		_, err := s.DB.Exec(`DELETE FROM tag_note WHERE tag=?`, tag)
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO tag_note(tag, note) VALUES(?,?)
		ON CONFLICT(tag) DO UPDATE SET tag=excluded.tag, note=excluded.note`, tag, note)
	return err
}

// TagLineages returns, for every lineage carrying one of the tags, which of them it
// carries (as stored).
func (s *Store) TagLineages(tags []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, t := range tags {
		rows, err := s.DB.Query(`SELECT lineage_id, tag FROM lineage_tag WHERE tag=?`, strings.TrimSpace(t))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var lin, tag string
			if err := rows.Scan(&lin, &tag); err != nil {
				rows.Close()
				return nil, err
			}
			out[lin] = append(out[lin], tag)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// MergeTags replaces the tags from with into on every lineage, in one transaction: a
// lineage carrying any of from carries into afterwards, at the first place one of them had.
// into may differ from a source only in case (a rename). The notes of from are joined onto
// into's; into keeps its colour, else takes the first colour among from. It returns how
// many lineages carried one of from.
func (s *Store) MergeTags(from []string, into string) (int, error) {
	return s.retag(from, strings.TrimSpace(into))
}

// DeleteTags removes the tags from every lineage, with their notes and colours. It returns how many
// lineages carried one of them.
func (s *Store) DeleteTags(tags []string) (int, error) { return s.retag(tags, "") }

func (s *Store) retag(from []string, into string) (int, error) {
	lins, err := s.TagLineages(from)
	if err != nil {
		return 0, err
	}
	notes, err := s.TagNotes()
	if err != nil {
		return 0, err
	}
	colors, err := s.TagColors()
	if err != nil {
		return 0, err
	}
	color := ""
	if into != "" {
		color = colors[strings.ToLower(into)]
		for _, t := range from {
			if color == "" {
				color = colors[strings.ToLower(strings.TrimSpace(t))]
			}
		}
	}
	var joined []string
	seen := map[string]bool{}
	addNote := func(t string) {
		if n := notes[strings.ToLower(strings.TrimSpace(t))]; n != "" && !seen[n] {
			seen[n] = true
			joined = append(joined, n)
		}
	}
	if into != "" {
		addNote(into)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	places, err := tagPositions(tx, from)
	if err != nil {
		return 0, err
	}
	for _, t := range from {
		t = strings.TrimSpace(t)
		addNote(t)
		if _, err := tx.Exec(`DELETE FROM tag_color WHERE tag=?`, t); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM lineage_tag WHERE tag=?`, t); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM tag_note WHERE tag=?`, t); err != nil {
			return 0, err
		}
	}
	if into != "" {
		for lin := range lins {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO lineage_tag(environment_id, lineage_id, tag, pos) VALUES(?,?,?,?)`, Shared, lin, into, places[lin]); err != nil {
				return 0, err
			}
		}
		if color != "" {
			if _, err := tx.Exec(`INSERT INTO tag_color(tag, color) VALUES(?,?)
				ON CONFLICT(tag) DO UPDATE SET tag=excluded.tag, color=excluded.color`, into, color); err != nil {
				return 0, err
			}
		}
		if _, err := tx.Exec(`DELETE FROM tag_note WHERE tag=?`, into); err != nil {
			return 0, err
		}
		if len(joined) > 0 {
			if _, err := tx.Exec(`INSERT INTO tag_note(tag, note) VALUES(?,?)`, into, strings.Join(joined, "\n")); err != nil {
				return 0, err
			}
		}
	}
	return len(lins), tx.Commit()
}
