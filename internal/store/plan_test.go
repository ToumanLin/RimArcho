package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixed = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// A plan's write path: deleting it removes its data, and the current plan falls back.
func TestPlanStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rimarcho.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.DB.Exec(`INSERT INTO environment(id, name, game_root, user_data_root, created_at) VALUES ('e', 'e', 'g', 'u', 0)`)
	if p, _ := s.CurrentPlan("e"); p != nil {
		t.Fatal("no plan yet")
	}
	a, err := s.CreatePlan("e", "a", fixed)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreatePlan("e", "b", fixed)
	if _, err := s.CreatePlan("e", "a", fixed); err == nil {
		t.Fatal("names are unique per environment")
	}
	if p, _ := s.CurrentPlan("e"); p != nil {
		t.Fatal("a game that never chose a list has none: its first use imports ModsConfig.xml")
	}
	s.SetCurrentPlan("e", b.ID)
	s.SetWorkspace(&Workspace{PlanID: b.ID, Entries: "x", Updated: fixed.Add(time.Hour)})
	s.PushStep(b.ID, "undo", "y", "enable")
	s.AddVersion(b.ID, &Version{Source: "import", Created: fixed})
	s.SetMeta(StageKey(b.ID), "[]")
	if p, _ := s.PlanByID(b.ID); !p.Updated.Equal(fixed.Add(time.Hour)) {
		t.Fatal("updated_at follows the workspace")
	}
	if p, _ := s.PlanByName("B"); p == nil || p.ID != b.ID {
		t.Fatal("case-insensitive lookup")
	}
	if err := s.DeletePlan(b.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ := s.Workspace(b.ID); w != nil || s.StepCount(b.ID, "undo") != 0 || s.VersionCount(b.ID) != 0 || s.Meta(StageKey(b.ID)) != "" {
		t.Fatal("a removed plan's data is gone")
	}
	if p, _ := s.CurrentPlan("e"); p == nil || p.ID != a.ID {
		t.Fatal("the current plan falls back to the first")
	}
}

// A database from a development build before 1.0.0 is refused, not upgraded, and left as it
// was; a current one opens again (02 §7).
func TestPreReleaseDatabaseRefused(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal("current schema:", err)
	}
	if err := s.SetMeta("schema_version", fmt.Sprint(FirstReleaseSchema-1)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	_, err = Open(path)
	if !errors.Is(err, ErrPreRelease) || !strings.Contains(err.Error(), path) {
		t.Fatalf("pre-1.0 database: %v", err)
	}
	if v := PeekMeta(path, "schema_version"); v != fmt.Sprint(FirstReleaseSchema-1) {
		t.Fatalf("refused database changed: schema %q", v)
	}
}
