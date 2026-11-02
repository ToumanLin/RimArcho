package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// schemaV28 (02 §7, 03 §3): non-semantic manifest files other than DLLs move into one
// compressed block per snapshot (snapshot_list), and fact rows name their file by an index
// into their set's file list (fact_set.files). fact_set is the parent of the fact tables, so
// it only gains a column; the child tables are rebuilt by migrateV28, which also adds their
// indexes after the copy.
const schemaV28 = `
CREATE TABLE snapshot_list(snapshot_id INTEGER PRIMARY KEY, n INTEGER NOT NULL, data BLOB NOT NULL);
ALTER TABLE fact_set ADD COLUMN files TEXT NOT NULL DEFAULT '[]';
ALTER TABLE def_fact RENAME TO def_fact_v27;
ALTER TABLE assembly_fact RENAME TO assembly_fact_v27;
ALTER TABLE type_fact RENAME TO type_fact_v27;
ALTER TABLE harmony_fact RENAME TO harmony_fact_v27;
ALTER TABLE harmony_id RENAME TO harmony_id_v27;
CREATE TABLE def_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 def_type TEXT NOT NULL, def_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
 parent TEXT NOT NULL DEFAULT '', abstract INTEGER NOT NULL DEFAULT 0, file_ix INTEGER NOT NULL, line INTEGER NOT NULL,
 may_require TEXT NOT NULL DEFAULT '', may_require_any TEXT NOT NULL DEFAULT '');
CREATE TABLE assembly_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_ix INTEGER NOT NULL, skipped INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0,
 uses_harmony INTEGER NOT NULL DEFAULT 0, types INTEGER NOT NULL DEFAULT 0, patches INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '');
CREATE TABLE type_fact(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file_ix INTEGER NOT NULL, type_name TEXT NOT NULL);
CREATE TABLE harmony_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_ix INTEGER NOT NULL, patch_type TEXT NOT NULL, patch_method TEXT NOT NULL, kind TEXT NOT NULL,
 target TEXT NOT NULL, target_type TEXT NOT NULL DEFAULT '', target_method TEXT NOT NULL DEFAULT '', args TEXT NOT NULL DEFAULT '',
 skips INTEGER NOT NULL DEFAULT 0, dynamic INTEGER NOT NULL DEFAULT 0);
CREATE TABLE harmony_id(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file_ix INTEGER NOT NULL, id TEXT NOT NULL COLLATE NOCASE);
`

// factIndexesV28 are schemaV26's indexes, made after the rows are copied.
const factIndexesV28 = `
CREATE INDEX def_fact_set ON def_fact(set_id);
CREATE INDEX def_fact_lookup ON def_fact(def_name,set_id);
CREATE INDEX def_fact_named ON def_fact(name,set_id) WHERE name<>'';
CREATE INDEX assembly_fact_set ON assembly_fact(set_id);
CREATE INDEX type_fact_set ON type_fact(set_id);
CREATE INDEX type_fact_lookup ON type_fact(type_name,set_id);
CREATE INDEX harmony_fact_set ON harmony_fact(set_id);
CREATE INDEX harmony_id_set ON harmony_id(set_id);
CREATE INDEX harmony_id_lookup ON harmony_id(id,set_id);
`

// factTablesV28: each fact table's columns besides set_id and its file, in the order the
// file lists are built (a DLL set's list is its assemblies' files, then any other).
var factTablesV28 = []struct{ table, kind, columns string }{
	{"def_fact", "def", "def_type,def_name,name,parent,abstract,line,may_require,may_require_any"},
	{"assembly_fact", "dll", "skipped,manual,uses_harmony,types,patches,error"},
	{"type_fact", "dll", "type_name"},
	{"harmony_fact", "dll", "patch_type,patch_method,kind,target,target_type,target_method,args,skips,dynamic"},
	{"harmony_id", "dll", "id"},
}

// migrateV28 moves the files and converts the fact rows in the upgrade's transaction, without
// VACUUM (the next maintenance reclaims the space, 02 §7.2). report gets the rows done of the
// rows to do (manifest rows, then fact rows); it may be nil.
func migrateV28(tx *sql.Tx, report func(done, total int64)) error {
	if report == nil {
		report = func(int64, int64) {}
	}
	var files, facts int64
	if err := tx.QueryRow(`SELECT count(*) FROM snapshot_file`).Scan(&files); err != nil {
		return err
	}
	for _, t := range factTablesV28 {
		var n int64
		if err := tx.QueryRow(`SELECT count(*) FROM ` + t.table + `_v27`).Scan(&n); err != nil {
			return err
		}
		facts += n
	}
	total := files + facts
	report(0, total)
	if err := migrateListsV28(tx, func(done int64) { report(done, total) }); err != nil {
		return err
	}
	report(files, total)
	return migrateFactFilesV28(tx, func(done int64) { report(files+done, total) })
}

// migrateListsV28 builds every snapshot's block from its rows, then deletes those rows.
func migrateListsV28(tx *sql.Tx, progress func(done int64)) error {
	type block struct {
		snap int64
		n    int
		data []byte
	}
	var blocks []block
	var cur int64
	var es []listEntry
	var moved, done int64
	flush := func() error {
		if len(es) == 0 {
			return nil
		}
		data, err := encodeList(es)
		if err != nil {
			return err
		}
		blocks = append(blocks, block{cur, len(es), data})
		moved += int64(len(es))
		es = es[:0]
		return nil
	}
	rows, err := tx.Query(`SELECT snapshot_id, rel_path, size, hash IS NOT NULL FROM snapshot_file ORDER BY snapshot_id, rel_path`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var snap, size int64
		var rel string
		var hashed bool
		if err := rows.Scan(&snap, &rel, &size, &hashed); err != nil {
			rows.Close()
			return err
		}
		if snap != cur {
			if err := flush(); err != nil {
				rows.Close()
				return err
			}
			cur = snap
		}
		if listed(rel, hashed) {
			es = append(es, listEntry{rel, size})
		}
		if done++; done%50000 == 0 {
			progress(done)
		}
	}
	err = rows.Err()
	rows.Close()
	if err == nil {
		err = flush()
	}
	if err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT INTO snapshot_list(snapshot_id, n, data) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, b := range blocks {
		if _, err := st.Exec(b.snap, b.n, b.data); err != nil {
			return err
		}
	}
	// The same rule as listed: no hash, and not .dll (LIKE folds ASCII letters).
	res, err := tx.Exec(`DELETE FROM snapshot_file WHERE hash IS NULL AND rel_path NOT LIKE '%.dll'`)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != moved {
		return fmt.Errorf("store: v28 moved %d files into blocks but deleted %d rows (%v)", moved, n, err)
	}
	return nil
}

// migrateFactFilesV28 gives every fact set its file list and copies the fact rows with
// indexes into it, keeping their rowids (queries order by them). Rows whose set is gone have
// no reader (content_state references sets) and are not copied.
func migrateFactFilesV28(tx *sql.Tx, progress func(done int64)) error {
	lists := map[int64]*fileIndex{}
	var order []int64
	if err := eachRow(tx, `SELECT id FROM fact_set ORDER BY id`, func(r *sql.Rows) error {
		var id int64
		if err := r.Scan(&id); err != nil {
			return err
		}
		lists[id] = &fileIndex{}
		order = append(order, id)
		return nil
	}); err != nil {
		return err
	}
	for _, t := range factTablesV28 {
		if err := eachRow(tx, `SELECT set_id, file FROM `+t.table+`_v27 ORDER BY rowid`, func(r *sql.Rows) error {
			var set int64
			var file string
			if err := r.Scan(&set, &file); err != nil {
				return err
			}
			if x := lists[set]; x != nil {
				x.of(file)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE TEMP TABLE set_file_v28(set_id INTEGER NOT NULL, file TEXT NOT NULL, ix INTEGER NOT NULL, PRIMARY KEY(set_id, file)) WITHOUT ROWID`); err != nil {
		return err
	}
	upd, err := tx.Prepare(`UPDATE fact_set SET files=? WHERE id=?`)
	if err != nil {
		return err
	}
	defer upd.Close()
	ins, err := tx.Prepare(`INSERT INTO set_file_v28(set_id, file, ix) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, id := range order {
		x := lists[id]
		if _, err := upd.Exec(x.json(), id); err != nil {
			return err
		}
		for i, f := range x.files {
			if _, err := ins.Exec(id, f, i); err != nil {
				return err
			}
		}
	}
	// The old tables' indexes go first: the new tables reuse their names.
	var drop []string
	for _, t := range factTablesV28 {
		if err := eachRow(tx, `SELECT name FROM sqlite_master WHERE type='index' AND tbl_name=? AND sql IS NOT NULL`, func(r *sql.Rows) error {
			var name string
			if err := r.Scan(&name); err != nil {
				return err
			}
			drop = append(drop, name)
			return nil
		}, t.table+"_v27"); err != nil {
			return err
		}
	}
	for _, name := range drop {
		if _, err := tx.Exec(`DROP INDEX "` + strings.ReplaceAll(name, `"`, `""`) + `"`); err != nil {
			return err
		}
	}
	var done int64
	const chunk = 50000
	for _, t := range factTablesV28 {
		var last int64
		if err := tx.QueryRow(`SELECT coalesce(max(rowid),0) FROM ` + t.table + `_v27`).Scan(&last); err != nil {
			return err
		}
		cols := "x." + strings.ReplaceAll(t.columns, ",", ",x.")
		q := `INSERT INTO ` + t.table + `(rowid,set_id,file_ix,` + t.columns + `) SELECT x.rowid,x.set_id,p.ix,` + cols +
			` FROM ` + t.table + `_v27 x JOIN set_file_v28 p ON p.set_id=x.set_id AND p.file=x.file WHERE x.rowid>? AND x.rowid<=? ORDER BY x.rowid`
		for from := int64(0); from < last; from += chunk {
			res, err := tx.Exec(q, from, from+chunk)
			if err != nil {
				return fmt.Errorf("store: v28 %s: %w", t.table, err)
			}
			n, _ := res.RowsAffected()
			done += n
			progress(done)
		}
		if _, err := tx.Exec(`DROP TABLE ` + t.table + `_v27`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DROP TABLE set_file_v28`); err != nil {
		return err
	}
	_, err = tx.Exec(factIndexesV28)
	return err
}
