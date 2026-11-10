package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// indexTables are the tables a mod index is built from (app.Index): instances, their
// snapshots, sources, lineages, the player's data on lineages (tag colours too: the rows
// show them) and the environments.
// List tables (workspace, undo, versions, plans) are not among them.
var indexTables = []string{"environment", "source", "lineage", "instance", "snapshot", "lineage_data", "lineage_tag", "tag_color"}

// IndexKey changes only when an index table changes, including writes by another
// connection/process. data_version cannot distinguish a list edit from a scan.
// The durable triggers let WAL readers retain their index across editor commits.
func (s *Store) IndexKey() string {
	var token string
	var n int64
	if err := s.DB.QueryRow(`SELECT token, n FROM archo_index_gen`).Scan(&token, &n); err != nil {
		return "c" + s.ChangeKey()
	}
	return fmt.Sprintf("i%s.%d", token, n)
}

// Only Open installs the shared counter. OpenSession and IndexKey never write,
// including when a different connection is holding a WAL write transaction.
func (s *Store) installIndexTriggers() error {
	b := make([]byte, 6)
	rand.Read(b)
	var sql strings.Builder
	sql.WriteString(`CREATE TABLE IF NOT EXISTS archo_index_gen(token TEXT NOT NULL, n INTEGER NOT NULL);
INSERT INTO archo_index_gen SELECT '` + hex.EncodeToString(b) + `', 0 WHERE NOT EXISTS(SELECT 1 FROM archo_index_gen);
`)
	for _, t := range indexTables {
		for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
			fmt.Fprintf(&sql, "CREATE TRIGGER IF NOT EXISTS archo_ix_%s_%s AFTER %s ON %s BEGIN UPDATE archo_index_gen SET n = n + 1; END;\n",
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
