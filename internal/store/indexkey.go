package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// indexTables are the tables a mod index is built from (app.Index): instances, their
// snapshots, sources, lineages, the player's data on lineages and the environments.
// List tables (workspace, undo, versions, plans) are not among them.
var indexTables = []string{"environment", "source", "lineage", "instance", "snapshot", "lineage_data", "lineage_tag"}

// IndexKey changes whenever the index tables may have changed: a commit by another
// connection or process (data_version), or a write by this connection to one of them,
// counted by temporary triggers. This connection's list edits leave it unchanged, so a
// list edit does not reload every mod. Without the triggers it is ChangeKey.
func (s *Store) IndexKey() string {
	var dv int64
	var token string
	var n int64
	if err := s.DB.QueryRow(`PRAGMA data_version`).Scan(&dv); err != nil {
		return s.ChangeKey()
	}
	if err := s.DB.QueryRow(`SELECT token, n FROM temp.archo_index_gen`).Scan(&token, &n); err != nil {
		// First call, or the pool opened a new connection (temporary objects are per
		// connection): install the triggers; this answer never matches an older key.
		if s.installIndexTriggers() != nil {
			return s.ChangeKey()
		}
		return "c" + s.ChangeKey()
	}
	return fmt.Sprintf("i%d.%s.%d", dv, token, n)
}

// installIndexTriggers creates the counter and its triggers on this connection. A fresh
// token keeps keys from a previous connection from matching.
func (s *Store) installIndexTriggers() error {
	b := make([]byte, 6)
	rand.Read(b)
	var sql strings.Builder
	sql.WriteString(`DROP TABLE IF EXISTS temp.archo_index_gen;
CREATE TEMP TABLE archo_index_gen(token TEXT NOT NULL, n INTEGER NOT NULL);
INSERT INTO temp.archo_index_gen VALUES('` + hex.EncodeToString(b) + `', 0);
`)
	for _, t := range indexTables {
		for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
			fmt.Fprintf(&sql, "CREATE TEMP TRIGGER IF NOT EXISTS archo_ix_%s_%s AFTER %s ON main.%s BEGIN UPDATE archo_index_gen SET n = n + 1; END;\n",
				t, strings.ToLower(op), op, t)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(sql.String()); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
