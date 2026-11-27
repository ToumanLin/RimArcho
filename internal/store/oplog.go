package store

import "database/sql"

// opLogSchema is the operation log (04 §4, 02 §1): one row per action RimArcho took on the
// player's files, written and read by internal/oplog.
const opLogSchema = `CREATE TABLE IF NOT EXISTS op_log (
	id INTEGER PRIMARY KEY,
	at INTEGER NOT NULL,
	origin TEXT NOT NULL,
	env_id TEXT NOT NULL DEFAULT '',
	op TEXT NOT NULL,
	subject TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT '',
	batch INTEGER NOT NULL DEFAULT 0,
	ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS op_log_at ON op_log(at);
`

func upgradeOpLog(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(opLogSchema)
	return err
}

// upgradeOpLogDuration adds op_log.ms, how long a Begin row's action took (13 §1.3). A
// database older than 43 already has it: step 43 creates the table from opLogSchema.
func upgradeOpLogDuration(tx *sql.Tx, _ func(int64, int64)) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('op_log') WHERE name = 'ms'`).Scan(&n); err != nil || n > 0 {
		return err
	}
	_, err := tx.Exec(`ALTER TABLE op_log ADD COLUMN ms INTEGER NOT NULL DEFAULT 0`)
	return err
}
