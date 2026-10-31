package store_test

// Queries and writes timed by TestIntKeyResearch, one SQL shape per prototype layout. The
// baseline shapes copy internal/store (analysis.go, files.go, content.go, textures.go,
// library.go, instances.go, changes.go). Each returns a checksum so layouts can be compared.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zeebo/xxh3"
)

type ikCtx struct {
	db  *sql.DB
	l   ikLayout
	smp *ikSample
}

// ikHash hashes result lines; unordered results are sorted first.
func ikHash(lines []string, ordered bool) uint64 {
	if !ordered {
		sort.Strings(lines)
	}
	return xxh3.HashString(strings.Join(lines, "\n"))
}

func ikLines(rows *sql.Rows, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	vals := make([]sql.RawBytes, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var out []string
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = string(v)
		}
		out = append(out, strings.Join(parts, "\x1f"))
	}
	return out, rows.Err()
}

func (c ikCtx) defSelect(where, order string) string {
	return c.defSelectJoin("JOIN", where, order)
}

// defSelectJoin with "CROSS JOIN" forces def_fact as the outer loop (SQLite keeps the order).
func (c ikCtx) defSelectJoin(join, where, order string) string {
	col, ij := c.l.inst()
	fcol, fj := c.l.file("c.inst")
	extra := ""
	if c.l.path == 4 {
		// the set's file list is read once per set in Go (defRows), not parsed per row
		fcol, fj, extra = "coalesce(f.file_id,'')", "", ", f.set_id"
	}
	return `SELECT ` + col + `, f.def_type, f.def_name, f.name, f.parent, f.abstract, ` + fcol + `, f.line, f.may_require, f.may_require_any` + extra + ` FROM def_fact f ` + join + ` content_state c ON c.def_set_id=f.set_id` + ij + fj + ` WHERE ` + where + ` ORDER BY ` + order
}

// defRows runs a defSelect query; with per-set file lists it maps file indexes to paths,
// loading each set's list once per call (no cache across calls).
func (c ikCtx) defRows(q string, args ...any) ([]string, error) {
	ls, err := ikLines(c.db.Query(q, args...))
	if err != nil || c.l.path != 4 {
		return ls, err
	}
	lists := map[string][]string{}
	for i, l := range ls {
		parts := strings.Split(l, "\x1f")
		set := parts[len(parts)-1]
		files, ok := lists[set]
		if !ok {
			var js string
			if err := c.db.QueryRow(`SELECT files FROM fact_set WHERE id=?`, set).Scan(&js); err != nil {
				return nil, err
			}
			json.Unmarshal([]byte(js), &files)
			lists[set] = files
		}
		if k, err := strconv.Atoi(parts[6]); err == nil && k < len(files) {
			parts[6] = files[k]
		}
		ls[i] = strings.Join(parts[:len(parts)-1], "\x1f")
	}
	return ls, nil
}

type ikQuery struct {
	name    string
	runs    int
	ordered bool
	run     func(c ikCtx) ([]string, error)
}

func ikLike(pat, s string) bool {
	// ASCII case-insensitive LIKE with % and _ (enough for InstanceFiles patterns)
	p, x := strings.ToLower(pat), strings.ToLower(s)
	var m func(i, j int) bool
	m = func(i, j int) bool {
		for i < len(p) {
			switch p[i] {
			case '%':
				for k := j; k <= len(x); k++ {
					if m(i+1, k) {
						return true
					}
				}
				return false
			case '_':
				if j >= len(x) {
					return false
				}
			default:
				if j >= len(x) || x[j] != p[i] {
					return false
				}
			}
			i, j = i+1, j+1
		}
		return j == len(x)
	}
	return m(0, 0)
}

// ikBlobOf reads a snapshot's non-semantic list.
func (c ikCtx) blobOf(snap int64, fn func(rel string, size int64)) error {
	q := `SELECT data FROM snapshot_list WHERE snapshot_id=?`
	if c.l.blob == 2 {
		q = `SELECT b.data FROM snapshot_list s JOIN file_blob b ON b.id=s.blob_id WHERE s.snapshot_id=?`
	}
	var data []byte
	if err := c.db.QueryRow(q, snap).Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	return ikDecode(data, fn)
}

// instOfSnap: the instance integer of a snapshot (path mode 3 needs it to read paths).
func (c ikCtx) instOfSnap(snap int64) (int64, error) {
	var n int64
	err := c.db.QueryRow(`SELECT k.n FROM snapshot s JOIN instance_key k ON k.id=s.instance_id WHERE s.id=?`, snap).Scan(&n)
	return n, err
}

func (c ikCtx) instanceFiles(like string) ([]string, error) {
	kj, ie := "", ""
	if c.l.path == 3 {
		kj, ie = " JOIN instance_key k ON k.id=i.id", "k.n"
	}
	rcol, rj := c.l.rel(ie)
	lines, err := ikLines(c.db.Query(`SELECT i.id, `+rcol+` FROM instance i`+kj+` JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id`+rj+`
		WHERE i.environment_id IN ('', ?) AND i.tombstone_at IS NULL AND `+rcol+` LIKE ?`, c.smp.envID, like))
	if err != nil || c.l.blob == 0 || strings.HasSuffix(strings.ToLower(like), ".dll") {
		return lines, err
	}
	rows, err := c.db.Query(`SELECT id, current_snapshot_id FROM instance WHERE environment_id IN ('', ?) AND tombstone_at IS NULL AND current_snapshot_id IS NOT NULL`, c.smp.envID)
	if err != nil {
		return nil, err
	}
	type is struct {
		id   string
		snap int64
	}
	var all []is
	for rows.Next() {
		var x is
		rows.Scan(&x.id, &x.snap)
		all = append(all, x)
	}
	rows.Close()
	for _, x := range all {
		if err := c.blobOf(x.snap, func(rel string, _ int64) {
			if ikLike(like, rel) {
				lines = append(lines, x.id+"\x1f"+rel)
			}
		}); err != nil {
			return nil, err
		}
	}
	return lines, nil
}

func ikQueries() []ikQuery {
	return []ikQuery{
		{"defName×100", 1, true, func(c ikCtx) ([]string, error) {
			col, _ := c.l.inst()
			var all []string
			for _, n := range c.smp.defNames[:min(100, len(c.smp.defNames))] {
				ls, err := c.defRows(c.defSelect(`f.def_name=? AND c.game_mm=?`, col+", f.rowid"), n, "1.6")
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"Name×190", 1, true, func(c ikCtx) ([]string, error) {
			col, _ := c.l.inst()
			var all []string
			for _, n := range c.smp.names[:min(190, len(c.smp.names))] {
				ls, err := c.defRows(c.defSelect(`f.name=? AND f.name<>'' AND c.game_mm=?`, col+", f.rowid"), n, "1.6")
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"defName×100(CROSS JOIN)", 1, true, func(c ikCtx) ([]string, error) {
			col, _ := c.l.inst()
			var all []string
			for _, n := range c.smp.defNames[:min(100, len(c.smp.defNames))] {
				ls, err := c.defRows(c.defSelectJoin("CROSS JOIN", `f.def_name=? AND c.game_mm=?`, col+", f.rowid"), n, "1.6")
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"Name×190(CROSS JOIN)", 1, true, func(c ikCtx) ([]string, error) {
			col, _ := c.l.inst()
			var all []string
			for _, n := range c.smp.names[:min(190, len(c.smp.names))] {
				ls, err := c.defRows(c.defSelectJoin("CROSS JOIN", `f.name=? AND f.name<>'' AND c.game_mm=?`, col+", f.rowid"), n, "1.6")
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"DefsOf×100", 1, true, func(c ikCtx) ([]string, error) {
			var all []string
			for _, id := range c.smp.insts[:min(100, len(c.smp.insts))] {
				ls, err := c.defRows(c.defSelect(c.l.instIs()+` AND c.game_mm=?`, "f.rowid"), id, "1.6")
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"DefCounts(全表)", 1, false, func(c ikCtx) ([]string, error) {
			q := `SELECT c.instance_id, count(*) FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id WHERE c.game_mm=? GROUP BY c.instance_id`
			if c.l.ik {
				q = `SELECT k.id, x.n FROM (SELECT c.inst, count(*) n FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id WHERE c.game_mm=? GROUP BY c.inst) x JOIN instance_key k ON k.n=x.inst`
			}
			return ikLines(c.db.Query(q, "1.6"))
		}},
		{"CachedDefs(未启用全部)", 1, true, func(c ikCtx) ([]string, error) {
			var all []string
			fields := `f.def_type, f.def_name, f.name, f.parent, f.abstract, %s, f.line, f.may_require, f.may_require_any FROM def_fact f%s WHERE f.set_id=? ORDER BY f.rowid`
			for _, k := range c.smp.cached {
				var id, n int64
				var err error
				if c.l.ik {
					err = c.db.QueryRow(`SELECT id, inst FROM fact_set WHERE inst=(SELECT n FROM instance_key WHERE id=?) AND kind='def' AND content_key=?`, k[0], k[1]).Scan(&id, &n)
				} else {
					err = c.db.QueryRow(`SELECT id FROM fact_set WHERE instance_id=? AND kind='def' AND content_key=?`, k[0], k[1]).Scan(&id)
				}
				if err != nil {
					return nil, err
				}
				fcol, fj := c.l.file("?")
				args := []any{id}
				if c.l.path == 3 {
					args = []any{n, id}
				}
				var files []string
				if c.l.path == 4 {
					fcol, fj = "coalesce(f.file_id,'')", ""
					var js string
					if err := c.db.QueryRow(`SELECT files FROM fact_set WHERE id=?`, id).Scan(&js); err != nil {
						return nil, err
					}
					json.Unmarshal([]byte(js), &files)
				}
				ls, err := ikLines(c.db.Query(`SELECT `+fmt.Sprintf(fields, fcol, fj), args...))
				if err != nil {
					return nil, err
				}
				if c.l.path == 4 {
					for i, l := range ls {
						parts := strings.Split(l, "\x1f")
						if k, err := strconv.Atoi(parts[5]); err == nil && k < len(files) {
							parts[5] = files[k]
						}
						ls[i] = strings.Join(parts, "\x1f")
					}
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"TypeOwners×100", 1, false, func(c ikCtx) ([]string, error) {
			col, ij := c.l.inst()
			fcol, fj := c.l.file("c.inst")
			var all []string
			for _, n := range c.smp.types[:min(100, len(c.smp.types))] {
				ls, err := ikLines(c.db.Query(`SELECT DISTINCT `+col+`, `+fcol+` FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id`+ij+fj+` WHERE f.type_name IN (?, ?) AND c.game_mm=?`, n, strings.ReplaceAll(n, ".", "+"), "1.6"))
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"HarmonyPatches(全部)", 1, true, func(c ikCtx) ([]string, error) {
			col, ij := c.l.inst()
			fcol, fj := c.l.file("c.inst")
			return ikLines(c.db.Query(`SELECT `+col+`, `+fcol+`, f.patch_type, f.patch_method, f.kind, f.target, f.skips, f.dynamic FROM harmony_fact f JOIN content_state c ON c.dll_set_id=f.set_id`+ij+fj+` WHERE c.game_mm=? ORDER BY f.target, f.rowid`, "1.6"))
		}},
		{"CurrentFiles(增量扫描)", 1, false, func(c ikCtx) ([]string, error) {
			kj, ie := "", ""
			if c.l.path == 3 {
				kj, ie = " JOIN instance_key k ON k.id=i.id", "k.n"
			}
			rcol, rj := c.l.rel(ie)
			return ikLines(c.db.Query(`SELECT i.source_id, i.rel_path, `+rcol+`, f.size, f.mtime, f.hash FROM instance i`+kj+`
				JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id`+rj+` WHERE i.environment_id IN ('', ?) AND f.hash IS NOT NULL`, c.smp.envID))
		}},
		{"SemanticSet×100", 1, false, func(c ikCtx) ([]string, error) {
			var all []string
			for _, snap := range c.smp.snaps[:min(100, len(c.smp.snaps))] {
				ie := ""
				if c.l.path == 3 {
					n, err := c.instOfSnap(snap)
					if err != nil {
						return nil, err
					}
					ie = fmt.Sprint(n)
				}
				rcol, rj := c.l.rel(ie)
				ls, err := ikLines(c.db.Query(`SELECT lower(`+rcol+`), f.hash FROM snapshot_file f`+rj+` WHERE f.snapshot_id=? AND f.hash IS NOT NULL`, snap))
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"SnapshotFiles×100(变更页)", 1, false, func(c ikCtx) ([]string, error) {
			var all []string
			for _, snap := range c.smp.snaps[:min(100, len(c.smp.snaps))] {
				ie := ""
				if c.l.path == 3 {
					n, err := c.instOfSnap(snap)
					if err != nil {
						return nil, err
					}
					ie = fmt.Sprint(n)
				}
				rcol, rj := c.l.rel(ie)
				ls, err := ikLines(c.db.Query(`SELECT `+rcol+`, f.size, f.hash FROM snapshot_file f`+rj+` WHERE f.snapshot_id=?`, snap))
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
				if c.l.blob != 0 {
					if err := c.blobOf(snap, func(rel string, size int64) { all = append(all, fmt.Sprintf("%s\x1f%d\x1f", rel, size)) }); err != nil {
						return nil, err
					}
				}
			}
			return all, nil
		}},
		{"InstanceFiles(%Textures/%)", 1, false, func(c ikCtx) ([]string, error) { return c.instanceFiles("%Textures/%") }},
		{"InstanceFiles(%x.dll)×20", 1, false, func(c ikCtx) ([]string, error) {
			var all []string
			for _, n := range c.smp.dllNames[:min(20, len(c.smp.dllNames))] {
				ls, err := c.instanceFiles("%" + n)
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
			}
			return all, nil
		}},
		{"ContentKinds(全部,无缓存)", 1, false, func(c ikCtx) ([]string, error) {
			var all []string
			ids := c.smp.snaps
			for len(ids) > 0 {
				n := min(len(ids), 500)
				args := make([]any, n)
				for i := range args {
					args[i] = ids[i]
				}
				ph := strings.TrimSuffix(strings.Repeat("?,", n), ",")
				var q string
				switch {
				case c.l.kinds:
					q = `SELECT id, kinds&1, (kinds>>1)&1, (kinds>>2)&1 FROM snapshot WHERE id IN (` + ph + `)`
				default:
					rcol, rj := c.l.rel("(SELECT k.n FROM snapshot s JOIN instance_key k ON k.id=s.instance_id WHERE s.id=f.snapshot_id)")
					r := rcol
					q = `SELECT f.snapshot_id, MAX(` + r + ` LIKE '%.dll'),
		  MAX((` + r + ` LIKE 'defs/%' OR ` + r + ` LIKE '%/defs/%' OR ` + r + ` LIKE 'patches/%' OR ` + r + ` LIKE '%/patches/%') AND ` + r + ` LIKE '%.xml'),
		  MAX(` + r + ` LIKE 'languages/%' OR ` + r + ` LIKE '%/languages/%')
		FROM snapshot_file f` + rj + ` WHERE f.snapshot_id IN (` + ph + `) GROUP BY f.snapshot_id`
				}
				ls, err := ikLines(c.db.Query(q, args...))
				if err != nil {
					return nil, err
				}
				all = append(all, ls...)
				ids = ids[n:]
			}
			return all, nil
		}},
		{"SnapshotsWithPNG(全部)", 1, false, func(c ikCtx) ([]string, error) {
			if c.l.kinds {
				return ikLines(c.db.Query(`SELECT i.current_snapshot_id FROM instance i JOIN snapshot s ON s.id=i.current_snapshot_id WHERE s.kinds&8<>0`))
			}
			kj, ie := "", ""
			if c.l.path == 3 {
				kj, ie = " JOIN instance_key k ON k.id=i.id", "k.n"
			}
			rcol, rj := c.l.rel(ie)
			return ikLines(c.db.Query(`SELECT i.current_snapshot_id FROM instance i` + kj + ` WHERE i.current_snapshot_id IS NOT NULL
		AND EXISTS(SELECT 1 FROM snapshot_file f` + rj + ` WHERE f.snapshot_id=i.current_snapshot_id AND ` + rcol + ` LIKE '%textures/%.png')`))
		}},
		{"Languages×100(详情)", 1, true, func(c ikCtx) ([]string, error) {
			var all []string
			for _, snap := range c.smp.snaps[:min(100, len(c.smp.snaps))] {
				if c.l.kinds {
					var s string
					if err := c.db.QueryRow(`SELECT langs FROM snapshot WHERE id=?`, snap).Scan(&s); err != nil {
						return nil, err
					}
					if s != "" {
						all = append(all, strings.Split(s, "\n")...)
					}
					continue
				}
				ie := ""
				if c.l.path == 3 {
					n, err := c.instOfSnap(snap)
					if err != nil {
						return nil, err
					}
					ie = fmt.Sprint(n)
				}
				rcol, rj := c.l.rel(ie)
				ls, err := ikLines(c.db.Query(`SELECT `+rcol+` FROM snapshot_file f`+rj+` WHERE f.snapshot_id=? AND lower(`+rcol+`) LIKE '%languages/%' ORDER BY `+rcol, snap))
				if err != nil {
					return nil, err
				}
				all = append(all, ikLangs(ls)...)
			}
			return all, nil
		}},
	}
}

// ---------- writes (rolled back) ----------

type ikManifest struct {
	inst string
	rows []ikRow
}

type ikDefSet struct {
	inst string
	defs []ikDef
}

// pathID resolves a path in the layout's path table, inserting it when missing.
type ikPaths struct {
	tx   *sql.Tx
	l    ikLayout
	inst int64
	mod  map[string]int64
	next int64
	memo map[string]int64 // global tables: paths already resolved in this write
}

func (p *ikPaths) id(rel string) (int64, error) {
	if p.l.path == 1 || p.l.path == 2 {
		if id, ok := p.memo[rel]; ok {
			return id, nil
		}
		id, err := p.lookup(rel)
		if err == nil {
			p.memo[rel] = id
		}
		return id, err
	}
	return p.lookup(rel)
}

func (p *ikPaths) lookup(rel string) (int64, error) {
	var id int64
	switch p.l.path {
	case 1:
		err := p.tx.QueryRow(`SELECT id FROM path WHERE rel=?`, rel).Scan(&id)
		if err == sql.ErrNoRows {
			r, err := p.tx.Exec(`INSERT INTO path(rel) VALUES(?)`, rel)
			if err != nil {
				return 0, err
			}
			return r.LastInsertId()
		}
		return id, err
	case 2:
		h := ikH64(rel)
		err := p.tx.QueryRow(`SELECT id FROM path WHERE h=? AND rel=?`, h, rel).Scan(&id)
		if err == sql.ErrNoRows {
			r, err := p.tx.Exec(`INSERT INTO path(h,rel) VALUES(?,?)`, h, rel)
			if err != nil {
				return 0, err
			}
			return r.LastInsertId()
		}
		return id, err
	case 3:
		if id, ok := p.mod[rel]; ok {
			return id, nil
		}
		p.next++
		if _, err := p.tx.Exec(`INSERT INTO mod_path(inst,id,rel) VALUES(?,?,?)`, p.inst, p.next, rel); err != nil {
			return 0, err
		}
		p.mod[rel] = p.next
		return p.next, nil
	}
	return 0, nil
}

func ikNewPaths(tx *sql.Tx, l ikLayout, inst string) (*ikPaths, error) {
	p := &ikPaths{tx: tx, l: l, memo: map[string]int64{}}
	if l.path != 3 {
		return p, nil
	}
	if err := tx.QueryRow(`SELECT n FROM instance_key WHERE id=?`, inst).Scan(&p.inst); err != nil {
		return nil, err
	}
	p.mod = map[string]int64{}
	rows, err := tx.Query(`SELECT id, rel FROM mod_path WHERE inst=?`, p.inst)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var rel string
		rows.Scan(&id, &rel)
		p.mod[rel] = id
		p.next = max(p.next, id)
	}
	return p, rows.Err()
}

// ikWriteSnapshots inserts new snapshots with the given manifests, as a scan would.
func ikWriteSnapshots(c ikCtx, ms []ikManifest) (time.Duration, error) {
	begin := time.Now()
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for k, m := range ms {
		snap := int64(50_000_000 + k)
		if _, err := tx.Exec(`INSERT INTO snapshot(id,instance_id,scan_id,snapshot_hash) VALUES(?,?,1,x'00')`, snap, m.inst); err != nil {
			return 0, err
		}
		paths, err := ikNewPaths(tx, c.l, m.inst)
		if err != nil {
			return 0, err
		}
		ins := `INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash) VALUES(?,?,?,?,?)`
		if c.l.path != 0 && c.l.path != 4 {
			ins = `INSERT INTO snapshot_file(snapshot_id, path_id, size, mtime, hash) VALUES(?,?,?,?,?)`
		}
		st, err := tx.Prepare(ins)
		if err != nil {
			return 0, err
		}
		var ns []ikRow
		var rels []string
		kinds := 0
		for _, r := range m.rows {
			kinds |= ikKinds(r.rel)
			rels = append(rels, r.rel)
			if c.l.blob != 0 && ikBlobbed(r) {
				ns = append(ns, r)
				continue
			}
			var key any = r.rel
			if c.l.path != 0 && c.l.path != 4 {
				id, err := paths.id(r.rel)
				if err != nil {
					return 0, err
				}
				key = id
			}
			var h any
			if r.hash != nil {
				h = r.hash
			}
			if _, err := st.Exec(snap, key, r.size, r.mtime, h); err != nil {
				return 0, err
			}
		}
		st.Close()
		if c.l.blob != 0 {
			if err := ikStoreList(tx, c.l.blob == 2, snap, ns, nil); err != nil {
				return 0, err
			}
		}
		if c.l.kinds {
			if _, err := tx.Exec(`UPDATE snapshot SET kinds=?, langs=? WHERE id=?`, kinds, strings.Join(ikLangs(rels), "\n"), snap); err != nil {
				return 0, err
			}
		}
	}
	return time.Since(begin), nil
}

// ikWriteDefSets saves new Def sets, as SaveAnalysis would for rebuilt mods.
func ikWriteDefSets(c ikCtx, sets []ikDefSet) (time.Duration, error) {
	begin := time.Now()
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for k, s := range sets {
		var r sql.Result
		if c.l.ik {
			r, err = tx.Exec(`INSERT INTO fact_set(inst,kind,content_key) VALUES((SELECT n FROM instance_key WHERE id=?),'def',?)`, s.inst, fmt.Sprint("bench-new-", k))
		} else {
			r, err = tx.Exec(`INSERT INTO fact_set(instance_id,kind,content_key) VALUES(?,'def',?)`, s.inst, fmt.Sprint("bench-new-", k))
		}
		if err != nil {
			return 0, err
		}
		set, _ := r.LastInsertId()
		local := map[string]int64{}
		if c.l.path == 4 {
			var files []string
			for _, d := range s.defs {
				if _, ok := local[d.file]; !ok {
					local[d.file] = int64(len(files))
					files = append(files, d.file)
				}
			}
			js, _ := json.Marshal(files)
			if _, err := tx.Exec(`UPDATE fact_set SET files=? WHERE id=?`, string(js), set); err != nil {
				return 0, err
			}
		}
		paths, err := ikNewPaths(tx, c.l, s.inst)
		if err != nil {
			return 0, err
		}
		ins := `INSERT INTO def_fact(set_id,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any) VALUES(?,?,?,?,?,?,?,?,?,?)`
		if c.l.path != 0 {
			ins = strings.Replace(ins, "file,", "file_id,", 1)
		}
		st, err := tx.Prepare(ins)
		if err != nil {
			return 0, err
		}
		for _, d := range s.defs {
			var file any = d.file
			if c.l.path == 4 {
				file = local[d.file]
			} else if c.l.path != 0 {
				id, err := paths.id(d.file)
				if err != nil {
					return 0, err
				}
				file = id
			}
			if _, err := st.Exec(set, d.typ, d.defName, d.name, d.parent, d.abstract, file, d.line, d.mr, d.mra); err != nil {
				return 0, err
			}
		}
		st.Close()
	}
	return time.Since(begin), nil
}
