package gui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// A job's end can arrive while the player does something else, so the status line names the
// job (06 §11 A12): a failed pack build says it was the pack build and which pack, while the
// task record, which shows the job beside it, keeps the bare reason. Texture cleanup, which
// has no event of its own, says how it ended through the task record.
func TestBackgroundEndsNameTheirJob(t *testing.T) {
	_, api, events := fixture(t)
	wait := func(prefix, want string) string {
		deadline := time.After(10 * time.Second)
		for {
			select {
			case ev := <-events:
				if strings.HasPrefix(ev, prefix) && strings.Contains(ev, want) {
					return ev
				}
			case <-deadline:
				t.Fatalf("no %s event with %s", prefix, want)
			}
		}
	}
	api.PackRunner = func(string) error { return errors.New("disk full") }
	if _, err := api.PackBuild("named", 0); err != nil {
		t.Fatal(err)
	}
	ev := wait("pack ", `"done":true`)
	if !strings.Contains(ev, view.TaskTitle(view.TaskPack)) || !strings.Contains(ev, "named: disk full") {
		t.Fatalf("pack end does not name its job: %s", ev)
	}
	for _, r := range api.Tasks().Rows {
		if r.Kind == view.TaskPack && strings.Contains(r.Line+r.Say, view.TaskTitle(view.TaskPack)) {
			t.Fatalf("task record repeats the job: %+v", r)
		}
	}
	if _, err := api.TexturesClean("plan"); err != nil {
		t.Fatal(err)
	}
	ev = wait("tasks ", `"ended"`)
	if !strings.Contains(ev, view.OpTexClean) || !strings.Contains(ev, view.TaskTitle(view.TaskTexClean)) {
		t.Fatalf("texture cleanup end: %s", ev)
	}
}
