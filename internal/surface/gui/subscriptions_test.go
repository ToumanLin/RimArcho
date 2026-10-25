package gui

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSubscriptionSourceTaskAndRunningGuard(t *testing.T) {
	e, api, _ := fixture(t)
	if _, err := api.CheckSubscriptionSource("invalid"); err == nil || len(api.tasks.List()) != 0 {
		t.Fatal("invalid source queued")
	}
	p, err := api.SubscriptionsPage(false, "")
	if err != nil || len(p.Sources) != 5 || p.History == nil {
		t.Fatalf("page: %+v %v", p, err)
	}
	if _, err := api.CheckSubscriptionSource("git"); err != nil {
		t.Fatal(err)
	}
	task := endedTask(t, api, view.TaskSourceCheck)
	if task.State != app.TaskDone {
		t.Fatalf("check: %+v", task)
	}
	if e.App.SubscriptionSourceCheck("git").At.IsZero() {
		t.Fatal("check did not persist its result")
	}
	if got := task.Target; got != view.PageChanges {
		t.Fatalf("task o: %q", got)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := api.WorkshopUpdatePreview("ws:1001"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running preview: %v", err)
	}
	if _, err := api.WorkshopUpdate("ws:1001"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running apply: %v", err)
	}
}
