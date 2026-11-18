package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// indexTables are the tables a mod index is built from (app.Index): instances, their
// snapshots, sources, lineages, the player's data on lineages (tag colours too: the rows
// show them), installations and environments.
// List tables (workspace, undo, versions, plans) are not among them.
var indexTables = []string{"game", "environment", "source", "lineage", "instance", "snapshot", "lineage_data", "lineage_tag", "tag_color"}

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
func indexTriggerSQL() string {
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
	return sql.String()
}

func (s *Store) installIndexTriggers() error {
	// Almost always everything is there already: say so from a read, which never waits for
	// another process's write. A deferred transaction that reads first and then writes gets
	// BUSY at once when another process committed in between, whatever busy_timeout says
	// (VM 2026-11-10: the TUI's scan failed while the window was writing).
	if s.indexTriggersInstalled() {
		return nil
	}
	var err error
	for try := 0; try < 20; try++ {
		if err = s.execTx(indexTriggerSQL()); err == nil || !isBusy(err) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func (s *Store) execTx(q string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(q); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// indexTriggersInstalled: the counter row and every trigger exist.
func (s *Store) indexTriggersInstalled() bool {
	var rows, triggers int
	if s.DB.QueryRow(`SELECT count(*) FROM archo_index_gen`).Scan(&rows) != nil || rows == 0 {
		return false
	}
	if s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'archo!_ix!_%' ESCAPE '!'`).Scan(&triggers) != nil {
		return false
	}
	return triggers == len(indexTables)*3
}

func isBusy(err error) bool {
	m := err.Error()
	return strings.Contains(m, "SQLITE_BUSY") || strings.Contains(m, "database is locked")
}
