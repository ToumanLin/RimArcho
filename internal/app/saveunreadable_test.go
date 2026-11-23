package app_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// A save whose start cannot be read (VM 2026-11-10: a file whose permissions deny reading)
// was shown as "loads with <list> as it is" next to its own error row, and its content
// card waited for a read that never came.
func TestCheckSaveUnreadableHead(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.UserData, "Saves", "Garbage.rws"), "not a save")
	s, _, err := e.App.FindSave("Garbage")
	if err != nil {
		t.Fatal(err)
	}
	if s.Head != nil {
		t.Fatalf("head read from garbage: %+v", s.Head)
	}
	c, err := e.App.CheckSave(s)
	if err != nil {
		t.Fatal(err)
	}
	if c.Level != app.LevelError || len(c.Issues) == 0 || c.Issues[0].Kind != app.IssueUnreadable {
		t.Fatalf("level %q issues %+v", c.Level, c.Issues)
	}
	content, err := e.App.SaveContent(s)
	if err != nil {
		t.Fatal(err)
	}
	if content.Stale || content.Note != "The save's Mod list could not be read." {
		t.Fatalf("content %+v", content)
	}
}
