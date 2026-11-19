package store

import "database/sql"

// The player's "functional Mod" mark on a lineage (12 §3.3 rule 2): 1 functional, 0 not
// functional; no row leaves it to detection. Player data like aliases and tags (07 §6.1).
const lineageFunctionalSchema = `CREATE TABLE IF NOT EXISTS lineage_functional (
	lineage_id TEXT PRIMARY KEY,
	functional INTEGER NOT NULL
);
`

func upgradeLineageFunctional(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(lineageFunctionalSchema)
	return err
}

// FunctionalMarks returns the player's marks by lineage.
func (s *Store) FunctionalMarks() (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT lineage_id, functional FROM lineage_functional`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var lin string
		var f int
		if err := rows.Scan(&lin, &f); err != nil {
			return nil, err
		}
		out[lin] = f != 0
	}
	return out, rows.Err()
}

// SetFunctional marks a lineage functional (true) or not functional (false); nil clears
// the mark so detection decides again.
func (s *Store) SetFunctional(lineage string, mark *bool) error {
	if mark == nil {
		_, err := s.DB.Exec(`DELETE FROM lineage_functional WHERE lineage_id=?`, lineage)
		return err
	}
	v := 0
	if *mark {
		v = 1
	}
	_, err := s.DB.Exec(`INSERT INTO lineage_functional(lineage_id, functional) VALUES(?,?)
		ON CONFLICT(lineage_id) DO UPDATE SET functional=excluded.functional`, lineage, v)
	return err
}

// remapFunctional moves marks to merged lineages (m: from → into); the kept lineage's own
// mark wins.
func remapFunctional(tx *sql.Tx, m map[string]string) error {
	for from, into := range m {
		if from == into || into == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO lineage_functional(lineage_id, functional)
			SELECT ?, functional FROM lineage_functional WHERE lineage_id=?`, into, from); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM lineage_functional WHERE lineage_id=?`, from); err != nil {
			return err
		}
	}
	return nil
}

// FunctionalByInstance returns the marks by instance, through each instance's lineage.
func (s *Store) FunctionalByInstance() (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT i.id, f.functional FROM instance i JOIN lineage_functional f ON f.lineage_id=i.lineage_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var f int
		if err := rows.Scan(&id, &f); err != nil {
			return nil, err
		}
		out[id] = f != 0
	}
	return out, rows.Err()
}
