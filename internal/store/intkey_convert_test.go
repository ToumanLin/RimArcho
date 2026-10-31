package store_test

// Prototype conversions for TestIntKeyResearch (intkey_research_test.go). Research only:
// they run on copies of a synthetic database, never on a player's archo.db.

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/xxh3"
	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("ik_h64", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		s, _ := args[0].(string)
		return int64(xxh3.HashString(s)), nil
	})
}

func ikH64(s string) int64 { return int64(xxh3.HashString(s)) }

// ikRebuildFacts replaces each fact table's file TEXT with file_id, keeping rowids (queries
// order by them).
func ikRebuildFacts(t *testing.T, tx *sql.Tx, lookup string) {
	for _, spec := range strings.Split(ikFactTables, "|") {
		table, cols, _ := strings.Cut(spec, ":")
		create, idx, _ := strings.Cut(ikFactDDL(table), ";\n")
		ikExec(t, tx, create)
		var sel []string
		for _, c := range strings.Split(cols, ",") {
			if c == "FILE" {
				sel = append(sel, "p.id")
			} else {
				sel = append(sel, "x."+c)
			}
		}
		ins := strings.ReplaceAll(cols, "FILE", "file_id")
		ikExec(t, tx, `INSERT INTO `+table+`2(rowid,`+ins+`) SELECT x.rowid,`+strings.Join(sel, ",")+` FROM `+table+` x `+lookup+` ORDER BY x.rowid`)
		ikExec(t, tx, `DROP TABLE `+table, `ALTER TABLE `+table+`2 RENAME TO `+table)
		ikExec(t, tx, strings.ReplaceAll(idx, table+"2(", table+"("))
	}
}

func ikRebuildSnapshotFiles(t *testing.T, tx *sql.Tx, lookup string) {
	ikExec(t, tx, `CREATE TABLE snapshot_file2(snapshot_id INTEGER NOT NULL, path_id INTEGER NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL, hash BLOB,
  PRIMARY KEY(snapshot_id, path_id)) WITHOUT ROWID`)
	ikExec(t, tx, `INSERT INTO snapshot_file2 SELECT x.snapshot_id, p.id, x.size, x.mtime, x.hash FROM snapshot_file x `+lookup)
	ikExec(t, tx, `DROP TABLE snapshot_file`, `ALTER TABLE snapshot_file2 RENAME TO snapshot_file`)
}

const ikAllPaths = `SELECT file rel FROM def_fact UNION SELECT file FROM assembly_fact UNION SELECT file FROM type_fact UNION SELECT file FROM harmony_fact UNION SELECT file FROM harmony_id UNION SELECT rel_path FROM snapshot_file`

func ikConvertPaths(t *testing.T, tx *sql.Tx, mode int) {
	switch mode {
	case 1:
		ikExec(t, tx, `CREATE TABLE path(id INTEGER PRIMARY KEY, rel TEXT NOT NULL UNIQUE)`)
		ikExec(t, tx, `INSERT INTO path(rel) `+ikAllPaths)
		ikRebuildFacts(t, tx, "JOIN path p ON p.rel=x.file")
		ikRebuildSnapshotFiles(t, tx, "JOIN path p ON p.rel=x.rel_path")
	case 2:
		ikExec(t, tx, `CREATE TABLE path(id INTEGER PRIMARY KEY, h INTEGER NOT NULL, rel TEXT NOT NULL)`)
		ikExec(t, tx, `INSERT INTO path(h,rel) SELECT ik_h64(rel), rel FROM (`+ikAllPaths+`)`)
		ikExec(t, tx, `CREATE INDEX path_h ON path(h)`)
		ikRebuildFacts(t, tx, "JOIN path p ON p.h=ik_h64(x.file) AND p.rel=x.file")
		ikRebuildSnapshotFiles(t, tx, "JOIN path p ON p.h=ik_h64(x.rel_path) AND p.rel=x.rel_path")
	case 4:
		// a file list per fact set: sets are immutable and own their rows, so the list
		// lives and dies with the set; snapshot_file keeps its text paths
		ikExec(t, tx, `ALTER TABLE fact_set ADD COLUMN files TEXT NOT NULL DEFAULT '[]'`)
		ikExec(t, tx, `UPDATE fact_set SET files=(SELECT json_group_array(file) FROM (SELECT file, min(rowid) r FROM def_fact WHERE set_id=fact_set.id GROUP BY file ORDER BY r)) WHERE kind='def'`)
		ikExec(t, tx, `UPDATE fact_set SET files=(SELECT json_group_array(file) FROM (SELECT file, min(rowid) r FROM assembly_fact WHERE set_id=fact_set.id GROUP BY file ORDER BY r)) WHERE kind='dll'`)
		ikExec(t, tx, `CREATE TEMP TABLE set_file(set_id INTEGER NOT NULL, file TEXT NOT NULL, id INTEGER NOT NULL, PRIMARY KEY(set_id, file))`)
		ikExec(t, tx, `INSERT INTO set_file SELECT s.id, j.value, j.key FROM fact_set s, json_each(s.files) j`)
		ikRebuildFacts(t, tx, "JOIN set_file p ON p.set_id=x.set_id AND p.file=x.file")
		ikExec(t, tx, `DROP TABLE set_file`)
	case 3:
		var u []string
		for _, tb := range []string{"def_fact", "assembly_fact", "type_fact", "harmony_fact", "harmony_id"} {
			u = append(u, `SELECT s.inst, x.file rel FROM `+tb+` x JOIN fact_set s ON s.id=x.set_id`)
		}
		u = append(u, `SELECT k.n, x.rel_path FROM snapshot_file x JOIN snapshot n ON n.id=x.snapshot_id JOIN instance_key k ON k.id=n.instance_id`)
		ikExec(t, tx, `CREATE TABLE mod_path(inst INTEGER NOT NULL, id INTEGER NOT NULL, rel TEXT NOT NULL, PRIMARY KEY(inst,id)) WITHOUT ROWID`)
		ikExec(t, tx, `INSERT INTO mod_path SELECT inst, row_number() OVER (PARTITION BY inst ORDER BY rel), rel FROM (`+strings.Join(u, " UNION ")+`)`)
		ikExec(t, tx, `CREATE INDEX mod_path_rel_tmp ON mod_path(inst, rel)`)
		ikRebuildFacts(t, tx, "JOIN fact_set s ON s.id=x.set_id JOIN mod_path p ON p.inst=s.inst AND p.rel=x.file")
		ikRebuildSnapshotFiles(t, tx, "JOIN snapshot n ON n.id=x.snapshot_id JOIN instance_key k ON k.id=n.instance_id JOIN mod_path p ON p.inst=k.n AND p.rel=x.rel_path")
		ikExec(t, tx, `DROP INDEX mod_path_rel_tmp`)
	}
}

// ikConvertSlim: 1 = inactive instances keep only type / defName / Name; 2 = inactive drop
// parent and abstract only (fields nothing reads for inactive mods).
func ikConvertSlim(t *testing.T, tx *sql.Tx, l ikLayout) {
	inactive := `SELECT id FROM fact_set WHERE kind='def' AND instance_id NOT IN (SELECT id FROM bench_active)`
	if l.ik {
		inactive = `SELECT id FROM fact_set WHERE kind='def' AND inst NOT IN (SELECT k.n FROM instance_key k JOIN bench_active a ON a.id=k.id)`
	}
	file := "file=''"
	if l.path != 0 {
		file = "file_id=NULL"
	}
	set := file + ", line=0, parent='', abstract=0, may_require='', may_require_any=''"
	if l.slim == 2 {
		set = "parent='', abstract=0"
	}
	ikExec(t, tx, `UPDATE def_fact SET `+set+` WHERE set_id IN (`+inactive+`)`)
}

// ---------- non-semantic file lists as one blob per snapshot ----------

const (
	ikHasDLL = 1 << iota
	ikHasDefsPatch
	ikHasLanguages
	ikHasPNG
)

func ikKinds(rel string) int {
	l := strings.ToLower(rel)
	k := 0
	if strings.HasSuffix(l, ".dll") {
		k |= ikHasDLL
	}
	if (strings.HasPrefix(l, "defs/") || strings.Contains(l, "/defs/") || strings.HasPrefix(l, "patches/") || strings.Contains(l, "/patches/")) && strings.HasSuffix(l, ".xml") {
		k |= ikHasDefsPatch
	}
	if strings.HasPrefix(l, "languages/") || strings.Contains(l, "/languages/") {
		k |= ikHasLanguages
	}
	if i := strings.Index(l, "textures/"); i >= 0 && strings.HasSuffix(l[i+9:], ".png") {
		k |= ikHasPNG
	}
	return k
}

// ikLangs mirrors Store.Languages over rows in primary-key order.
func ikLangs(rels []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, rel := range rels {
		if !strings.Contains(strings.ToLower(rel), "languages/") {
			continue
		}
		parts := strings.Split(rel, "/")
		for i := 0; i+2 < len(parts); i++ {
			if strings.EqualFold(parts[i], "Languages") && (i == 0 || i == 1) {
				if k := strings.ToLower(parts[i+1]); !seen[k] {
					seen[k] = true
					out = append(out, parts[i+1])
				}
				break
			}
		}
	}
	return out
}

// ikBlobbed: rows that move into the per-snapshot list. DLLs stay rows wherever they are, so
// the per-assembly log lookup (InstanceFiles "%x.dll") does not unpack every list.
func ikBlobbed(r ikRow) bool {
	return r.hash == nil && !strings.HasSuffix(strings.ToLower(r.rel), ".dll")
}

type ikRow struct {
	rel         string
	size, mtime int64
	hash        []byte
}

var (
	ikEnc, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	ikDec, _ = zstd.NewReader(nil)
)

// ikEncode front-codes sorted paths with their sizes (and optionally mtimes).
func ikEncode(rows []ikRow, mtime bool) []byte {
	var b []byte
	prev := ""
	var pm int64
	for _, r := range rows {
		p := 0
		for p < len(prev) && p < len(r.rel) && prev[p] == r.rel[p] {
			p++
		}
		b = binary.AppendUvarint(b, uint64(p))
		b = binary.AppendUvarint(b, uint64(len(r.rel)-p))
		b = append(b, r.rel[p:]...)
		b = binary.AppendUvarint(b, uint64(r.size))
		if mtime {
			b = binary.AppendVarint(b, r.mtime-pm)
			pm = r.mtime
		}
		prev = r.rel
	}
	return b
}

func ikDecode(data []byte, fn func(rel string, size int64)) error {
	b, err := ikDec.DecodeAll(data, nil)
	if err != nil {
		return err
	}
	var prev []byte
	for len(b) > 0 {
		p, n := binary.Uvarint(b)
		b = b[n:]
		s, n := binary.Uvarint(b)
		b = b[n:]
		cur := append(prev[:p:p], b[:s]...)
		b = b[s:]
		size, n := binary.Uvarint(b)
		b = b[n:]
		fn(string(cur), int64(size))
		prev = cur
	}
	return nil
}

type ikBlobStats struct{ raw, rawMtime, zstd, zstdMtime, stored, blobs, lists int64 }

// ikStoreList writes one snapshot's non-semantic list; with dedup, identical lists share a blob.
func ikStoreList(tx *sql.Tx, dedup bool, snap int64, ns []ikRow, st *ikBlobStats) error {
	raw := ikEncode(ns, false)
	z := ikEnc.EncodeAll(raw, nil)
	if st != nil {
		st.raw += int64(len(raw))
		st.zstd += int64(len(z))
		rm := ikEncode(ns, true)
		st.rawMtime += int64(len(rm))
		st.zstdMtime += int64(len(ikEnc.EncodeAll(rm, nil)))
		st.lists++
	}
	if !dedup {
		if st != nil {
			st.stored += int64(len(z))
		}
		_, err := tx.Exec(`INSERT INTO snapshot_list(snapshot_id,n,data) VALUES(?,?,?)`, snap, len(ns), z)
		return err
	}
	sum := xxh3.Hash128(raw).Bytes()
	var id int64
	err := tx.QueryRow(`SELECT id FROM file_blob WHERE sum=?`, sum[:]).Scan(&id)
	if err == sql.ErrNoRows {
		r, err := tx.Exec(`INSERT INTO file_blob(sum,n,data) VALUES(?,?,?)`, sum[:], len(ns), z)
		if err != nil {
			return err
		}
		id, _ = r.LastInsertId()
		if st != nil {
			st.stored += int64(len(z))
			st.blobs++
		}
	} else if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO snapshot_list(snapshot_id,blob_id) VALUES(?,?)`, snap, id)
	return err
}

func ikConvertBlob(t *testing.T, tx *sql.Tx, l ikLayout) ikBlobStats {
	var has int // schema v27 has the columns already
	tx.QueryRow(`SELECT count(*) FROM pragma_table_info('snapshot') WHERE name='kinds'`).Scan(&has)
	if has == 0 {
		ikExec(t, tx, `ALTER TABLE snapshot ADD COLUMN kinds INTEGER NOT NULL DEFAULT 0`, `ALTER TABLE snapshot ADD COLUMN langs TEXT NOT NULL DEFAULT ''`)
	}
	if l.blob == 2 {
		ikExec(t, tx, `CREATE TABLE file_blob(id INTEGER PRIMARY KEY, sum BLOB NOT NULL UNIQUE, n INTEGER NOT NULL, data BLOB NOT NULL)`,
			`CREATE TABLE snapshot_list(snapshot_id INTEGER PRIMARY KEY, blob_id INTEGER NOT NULL REFERENCES file_blob(id))`)
	} else {
		ikExec(t, tx, `CREATE TABLE snapshot_list(snapshot_id INTEGER PRIMARY KEY, n INTEGER NOT NULL, data BLOB NOT NULL)`)
	}
	var snaps []int64
	rows, err := tx.Query(`SELECT id FROM snapshot WHERE EXISTS(SELECT 1 FROM snapshot_file f WHERE f.snapshot_id=snapshot.id) ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		snaps = append(snaps, id)
	}
	rows.Close()
	var st ikBlobStats
	for _, snap := range snaps {
		rs, err := tx.Query(`SELECT rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=? ORDER BY rel_path`, snap)
		if err != nil {
			t.Fatal(err)
		}
		var ns []ikRow
		var rels []string
		kinds := 0
		for rs.Next() {
			var r ikRow
			rs.Scan(&r.rel, &r.size, &r.mtime, &r.hash)
			kinds |= ikKinds(r.rel)
			rels = append(rels, r.rel)
			if ikBlobbed(r) {
				ns = append(ns, r)
			}
		}
		rs.Close()
		if err := ikStoreList(tx, l.blob == 2, snap, ns, &st); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE snapshot SET kinds=?, langs=? WHERE id=?`, kinds, strings.Join(ikLangs(rels), "\n"), snap); err != nil {
			t.Fatal(err)
		}
	}
	ikExec(t, tx, `DELETE FROM snapshot_file WHERE hash IS NULL AND lower(rel_path) NOT LIKE '%.dll'`)
	return st
}

// ikConvert applies a layout to a copy in one transaction: integer keys, then blobs (so the
// path table holds only what stays in rows), then paths, then slim Def rows.
func ikConvert(t *testing.T, db *sql.DB, l ikLayout) (time.Duration, ikBlobStats) {
	begin := time.Now()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var st ikBlobStats
	if l.ik {
		ikConvertInstKey(t, tx)
	}
	if l.blob != 0 {
		st = ikConvertBlob(t, tx, l)
	}
	if l.path != 0 {
		ikConvertPaths(t, tx, l.path)
	}
	if l.slim != 0 {
		ikConvertSlim(t, tx, l)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return time.Since(begin), st
}
