package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMigrateV20StageHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO environment(id,name,game_root,user_data_root,created_at) VALUES('e','e','g','u',0)`); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlan("e", "test", fixed)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PushStep(p.ID, "undo", `[]`, "legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE workspace_undo DROP COLUMN staging; UPDATE meta SET value='20' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	steps, err := s.Steps(p.ID, "undo")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Staging != nil || steps[0].Entries != "[]" {
		t.Fatalf("migration lost legacy step: %+v", steps)
	}
	if s.Meta("schema_version") != fmt.Sprint(SchemaVersion) {
		t.Fatal("migration not completed")
	}
}

func TestStageHistoryRemapItemsAndAnchors(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(`INSERT INTO environment(id,name,game_root,user_data_root,created_at) VALUES('e','e','g','u',0)`); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlan("e", "test", fixed)
	if err != nil {
		t.Fatal(err)
	}
	raw := `[{"i":"old","p":7,"a":"anchor"},{"i":"other","p":8,"a":"old"}]`
	if err := s.SetMeta(StageKey(p.ID), raw); err != nil {
		t.Fatal(err)
	}
	if err := s.PushStep(p.ID, "undo", `[]`, "stage"); err != nil {
		t.Fatal(err)
	}
	if err := s.inTx(func(tx *sql.Tx) error {
		return remapHistoryStages(tx, map[string]string{"old": "new", "anchor": "new-anchor"}, &p.ID)
	}); err != nil {
		t.Fatal(err)
	}
	steps, err := s.Steps(p.ID, "undo")
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"a":"new-anchor","i":"new","p":7},{"a":"new","i":"other","p":8}]`
	if *steps[0].Staging != want {
		t.Fatalf("remap staging: %s", *steps[0].Staging)
	}
}
