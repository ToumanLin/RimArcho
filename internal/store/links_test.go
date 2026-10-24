package store

import (
	"path/filepath"
	"testing"
)

func TestMigrateV18LinkLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DROP TABLE managed_link; UPDATE meta SET value='17' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err = s.SetMeta("migration-keeper", "preserved"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Meta("schema_version") != "18" || s.Meta("migration-keeper") != "preserved" {
		t.Fatal("migration lost facts")
	}
	row := ManagedLink{Operation: "test", Mods: filepath.Join(t.TempDir(), "Mods"), Name: "_Archo_env_test", Kind: "junction", Instance: "test", Target: "original", NextPath: "entry", NextTarget: "destination"}
	if err = s.RecordLinkIntent([]ManagedLink{row}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ManagedLinks()
	if err != nil || len(rows) != 1 || rows[0].Phase != "intent" {
		t.Fatalf("intent %+v %v", rows, err)
	}
	if err = s.LinkPhase("test", "complete"); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ManagedLinks()
	if err != nil || len(rows) != 1 || rows[0].Phase != "complete" {
		t.Fatalf("completion %+v %v", rows, err)
	}
}
