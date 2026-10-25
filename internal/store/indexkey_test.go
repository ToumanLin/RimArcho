package store

import (
	"path/filepath"
	"testing"
)

// A list edit keeps the index key; a write to an index table, by this connection or
// another, changes it.
func TestIndexKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	k0 := s.IndexKey()
	if k0[0] != 'i' {
		t.Fatalf("triggers not installed: %q", k0)
	}
	if _, err := s.DB.Exec(`INSERT INTO workspace_undo(plan_id, stack, entries, op) VALUES(1, 'undo', '[]', 'test')`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("anything", "x"); err != nil {
		t.Fatal(err)
	}
	if k := s.IndexKey(); k != k0 {
		t.Fatalf("list/meta write changed the index key: %q → %q", k0, k)
	}
	if _, err := s.DB.Exec(`INSERT INTO lineage_data(environment_id, lineage_id, alias) VALUES('', 'L1', 'a')`); err != nil {
		t.Fatal(err)
	}
	k1 := s.IndexKey()
	if k1 == k0 {
		t.Fatal("own write to lineage_data kept the index key")
	}
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.SetMeta("from-another-connection", "x"); err != nil {
		t.Fatal(err)
	}
	if k := s.IndexKey(); k == k1 {
		t.Fatal("another connection's commit kept the index key")
	}
}
