package store

import (
	"context"
	"database/sql"
)

// schemaV25 shrinks the fact indexes (02 §7, 03 §3). Every fact query filters on game_mm,
// so the single-column indexes from before v13 are never chosen; the (instance_id, game_mm)
// ones cover the per-instance deletes. def_fact_mm_named is created again: two branches
// both shipped a v15 on 2026-10-22, and databases that took the texture one never got it.
const schemaV25 = `
DROP INDEX IF EXISTS def_fact_name;
DROP INDEX IF EXISTS type_fact_name;
DROP INDEX IF EXISTS type_fact_instance;
DROP INDEX IF EXISTS harmony_fact_instance;
DROP INDEX IF EXISTS harmony_fact_target;
CREATE INDEX IF NOT EXISTS def_fact_mm_named ON def_fact(game_mm, name) WHERE name<>'';
`

// migrateV25 compresses the log reports stored before v25 (03 §6.5).
func migrateV25(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT path, report FROM log_cache`)
	if err != nil {
		return err
	}
	type row struct {
		path   string
		report []byte
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.path, &r.report); err != nil {
			rows.Close()
			return err
		}
		if !isGzip(r.report) {
			todo = append(todo, r)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range todo {
		z, err := gzipBytes(r.report)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE log_cache SET report=? WHERE path=?`, z, r.path); err != nil {
			return err
		}
	}
	return nil
}

// TruncateWAL checkpoints the write-ahead log and truncates it to zero bytes, after a write
// that may have grown it far past journal_size_limit (an analysis, a scan that wrote new
// snapshots). It does not wait: while another connection still reads an older state, the
// checkpoint stops early and the next reset truncates the file instead (02 §7).
func (s *Store) TruncateWAL() {
	ctx := context.Background()
	c, err := s.DB.Conn(ctx)
	if err != nil {
		return
	}
	defer c.Close()
	c.ExecContext(ctx, `PRAGMA busy_timeout=0`)
	c.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	c.ExecContext(ctx, `PRAGMA busy_timeout=5000`)
}
