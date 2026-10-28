package store

import "strings"

const schemaV22 = `
CREATE TABLE IF NOT EXISTS version_mute(
  package_id TEXT NOT NULL, game_mm TEXT NOT NULL,
  PRIMARY KEY(package_id, game_mm));
`

// VersionMuteKey follows package identity across updates and independently scanned copies.
func VersionMuteKey(pkg string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(pkg)), "_steam")
}

// VersionMutes returns the packages muted for one game minor version, across environments.
func (s *Store) VersionMutes(mm string) (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT package_id FROM version_mute WHERE game_mm=?`, mm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var pkg string
		if err := rows.Scan(&pkg); err != nil {
			return nil, err
		}
		out[pkg] = true
	}
	return out, rows.Err()
}

// AllVersionMutes is used by the plan's per-version warning counts in one read.
func (s *Store) AllVersionMutes() (map[string]map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT game_mm, package_id FROM version_mute`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var mm, pkg string
		if err := rows.Scan(&mm, &pkg); err != nil {
			return nil, err
		}
		if out[mm] == nil {
			out[mm] = map[string]bool{}
		}
		out[mm][pkg] = true
	}
	return out, rows.Err()
}

// SetVersionMutes atomically changes a batch; repeating it is harmless.
func (s *Store) SetVersionMutes(packages []string, mm string, muted bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, pkg := range packages {
		pkg = VersionMuteKey(pkg)
		if muted {
			_, err = tx.Exec(`INSERT OR IGNORE INTO version_mute(package_id, game_mm) VALUES(?,?)`, pkg, mm)
		} else {
			_, err = tx.Exec(`DELETE FROM version_mute WHERE package_id=? AND game_mm=?`, pkg, mm)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
