// Rewriting instance and lineage IDs everywhere they are stored, used when copies are
// linked (linkedinstances.go) or credit records are remapped (creditremap.go).
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func eachRow(tx *sql.Tx, q string, f func(*sql.Rows) error, args ...any) error {
	rows, err := tx.Query(q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func rewriteInstances(tx *sql.Tx, m map[string]string) error {
	if len(m) == 0 {
		return nil
	}
	olds := sortedKeys(m)
	useTable := "source_cache"
	var library int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='source_use' AND type='table'`).Scan(&library); err != nil {
		return err
	}
	if library > 0 {
		useTable = "source_use"
	}
	for _, old := range olds {
		keep := m[old]
		if err := deleteInstanceFacts(tx, old); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE OR IGNORE instance_alias SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM instance_alias WHERE instance_id=?1`,
			`UPDATE snapshot SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM change_event WHERE instance_id=?1 AND kind='discovered'`,
			`UPDATE change_event SET instance_id=?2 WHERE instance_id=?1`,
			`UPDATE identity_question SET instance_id=?2 WHERE instance_id=?1`,
			`UPDATE identity_question SET candidate_instance_id=?2 WHERE candidate_instance_id=?1`,
			`UPDATE OR IGNORE ` + useTable + ` SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM ` + useTable + ` WHERE instance_id=?1`,
			`DELETE FROM instance WHERE id=?1`,
			`INSERT OR REPLACE INTO id_alias(old_id, new_id, kind) VALUES(?1, ?2, 'instance')`,
		} {
			if _, err := tx.Exec(q, old, keep); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		if err := mergeCopyNote(tx, old, keep); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE identity_question SET status='resolved', resolution='superseded'
		WHERE status='open' AND instance_id=candidate_instance_id`); err != nil {
		return err
	}
	return rewriteInstanceLists(tx, m)
}

// remapEntries rewrites the instance IDs of a stored list (loadout.Entry JSON: {"i": id} or
// {"m": token}); an instance listed twice after the rewrite keeps its first position.
func remapEntries(s string, m map[string]string) (string, bool) {
	var items []map[string]any
	if json.Unmarshal([]byte(s), &items) != nil {
		return s, false
	}
	changed := false
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if id, ok := it["i"].(string); ok && id != "" {
			if n := m[id]; n != "" {
				it["i"], id, changed = n, n, true
			}
			if seen[id] {
				changed = true
				continue
			}
			seen[id] = true
		}
		out = append(out, it)
	}
	if !changed {
		return s, false
	}
	b, _ := json.Marshal(out)
	return string(b), true
}

// EntriesFingerprint is app.ListFingerprint of a stored list: sha256 over each entry's key
// (the instance ID, or "missing:<token>") and a newline, hex, 24 characters.
func EntriesFingerprint(s string) string {
	var items []struct {
		I string `json:"i"`
		M string `json:"m"`
	}
	json.Unmarshal([]byte(s), &items)
	h := sha256.New()
	for _, it := range items {
		k := it.I
		if k == "" {
			k = "missing:" + it.M
		}
		fmt.Fprintf(h, "%s\n", k)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:24]
}

func mergeCopyNote(tx *sql.Tx, old, keep string) error {
	var ot, om string
	err := tx.QueryRow(`SELECT text, made FROM instance_note WHERE instance_id=?`, old).Scan(&ot, &om)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var kt, km string
	var at int64
	err = tx.QueryRow(`SELECT text, made, updated_at FROM instance_note WHERE instance_id=?`, keep).Scan(&kt, &km, &at)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`UPDATE instance_note SET instance_id=? WHERE instance_id=?`, keep, old)
		return err
	}
	if err != nil {
		return err
	}
	if km == "" {
		km = om
	}
	if _, err := tx.Exec(`UPDATE instance_note SET text=?, made=? WHERE instance_id=?`, joinText(kt, ot), km, keep); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM instance_note WHERE instance_id=?`, old)
	return err
}

// joinText keeps both of two texts, once each.
func joinText(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case b == "" || a == b:
		return a
	case a == "":
		return b
	}
	return a + "\n\n" + b
}

func rewriteLineages(tx *sql.Tx, m map[string]string) error {
	for _, old := range sortedKeys(m) {
		keep := m[old]
		for _, q := range []string{
			`UPDATE instance SET lineage_id=?2 WHERE lineage_id=?1`,
			`UPDATE user_rule SET a_lineage=?2 WHERE a_lineage=?1`,
			`UPDATE user_rule SET b_lineage=?2 WHERE b_lineage=?1`,
			`UPDATE OR IGNORE finding_ignore SET lineage_id=?2 WHERE lineage_id=?1`,
			`DELETE FROM finding_ignore WHERE lineage_id=?1`,
			`DELETE FROM lineage WHERE id=?1`,
			`INSERT OR REPLACE INTO id_alias(old_id, new_id, kind) VALUES(?1, ?2, 'lineage')`,
		} {
			if _, err := tx.Exec(q, old, keep); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
	}
	return nil
}

// rewriteOrigins rewrites the IDs in stored origin markers (snapshot.origin), so a
// snapshot read again from an unchanged origin.json is recognised as unchanged once the
// scan maps the file's IDs through id_alias.
func rewriteOrigins(tx *sql.Tx, im, lm map[string]string) error {
	if len(im) == 0 && len(lm) == 0 {
		return nil
	}
	type row struct {
		id     int64
		origin string
	}
	var rows []row
	if err := eachRow(tx, `SELECT id, origin FROM snapshot WHERE origin<>''`, func(r *sql.Rows) error {
		var x row
		if err := r.Scan(&x.id, &x.origin); err != nil {
			return err
		}
		rows = append(rows, x)
		return nil
	}); err != nil {
		return err
	}
	for _, x := range rows {
		var o map[string]any
		if json.Unmarshal([]byte(x.origin), &o) != nil {
			continue
		}
		changed := false
		if v, _ := o["origin_instance"].(string); im[v] != "" {
			o["origin_instance"], changed = im[v], true
		}
		if v, _ := o["origin_lineage"].(string); lm[v] != "" {
			o["origin_lineage"], changed = lm[v], true
		}
		if changed {
			b, _ := json.Marshal(o)
			if _, err := tx.Exec(`UPDATE snapshot SET origin=? WHERE id=?`, string(b), x.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergePlayerData merges the player's data of merged lineages: aliases and notes
// are combined (the kept lineage's alias wins; different notes are both kept), tags
// are joined.
func mergePlayerData(tx *sql.Tx, m map[string]string) error {
	type data struct{ alias, note string }
	var order []string
	all := map[string]*data{}
	type drow struct{ lin, alias, note string }
	var drows []drow
	if err := eachRow(tx, `SELECT lineage_id, alias, note FROM lineage_data ORDER BY rowid`, func(r *sql.Rows) error {
		var x drow
		if err := r.Scan(&x.lin, &x.alias, &x.note); err != nil {
			return err
		}
		drows = append(drows, x)
		return nil
	}); err != nil {
		return err
	}
	// The kept lineage's own row first, so its alias wins.
	sort.SliceStable(drows, func(i, j int) bool { return m[drows[i].lin] == "" && m[drows[j].lin] != "" })
	for _, x := range drows {
		lin := x.lin
		if n := m[lin]; n != "" {
			lin = n
		}
		d := all[lin]
		if d == nil {
			d = &data{}
			all[lin] = d
			order = append(order, lin)
		}
		if d.alias == "" {
			d.alias = strings.TrimSpace(x.alias)
		}
		d.note = joinText(d.note, x.note)
	}
	type trow struct{ lin, tag string }
	var tags []trow
	if err := eachRow(tx, `SELECT lineage_id, tag FROM lineage_tag ORDER BY rowid`, func(r *sql.Rows) error {
		var x trow
		if err := r.Scan(&x.lin, &x.tag); err != nil {
			return err
		}
		tags = append(tags, x)
		return nil
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM lineage_data`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM lineage_tag`); err != nil {
		return err
	}
	for _, lin := range order {
		d := all[lin]
		if d.alias == "" && d.note == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO lineage_data(environment_id, lineage_id, alias, note) VALUES('', ?, ?, ?)`, lin, d.alias, d.note); err != nil {
			return err
		}
	}
	for _, t := range tags {
		lin := t.lin
		if n := m[lin]; n != "" {
			lin = n
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO lineage_tag(environment_id, lineage_id, tag) VALUES('', ?, ?)`, lin, t.tag); err != nil {
			return err
		}
	}
	return nil
}

// mergeRules rewrites user rules to the kept IDs; a rule now identical to an earlier one is dropped
// (its note, when different, is kept on the earlier one).
func mergeRules(tx *sql.Tx, m map[string]string) error {
	type rule struct {
		id                    int64
		kind, a, b, tgt, note string
	}
	var rules []*rule
	if err := eachRow(tx, `SELECT id, kind, a_lineage, b_lineage, target, note FROM user_rule ORDER BY id`, func(r *sql.Rows) error {
		x := &rule{}
		if err := r.Scan(&x.id, &x.kind, &x.a, &x.b, &x.tgt, &x.note); err != nil {
			return err
		}
		rules = append(rules, x)
		return nil
	}); err != nil {
		return err
	}
	first := map[string]*rule{}
	for _, x := range rules {
		k := x.kind + "|" + x.a + "|" + x.b + "|" + x.tgt
		if f := first[k]; f != nil {
			if n := joinText(f.note, x.note); n != f.note {
				f.note = n
				if _, err := tx.Exec(`UPDATE user_rule SET note=? WHERE id=?`, n, f.id); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`DELETE FROM user_rule WHERE id=?`, x.id); err != nil {
				return err
			}
			continue
		}
		first[k] = x
	}
	_, err := tx.Exec(`UPDATE user_rule SET environment_id=''`)
	return err
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IDAliases maps merged-away IDs to the IDs kept (02 §0.1), for IDs written to disk
// before the merge: pack.json entries and .rimarcho/origin.json.
func (s *Store) IDAliases() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT old_id, new_id FROM id_alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var o, n string
		if err := rows.Scan(&o, &n); err != nil {
			return nil, err
		}
		out[o] = n
	}
	return out, rows.Err()
}
