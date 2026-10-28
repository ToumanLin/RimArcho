package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// schemaV14: one environment = one game folder (02 §0). The Workshop, SteamCMD, extra,
// monitor and lock-library folders are shared by every environment (an empty
// source.environment_id), and so are their instances, every lineage, the player's data on lineages and the
// user rules. id_alias maps IDs merged away to the ones kept, for IDs written to disk before
// the merge (pack.json, .rimarcho/origin.json). The merge itself is migrateV14.
// source loses its foreign key to environment: a shared source has an empty environment_id.
const schemaV14 = `
CREATE TABLE id_alias(old_id TEXT PRIMARY KEY, new_id TEXT NOT NULL, kind TEXT NOT NULL);
CREATE TABLE source_v14(
  id TEXT PRIMARY KEY, environment_id TEXT NOT NULL,
  kind TEXT NOT NULL, label TEXT NOT NULL, root TEXT NOT NULL, status TEXT NOT NULL,
  pending_root TEXT NOT NULL DEFAULT '', UNIQUE(environment_id, label));
INSERT INTO source_v14(id, environment_id, kind, label, root, status, pending_root)
  SELECT id, environment_id, kind, label, root, status, pending_root FROM source ORDER BY rowid;
DROP TABLE source; ALTER TABLE source_v14 RENAME TO source;
`

var lockLabelRe = regexp.MustCompile(`^locked[0-9]*$`)

type m14Source struct {
	id, env, kind, label, root, status string
	envName                            string
}

type m14Inst struct {
	id, source, rel, status, lineage, env string
	first                                 int64
}

// migrateV14 merges each environment's copies of the shared folders into one source each,
// their instances by folder name, and the lineages those instances were in; then it rewrites
// every reference to an ID merged away (02 §0.1).
func migrateV14(tx *sql.Tx) error {
	envNames := map[string]string{}
	if err := eachRow(tx, `SELECT id, name FROM environment`, func(r *sql.Rows) error {
		var id, name string
		if err := r.Scan(&id, &name); err != nil {
			return err
		}
		envNames[id] = name
		return nil
	}); err != nil {
		return err
	}

	// 1. Shared sources: group by (kind, folder); the first one of a group is kept.
	var srcs []*m14Source
	if err := eachRow(tx, `SELECT id, environment_id, kind, label, root, status FROM source
		WHERE kind IN ('workshop','steamcmd','extra') ORDER BY rowid`, func(r *sql.Rows) error {
		s := &m14Source{}
		if err := r.Scan(&s.id, &s.env, &s.kind, &s.label, &s.root, &s.status); err != nil {
			return err
		}
		s.envName = envNames[s.env]
		srcs = append(srcs, s)
		return nil
	}); err != nil {
		return err
	}
	srcMap := map[string]string{} // merged source → kept source
	var kept []*m14Source
	byKey := map[string]*m14Source{}
	for _, s := range srcs {
		if s.status != "retired" {
			k := s.kind + "|" + env.PathKey(s.root)
			if c := byKey[k]; c != nil {
				srcMap[s.id] = c.id
				continue
			}
			byKey[k] = s
		}
		kept = append(kept, s)
	}
	// Labels: unique among the shared sources. A per-environment lock library folder
	// (<lock.dir>/<env>) becomes locked-<env>: the global library is lock.dir itself.
	used := map[string]bool{}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].status != "retired" && kept[j].status == "retired" })
	for _, s := range kept {
		want := s.label
		if s.kind == "extra" && lockLabelRe.MatchString(s.label) && s.envName != "" {
			want = "locked-" + s.envName
		}
		s.label = freeLabel14(used, want)
		used[s.label] = true
	}
	for _, s := range kept {
		// Two steps, so a label that moves between rows never collides with UNIQUE(env, label).
		if _, err := tx.Exec(`UPDATE source SET environment_id='', label=? WHERE id=?`, "~m14~"+s.id, s.id); err != nil {
			return err
		}
	}
	for _, s := range kept {
		if _, err := tx.Exec(`UPDATE source SET label=? WHERE id=?`, s.label, s.id); err != nil {
			return err
		}
	}

	// 2. Instances of the shared sources: one per folder name (case-insensitive).
	var insts []*m14Inst
	if err := eachRow(tx, `SELECT i.id, i.source_id, i.rel_path, i.status, i.lineage_id, i.environment_id, i.first_seen
		FROM instance i JOIN source s ON s.id=i.source_id WHERE s.kind IN ('workshop','steamcmd','extra') ORDER BY i.id`, func(r *sql.Rows) error {
		x := &m14Inst{}
		if err := r.Scan(&x.id, &x.source, &x.rel, &x.status, &x.lineage, &x.env, &x.first); err != nil {
			return err
		}
		insts = append(insts, x)
		return nil
	}); err != nil {
		return err
	}
	groups := map[string][]*m14Inst{}
	var order []string
	for _, x := range insts {
		src := x.source
		if k, ok := srcMap[src]; ok {
			src = k
		}
		k := src + "|" + strings.ToLower(x.rel)
		if groups[k] == nil {
			order = append(order, k)
		}
		groups[k] = append(groups[k], x)
	}
	instMap := map[string]string{} // merged instance → kept instance
	uf := newUnionFind()
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool {
			a, b := g[i], g[j]
			if (a.status == "live") != (b.status == "live") {
				return a.status == "live"
			}
			if a.first != b.first {
				return a.first < b.first
			}
			return a.id < b.id
		})
		for _, x := range g[1:] {
			instMap[x.id] = g[0].id
			uf.union(x.lineage, g[0].lineage)
		}
	}
	for old, keep := range srcMap {
		if _, err := tx.Exec(`UPDATE instance SET source_id=? WHERE source_id=?`, keep, old); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE OR IGNORE instance_alias SET source_id=? WHERE source_id=?`, keep, old); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM instance_alias WHERE source_id=?`, old); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE instance SET environment_id='' WHERE source_id IN (SELECT id FROM source WHERE environment_id='')`); err != nil {
		return err
	}

	// 3. Lineages joined through a merged instance become one: the oldest is kept.
	created := map[string]int64{}
	if err := eachRow(tx, `SELECT id, created_at FROM lineage`, func(r *sql.Rows) error {
		var id string
		var at int64
		if err := r.Scan(&id, &at); err != nil {
			return err
		}
		created[id] = at
		return nil
	}); err != nil {
		return err
	}
	linMap := map[string]string{}
	for _, cls := range uf.classes() {
		sort.Slice(cls, func(i, j int) bool {
			if created[cls[i]] != created[cls[j]] {
				return created[cls[i]] < created[cls[j]]
			}
			return cls[i] < cls[j]
		})
		for _, l := range cls[1:] {
			linMap[l] = cls[0]
		}
	}

	if err := rewriteInstances14(tx, instMap); err != nil {
		return err
	}
	if err := rewriteLineages14(tx, linMap); err != nil {
		return err
	}
	if err := rewriteOrigins14(tx, instMap, linMap); err != nil {
		return err
	}
	for old := range srcMap {
		if _, err := tx.Exec(`DELETE FROM source WHERE id=?`, old); err != nil {
			return err
		}
	}
	if len(instMap) > 0 {
		// Analysis runs and the attributed log reports name instances; they are caches.
		for _, q := range []string{`DELETE FROM patch_fact`, `DELETE FROM finding`, `DELETE FROM analysis`, `DELETE FROM log_cache`} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE lineage SET environment_id=''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE identity_question SET environment_id=''
		WHERE instance_id IN (SELECT id FROM instance WHERE environment_id='')`); err != nil {
		return err
	}
	if err := mergePlayerData14(tx, linMap); err != nil {
		return err
	}
	if err := mergeRules14(tx, linMap); err != nil {
		return err
	}
	return recomputeAllPrimaries(tx)
}

func freeLabel14(used map[string]bool, base string) string {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		if l := base + strconv.Itoa(i); !used[l] {
			return l
		}
	}
}

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

func rewriteInstances14(tx *sql.Tx, m map[string]string) error {
	if len(m) == 0 {
		return nil
	}
	olds := sortedKeys(m)
	for _, old := range olds {
		keep := m[old]
		for _, q := range []string{
			`UPDATE OR IGNORE instance_alias SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM instance_alias WHERE instance_id=?1`,
			`UPDATE snapshot SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM change_event WHERE instance_id=?1 AND kind='discovered'`,
			`UPDATE change_event SET instance_id=?2 WHERE instance_id=?1`,
			`UPDATE identity_question SET instance_id=?2 WHERE instance_id=?1`,
			`UPDATE identity_question SET candidate_instance_id=?2 WHERE candidate_instance_id=?1`,
			`UPDATE OR IGNORE source_cache SET instance_id=?2 WHERE instance_id=?1`,
			`DELETE FROM source_cache WHERE instance_id=?1`,
			`DELETE FROM def_fact WHERE instance_id=?1`,
			`DELETE FROM assembly_fact WHERE instance_id=?1`,
			`DELETE FROM type_fact WHERE instance_id=?1`,
			`DELETE FROM harmony_fact WHERE instance_id=?1`,
			`DELETE FROM harmony_id WHERE instance_id=?1`,
			`DELETE FROM content_state WHERE instance_id=?1`,
			`DELETE FROM instance WHERE id=?1`,
			`INSERT OR REPLACE INTO id_alias(old_id, new_id, kind) VALUES(?1, ?2, 'instance')`,
		} {
			if _, err := tx.Exec(q, old, keep); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		if err := mergeCopyNote14(tx, old, keep); err != nil {
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

func mergeCopyNote14(tx *sql.Tx, old, keep string) error {
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

func rewriteLineages14(tx *sql.Tx, m map[string]string) error {
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

// rewriteOrigins14 rewrites the IDs in stored origin markers (snapshot.origin), so a
// snapshot read again from an unchanged origin.json is recognised as unchanged once the
// scan maps the file's IDs through id_alias.
func rewriteOrigins14(tx *sql.Tx, im, lm map[string]string) error {
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

// mergePlayerData14 makes the player's data on lineages global: aliases and notes of merged
// lineages are combined (the kept lineage's alias wins; different notes are both kept), tags
// are joined.
func mergePlayerData14(tx *sql.Tx, m map[string]string) error {
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

// mergeRules14 makes user rules global; a rule now identical to an earlier one is dropped
// (its note, when different, is kept on the earlier one).
func mergeRules14(tx *sql.Tx, m map[string]string) error {
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

type unionFind struct{ parent map[string]string }

func newUnionFind() *unionFind { return &unionFind{parent: map[string]string{}} }

func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// classes returns every set with more than one member.
func (u *unionFind) classes() [][]string {
	by := map[string][]string{}
	for x := range u.parent {
		r := u.find(x)
		by[r] = append(by[r], x)
	}
	var out [][]string
	for _, c := range by {
		if len(c) > 1 {
			sort.Strings(c)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// IDAliases maps IDs merged away by migration v14 to the IDs kept (02 §0.1), for IDs written
// to disk before the merge: pack.json entries and .rimarcho/origin.json.
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
