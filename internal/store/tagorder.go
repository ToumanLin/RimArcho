package store

import (
	"database/sql"
	"strings"
)

// A Mod's tags have an order (pos, 0 first) the player sets by dragging them; the first
// tag with a colour gives the Mod its mark in the lists (16 §2.9 Tag manager). A tag's
// colour is a preset name (blue, cyan, …) or #rrggbb, kept in tag_color.

// upgradeTagOrder (33 → 34) adds lineage_tag.pos, numbering each lineage's tags in the
// order they were shown (by name), and tag_color. The table is rebuilt, as SQLite cannot
// add a column to the middle of a CREATE TABLE text.
func upgradeTagOrder(tx *sql.Tx, _ func(done, total int64)) error {
	for _, q := range []string{
		`CREATE TABLE lineage_tag_v34(
  environment_id TEXT NOT NULL, lineage_id TEXT NOT NULL, tag TEXT NOT NULL COLLATE NOCASE, pos INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(environment_id, lineage_id, tag))`,
		`INSERT INTO lineage_tag_v34(environment_id, lineage_id, tag, pos)
		SELECT t.environment_id, t.lineage_id, t.tag, (SELECT count(*) FROM lineage_tag o
			WHERE o.environment_id=t.environment_id AND o.lineage_id=t.lineage_id AND o.tag < t.tag) FROM lineage_tag t`,
		`DROP TABLE lineage_tag`,
		`ALTER TABLE lineage_tag_v34 RENAME TO lineage_tag`,
		`CREATE TABLE tag_color(tag TEXT PRIMARY KEY COLLATE NOCASE, color TEXT NOT NULL)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SetTagOrder puts a lineage's tags in this order; tags it carries that are not named
// keep their order after them. Names it does not carry are ignored.
func (s *Store) SetTagOrder(lineage string, tags []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var have []string
	if err := eachRow(tx, `SELECT tag FROM lineage_tag WHERE lineage_id=? ORDER BY pos, tag`, func(r *sql.Rows) error {
		var t string
		if err := r.Scan(&t); err != nil {
			return err
		}
		have = append(have, t)
		return nil
	}, lineage); err != nil {
		return err
	}
	var order []string
	taken := map[string]bool{}
	for _, t := range append(append([]string{}, tags...), have...) {
		k := strings.ToLower(strings.TrimSpace(t))
		if taken[k] {
			continue
		}
		for _, h := range have {
			if strings.ToLower(h) == k {
				taken[k] = true
				order = append(order, h)
				break
			}
		}
	}
	for i, t := range order {
		if _, err := tx.Exec(`UPDATE lineage_tag SET pos=? WHERE lineage_id=? AND tag=?`, i, lineage, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TagColors returns every tag's colour, by lower-cased tag.
func (s *Store) TagColors() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT tag, color FROM tag_color`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		out[strings.ToLower(t)] = c
	}
	return out, rows.Err()
}

// SetTagColor sets (or with "" clears) a tag's colour. The caller checks the value.
func (s *Store) SetTagColor(tag, color string) error {
	tag, color = strings.TrimSpace(tag), strings.TrimSpace(color)
	if color == "" {
		_, err := s.DB.Exec(`DELETE FROM tag_color WHERE tag=?`, tag)
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO tag_color(tag, color) VALUES(?,?)
		ON CONFLICT(tag) DO UPDATE SET tag=excluded.tag, color=excluded.color`, tag, color)
	return err
}

// tagPositions returns, for every lineage carrying one of the tags, the first place one of
// them has among its tags.
func tagPositions(tx *sql.Tx, tags []string) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range tags {
		if err := eachRow(tx, `SELECT lineage_id, pos FROM lineage_tag WHERE tag=?`, func(r *sql.Rows) error {
			var lin string
			var pos int
			if err := r.Scan(&lin, &pos); err != nil {
				return err
			}
			if p, ok := out[lin]; !ok || pos < p {
				out[lin] = pos
			}
			return nil
		}, strings.TrimSpace(t)); err != nil {
			return nil, err
		}
	}
	return out, nil
}
