package view

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// A stopped batch whose rest a new task took over offers no second continue the rest (VM run B1:
// pressed twice, the same ten deletions ran twice and all failed).
func TestStoppedBatchHandedOverOffersNoResume(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	task := app.Task{ID: "t1", Kind: LibDelete, Lane: app.LaneFiles, State: app.TaskStopped, Total: 3, Items: []app.TaskItem{
		{Ref: "#a", Name: "A", State: app.ItemOK},
		{Ref: "#b", Name: "B", State: app.ItemSkip},
		{Ref: "#c", Name: "C", State: app.ItemSkip},
	}}
	has := func(r TaskRow, id string) bool {
		for _, a := range r.Acts {
			if a.ID == id {
				return true
			}
		}
		return false
	}
	if r := BuildTasks([]app.Task{task}, time.Now()).Rows[0]; !has(r, "resume") {
		t.Fatalf("no resume before: %+v", r.Acts)
	}
	task.Items[1].Again, task.Items[2].Again = "t2", "t2"
	r := BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if has(r, "resume") {
		t.Fatalf("resume offered again: %+v", r.Acts)
	}
	if !strings.Contains(r.Line, "2 项已交给新的任务") || r.Items[1].Why != "已交给新的任务" {
		t.Fatalf("line %q items %+v", r.Line, r.Items)
	}
}

// A pack build the window left running in its own process says so, points to the Packs page
// for how it ended, and marks no step failed (VM round 2026-11-10: it read "stopped" with the
// copy step failed, though the build finished; 10 K24).
func TestDetachedPackBuildNotStopped(t *testing.T) {
	task := app.Task{ID: "p1", Kind: TaskPack, Lane: app.LaneSelf, State: app.TaskStopped, Closed: true, Detached: true, Step: 2}
	r := BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if r.StateText != T("went on in the background") || strings.Contains(r.Say, T("stopped")) || !strings.Contains(r.Line, "Packs page") {
		t.Fatalf("state %q line %q say %q", r.StateText, r.Line, r.Say)
	}
	for _, s := range r.Steps {
		if s.State == app.ItemFail {
			t.Fatalf("a step marked failed: %+v", r.Steps)
		}
	}
}
