package store

import (
	"database/sql"
	"encoding/json"
)

// rewriteInstanceLists follows a merged ID through editable and historical lists.
// Duplicate entries keep the first position. Fixed pack manifests use id_alias on read.
func rewriteInstanceLists(tx *sql.Tx, m map[string]string) error {
	// Plans: workspaces, undo stacks and versions hold instance IDs. A launch record keeps
	// its list's fingerprint (over instance IDs); it follows the rewritten list.
	fps := map[string]string{}
	for _, t := range []struct{ table, key string }{{"workspace", "plan_id"}, {"workspace_undo", "id"}, {"loadout_version", "id"}} {
		type row struct {
			k       int64
			entries string
		}
		var rows []row
		if err := eachRow(tx, `SELECT `+t.key+`, entries FROM `+t.table, func(r *sql.Rows) error {
			var x row
			if err := r.Scan(&x.k, &x.entries); err != nil {
				return err
			}
			rows = append(rows, x)
			return nil
		}); err != nil {
			return err
		}
		for _, x := range rows {
			out, changed := remapEntries(x.entries, m)
			if !changed {
				continue
			}
			fps[EntriesFingerprint(x.entries)] = EntriesFingerprint(out)
			if _, err := tx.Exec(`UPDATE `+t.table+` SET entries=? WHERE `+t.key+`=?`, out, x.k); err != nil {
				return err
			}
		}
	}
	for old, n := range fps {
		if _, err := tx.Exec(`UPDATE launch SET fingerprint=? WHERE fingerprint=?`, n, old); err != nil {
			return err
		}
	}
	// Staged mods (stage:<plan>, JSON items with "i" and "a").
	var stages [][2]string
	if err := eachRow(tx, `SELECT key, value FROM meta WHERE key LIKE 'stage:%'`, func(r *sql.Rows) error {
		var k, v string
		if err := r.Scan(&k, &v); err != nil {
			return err
		}
		stages = append(stages, [2]string{k, v})
		return nil
	}); err != nil {
		return err
	}
	for _, s := range stages {
		var items []map[string]any
		if json.Unmarshal([]byte(s[1]), &items) != nil {
			continue
		}
		changed := false
		var out []map[string]any
		seen := map[string]bool{}
		for _, it := range items {
			for _, f := range []string{"i", "a"} {
				if v, ok := it[f].(string); ok && m[v] != "" {
					it[f] = m[v]
					changed = true
				}
			}
			if id, _ := it["i"].(string); id != "" {
				if seen[id] {
					changed = true
					continue
				}
				seen[id] = true
			}
			out = append(out, it)
		}
		if changed {
			b, _ := json.Marshal(out)
			if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key=?`, string(b), s[0]); err != nil {
				return err
			}
		}
	}
	return nil
}
