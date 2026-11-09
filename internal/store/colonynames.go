package store

import (
	"database/sql"
	"strings"
)

// Colony names (12 §4.1): the name the player gave a colony on the Saves page, by the
// colony's key. Only RimArcho shows it; the save files and the game keep their own names.

func upgradeColonyNames(tx *sql.Tx, _ func(done, total int64)) error {
	_, err := tx.Exec(`CREATE TABLE colony_name(key TEXT PRIMARY KEY, name TEXT NOT NULL)`)
	return err
}

// ColonyNames returns every colony name the player set, by colony key.
func (s *Store) ColonyNames() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT key, name FROM colony_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, n string
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// ReplaceColonyName clears every key in clear, then names the colony by key ("" leaves it
// unnamed), in one transaction: a colony answers to several keys (12 §4.1).
func (s *Store) ReplaceColonyName(key string, clear []string, name string) error {
	name = strings.TrimSpace(name)
	return s.inTx(func(tx *sql.Tx) error {
		for _, k := range clear {
			if _, err := tx.Exec(`DELETE FROM colony_name WHERE key=?`, k); err != nil {
				return err
			}
		}
		if name == "" {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO colony_name(key, name) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET name=excluded.name`, key, name)
		return err
	})
}

// DeleteColonyNames removes the names kept by these keys.
func (s *Store) DeleteColonyNames(keys []string) error {
	return s.ReplaceColonyName("", keys, "")
}

// SetColonyName names a colony; "" clears the name, so the colony takes its saves' name.
func (s *Store) SetColonyName(key, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		_, err := s.DB.Exec(`DELETE FROM colony_name WHERE key=?`, key)
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO colony_name(key, name) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET name=excluded.name`, key, name)
	return err
}
