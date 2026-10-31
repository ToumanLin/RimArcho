package store

import (
	"database/sql"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

// For the external benchmarks (v28_bench_test.go).

// NewV27DB creates a database as a v27 build did.
func NewV27DB(t *testing.T, path string) *sql.DB { return v27Store(t, path) }

// InsertFilesTx writes a snapshot's manifest as a scan does.
func InsertFilesTx(tx *sql.Tx, snap int64, files []scan.File) error {
	return insertFiles(tx, snap, files)
}

// DropSetFileCache empties the fact sets' file-list cache.
func (s *Store) DropSetFileCache() {
	if s.setFiles != nil {
		s.setFiles.mu.Lock()
		s.setFiles.sets, s.setFiles.n, s.setFiles.key = nil, 0, changeKey{}
		s.setFiles.mu.Unlock()
	}
}
