package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Refused moves into the lock library and partly finished deletions read in the player's
// language, with each path ending at punctuation so the status line's copy button copies
// only the path (VM round 2026-11-10: both showed in English in the Chinese window).
func TestMoveAndDeleteFailuresTranslated(t *testing.T) {
	defer view.SetLang("en")
	view.SetLang("zh")
	const dir, file = `C:\Mods\Archo Test`, `C:\Mods\Archo Test\Textures\a.txt`
	for _, msg := range []string{
		dir + ": only part of it was deleted (remove " + file + ": The process cannot access the file); the game links to it were removed, delete the rest yourself",
		".rimarcho/origin.json could not be written in " + dir + "; nothing was moved: mkdir " + dir + `\.rimarcho: denied`,
		"the copy of " + dir + " is incomplete; nothing was moved",
		"the copy of " + dir + " differs at About/About.xml; nothing was moved",
		dir + " already exists; nothing was moved",
	} {
		got := view.Text(msg)
		if strings.Contains(got, "nothing was moved") || strings.Contains(got, "only part") || !strings.Contains(got, dir) {
			t.Errorf("%q\n  -> %q", msg, got)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("format arguments lost: %q", got)
		}
	}
	// A task of one item that failed says the item's reason in the status line, translated
	// as its row in Tasks is (VM round: the quick delete's line stayed English).
	why := dir + ": only part of it was deleted (in use); the game links to it were removed, delete the rest yourself"
	task := app.Task{ID: "t1", Kind: view.OpSubRemoveFast, Lane: app.LaneFiles, State: app.TaskFailed, Total: 1, Items: []app.TaskItem{{Ref: "#a", Name: "A", State: app.ItemFail, Why: why}}}
	row := view.BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if row.Line != row.Items[0].Why || strings.Contains(row.Line, "only part") || !strings.Contains(row.Line, dir) {
		t.Errorf("line %q, item %q", row.Line, row.Items[0].Why)
	}
	view.SetLang("en")
	msg := dir + " already exists; nothing was moved"
	if got := view.Text(msg); got != msg {
		t.Errorf("English changed: %q", got)
	}
}
