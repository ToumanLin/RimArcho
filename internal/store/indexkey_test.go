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
	if k := s.IndexKey(); k != k1 {
		t.Fatal("another connection's unrelated commit invalidated the index")
	}
	if err := other.SetAlias("", "L1", "renamed"); err != nil {
		t.Fatal(err)
	}
	if k := s.IndexKey(); k == k1 || k != other.IndexKey() {
		t.Fatal("index table write did not publish the same generation to both sessions")
	}
	key := s.IndexKey()
	tx, err := other.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE lineage_data SET alias='uncommitted'`); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if k := reader.IndexKey(); k != key {
		t.Fatal("new WAL reader wrote or observed an uncommitted generation")
	}
}
