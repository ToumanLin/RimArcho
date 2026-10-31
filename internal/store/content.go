package store

import (
	"database/sql"
	"sort"
	"strings"
)

// Content kinds of snapshots (06 §3.7 type:assembly|xml|language), kept in snapshot.kinds.
const (
	HasDLL       = 1 << iota // a .dll anywhere
	HasDefsPatch             // XML under Defs/ or Patches/ (at any load-folder depth)
	HasLanguages             // something under Languages/
	HasPNG                   // a .png under a folder whose name ends in "textures" (convertible textures, 08 §5.2)

	contentKinds = HasDLL | HasDefsPatch | HasLanguages
)

// schemaV27: what a snapshot's file list says about its content, computed when the snapshot
// is written (02 §4.3). A snapshot's paths never change (another file list hashes to another
// snapshot), so the columns stay right for its whole life, also after 02 §7.1 drops its rows.
const schemaV27 = `
ALTER TABLE snapshot ADD COLUMN kinds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE snapshot ADD COLUMN langs TEXT NOT NULL DEFAULT '';
`

// migrateV27 fills kinds and langs from the stored file rows, in their primary-key order.
func migrateV27(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT snapshot_id, rel_path FROM snapshot_file ORDER BY snapshot_id, rel_path`)
	if err != nil {
		return err
	}
	type derived struct {
		id    int64
		kinds int
		langs string
	}
	var out []derived
	var cur int64
	var d deriver
	flush := func() {
		if k, l := d.result(); k != 0 || l != "" {
			out = append(out, derived{cur, k, l})
		}
		d = deriver{}
	}
	for rows.Next() {
		var id int64
		var rel string
		if err := rows.Scan(&id, &rel); err != nil {
			rows.Close()
			return err
		}
		if id != cur {
			flush()
			cur = id
		}
		d.add(rel)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	flush()
	st, err := tx.Prepare(`UPDATE snapshot SET kinds=?, langs=? WHERE id=?`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, x := range out {
		if _, err := st.Exec(x.kinds, x.langs, x.id); err != nil {
			return err
		}
	}
	return nil
}

// deriveContent computes snapshot.kinds and snapshot.langs from a file list.
func deriveContent(rels []string) (kinds int, langs string) {
	rels = append([]string(nil), rels...)
	sort.Strings(rels) // langs keep the order of snapshot_file's primary key
	var d deriver
	for _, r := range rels {
		d.add(r)
	}
	return d.result()
}

// deriver folds paths the way the old queries over snapshot_file did: SQLite's LIKE and lower()
// fold ASCII letters only.
type deriver struct {
	kinds int
	langs []string
	seen  map[string]bool
}

func (d *deriver) add(rel string) {
	l := asciiLower(rel)
	if strings.HasSuffix(l, ".dll") {
		d.kinds |= HasDLL
	}
	if strings.HasSuffix(l, ".xml") && (strings.HasPrefix(l, "defs/") || strings.Contains(l, "/defs/") ||
		strings.HasPrefix(l, "patches/") || strings.Contains(l, "/patches/")) {
		d.kinds |= HasDefsPatch
	}
	if strings.HasPrefix(l, "languages/") || strings.Contains(l, "/languages/") {
		d.kinds |= HasLanguages
	}
	if i := strings.Index(l, "textures/"); i >= 0 && strings.HasSuffix(l, ".png") && i+len("textures/") <= len(l)-len(".png") {
		d.kinds |= HasPNG
	}
	if !strings.Contains(l, "languages/") {
		return
	}
	// a language is a folder in Languages/ at the mod root or one load folder down, with
	// something in it
	parts := strings.Split(rel, "/")
	for i := 0; i+2 < len(parts) && i <= 1; i++ {
		if strings.EqualFold(parts[i], "Languages") {
			if k := strings.ToLower(parts[i+1]); !d.seen[k] {
				if d.seen == nil {
					d.seen = map[string]bool{}
				}
				d.seen[k] = true
				d.langs = append(d.langs, parts[i+1])
			}
			break
		}
	}
}

// result: the kinds, and the languages joined by "/" (which no folder name contains).
func (d *deriver) result() (int, string) { return d.kinds, strings.Join(d.langs, "/") }

func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if 'A' <= b[j] && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// ContentKinds returns the content flags of the current snapshots of the given instances
// (instance ID → flags) from snapshot.kinds, by primary key.
func (s *Store) ContentKinds(snapOf map[string]int64) (map[string]int, error) {
	bySnap := map[int64]int{}
	if err := s.eachSnapshot(snapshotIDs(snapOf), `kinds`, func(r *sql.Rows) error {
		var id int64
		var k int
		if err := r.Scan(&id, &k); err != nil {
			return err
		}
		bySnap[id] = k & contentKinds
		return nil
	}); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(snapOf))
	for inst, snap := range snapOf {
		out[inst] = bySnap[snap]
	}
	return out, nil
}

// eachSnapshot reads the given column(s) of the snapshots with these IDs, the ID first, in
// batches that stay below SQLite's parameter limit.
func (s *Store) eachSnapshot(ids []any, cols string, f func(*sql.Rows) error) error {
	for len(ids) > 0 {
		n := min(len(ids), 500)
		rows, err := s.DB.Query(`SELECT id, `+cols+` FROM snapshot WHERE id IN (`+placeholders(n)+`)`, ids[:n]...)
		if err != nil {
			return err
		}
		for rows.Next() {
			if err := f(rows); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// snapshotIDs deduplicates the nonempty current snapshots before querying.
func snapshotIDs(snapOf map[string]int64) []any {
	seen := map[int64]bool{}
	var ids []any
	for _, id := range snapOf {
		if id != 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

// SnapshotsWithPNGFor: which of the requested copies have convertible textures.
func (s *Store) SnapshotsWithPNGFor(snapOf map[string]int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	err := s.eachSnapshot(snapshotIDs(snapOf), `kinds`, func(r *sql.Rows) error {
		var id int64
		var k int
		if err := r.Scan(&id, &k); err != nil {
			return err
		}
		if k&HasPNG != 0 {
			out[id] = true
		}
		return nil
	})
	return out, err
}
