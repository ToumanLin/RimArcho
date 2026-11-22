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
	batch INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS op_log_at ON op_log(at);
`

func upgradeOpLog(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(opLogSchema)
	return err
}
