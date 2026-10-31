package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
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
	downgradeFacts(t, s)
	if _, err := s.DB.Exec(`ALTER TABLE snapshot DROP COLUMN kinds; ALTER TABLE snapshot DROP COLUMN langs; ALTER TABLE identity_question DROP COLUMN summary; ALTER TABLE workspace_undo DROP COLUMN staging; UPDATE meta SET value='20' WHERE key='schema_version'`); err != nil {
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

// A background analyzer owns a separate SQLite connection and can commit while
// an editor transaction is reading. Writer reservation must precede those reads.
func TestStageHistoryWaitsForBackgroundWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
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
	w := &Workspace{PlanID: p.ID, Entries: `[]`, Updated: fixed}
	if err := s.SetWorkspace(w); err != nil {
		t.Fatal(err)
	}
	bg, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bg.Close()
	wait := func(edit func() error) {
		t.Helper()
		tx, err := bg.DB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE meta SET value=value WHERE key='schema_version'`); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- edit() }()
		select {
		case err := <-done:
			t.Fatalf("history edit should wait for background writer, got %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("history edit failed after background commit: %v", err)
		}
	}
	staged := `[{"i":"mod","p":2,"a":"anchor"}]`
	wait(func() error { return s.EditWorkspace(w, `[]`, "stage", &staged) })
	step, err := s.PeekStep(p.ID, "undo")
	if err != nil {
		t.Fatal(err)
	}
	wait(func() error { return s.RestoreStep(w, `[]`, "undo", "redo", *step) })
	if s.Meta(StageKey(p.ID)) != "" {
		t.Fatal("undo failed to restore staging")
	}
	wait(func() error { return s.PushStep(p.ID, "undo", `[]`, "fixture") })
}
