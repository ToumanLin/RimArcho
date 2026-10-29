package store

import (
	"os"
	"path/filepath"
	"testing"
)

func downgradeFacts(t *testing.T, s *Store) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "store", "facts-v25.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(string(data)); err != nil {
		t.Fatal(err)
	}
}
