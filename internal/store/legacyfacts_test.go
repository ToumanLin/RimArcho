package store

import (
	"os"
	"path/filepath"
	"testing"
)

func downgradeFacts(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.DB.Exec(`DROP TABLE source_library; ALTER TABLE source_use DROP COLUMN mode; ALTER TABLE source_use RENAME TO source_cache`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "store", "facts-v25.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(string(data)); err != nil {
		t.Fatal(err)
	}
}
