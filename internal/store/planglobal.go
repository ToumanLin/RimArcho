package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// UniquePlanName is base, or base followed by " 2", " 3"… when taken.
func UniquePlanName(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s %d", base, i); !taken(n) {
			return n
		}
	}
}

// keepLists rewrites the entries naming envID's own instances (its Data/ and Mods/) as
// their packageIds in every list, version and undo step, before that game is removed: the
// lists stay, and the game that reads them next finds its own copy (07 §1.1).
func keepLists(tx *sql.Tx, envID string) error {
	rows, err := tx.Query(`SELECT i.id, lower(coalesce(n.package_id, '')) FROM instance i LEFT JOIN snapshot n ON n.id = i.current_snapshot_id
		WHERE i.environment_id = ?`, envID)
	if err != nil {
		return err
	}
	pkgs := map[string]string{}
	for rows.Next() {
		var id, pkg string
		if err := rows.Scan(&id, &pkg); err != nil {
			rows.Close()
			return err
		}
		pkgs[id] = pkg
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(pkgs) == 0 {
		return err
	}
	type entry struct {
		I string `json:"i,omitempty"`
		M string `json:"m,omitempty"`
	}
	for _, t := range []struct{ table, key string }{{"workspace", "plan_id"}, {"workspace_undo", "id"}, {"loadout_version", "id"}} {
		rows, err := tx.Query(`SELECT ` + t.key + `, entries FROM ` + t.table)
		if err != nil {
			return err
		}
		changed := map[int64]string{}
		for rows.Next() {
			var k int64
			var s string
			if err := rows.Scan(&k, &s); err != nil {
				rows.Close()
				return err
			}
			var l []entry
			if json.Unmarshal([]byte(s), &l) != nil {
				continue
			}
			dirty := false
			for i, e := range l {
				if pkg, ok := pkgs[e.I]; ok && e.I != "" {
					if pkg == "" {
						continue // never read: nothing to name it by
					}
					l[i], dirty = entry{M: pkg}, true
				}
			}
			if dirty {
				b, _ := json.Marshal(l)
				changed[k] = string(b)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for k, s := range changed {
			if _, err := tx.Exec(`UPDATE `+t.table+` SET entries=? WHERE `+t.key+`=?`, s, k); err != nil {
				return err
			}
		}
	}
	return nil
}

// ElsewhereMods lists the other games' own Mods/ instances, leaving out links
// RimArcho made there (_Archo_*). Tombstones keep a plan's deleted copy missing
// instead of healing its packageId to another installed copy (16 §2.9, 07 §1.7).
func (s *Store) ElsewhereMods(envID string) ([]*Mod, error) {
	rows, err := s.DB.Query(`SELECT `+modColsLite+modFrom+` WHERE i.environment_id NOT IN ('', ?) AND s.kind = 'local'
		AND substr(lower(i.rel_path), 1, 7) <> '_archo_' ORDER BY i.id`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mod
	for rows.Next() {
		m, err := scanMod(rows)
		if err != nil {
			return nil, err
		}
		m.store.Store(s)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ForeignInstance is what a list entry from another game says about itself.
type ForeignInstance struct {
	PackageID string // lower case
	RealPath  string
	Official  bool
}

// ForeignInstances describes those of ids that are another game's own instances (its
// Data/ or Mods/).
func (s *Store) ForeignInstances(envID string, ids []string) (map[string]ForeignInstance, error) {
	out := map[string]ForeignInstance{}
	for len(ids) > 0 {
		n := min(len(ids), 500)
		args := []any{envID}
		for _, id := range ids[:n] {
			args = append(args, id)
		}
		rows, err := s.DB.Query(`SELECT i.id, lower(coalesce(n.package_id, '')), i.real_path, s.kind FROM instance i
			JOIN source s ON s.id = i.source_id LEFT JOIN snapshot n ON n.id = i.current_snapshot_id
			WHERE i.environment_id NOT IN ('', ?) AND i.id IN (?`+strings.Repeat(",?", n-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, kind string
			var f ForeignInstance
			if err := rows.Scan(&id, &f.PackageID, &f.RealPath, &kind); err != nil {
				rows.Close()
				return nil, err
			}
			f.Official = kind == "official"
			out[id] = f
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		ids = ids[n:]
	}
	return out, nil
}
