package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSourceLibrarySettingTaskStopRetryAndHistory(t *testing.T) {
	e, api, events := fixture(t)
	e.WriteFile(filepath.Join(e.Workshop, "1001", "Assemblies", "Alpha.dll"), "synthetic assembly")
	e.MustRun("scan")
	started := make(chan struct{})
	e.App.Decompile = func(ctx context.Context, _, out, typ string, _ []string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	out, err := api.ApplySetting([]string{"source", "decompile", "--active"}, "")
	if err != nil || len(out) != 1 {
		t.Fatalf("queue %v %v", out, err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start")
	}
	settings := make(chan []view.SettingGroup, 1)
	go func() { settings <- api.Settings() }()
	select {
	case groups := <-settings:
		found := 0
		for _, g := range groups {
			for _, it := range g.Items {
				if it.Directory == "source" {
					found++
					if len(it.Actions) != 2 || it.ValueNote == "" {
						t.Fatalf("library row %+v", it)
					}
				}
				if strings.Join(it.Argv, " ") == "cache clear source" {
					t.Fatal("clearable decompile cache")
				}
			}
		}
		if found != 1 {
			t.Fatalf("library rows %d", found)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decompile blocked Settings")
	}
	var id string
	for _, task := range api.tasks.List() {
		if task.Kind == view.TaskSourceDecompile {
			id = task.ID
			if task.Total != 1 {
				t.Fatalf("missing progress %+v", task)
			}
		}
	}
	if _, err := api.TaskAct("stop", []string{id}, ""); err != nil {
		t.Fatal(err)
	}
	r := endedTask(t, api, view.TaskSourceDecompile)
	if r.State != app.TaskStopped {
		t.Fatalf("stop result %+v", r)
	}
	waitSettingEnd(t, events, id)
	e.App.Decompile = func(_ context.Context, _, out, typ string, _ []string) error {
		return os.WriteFile(filepath.Join(out, "Alpha.cs"), []byte("class Alpha {}"), 0o644)
	}
	if _, err := api.TaskAct("retry", []string{id}, ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var complete bool
		for _, task := range api.tasks.List() {
			if task.Kind == view.TaskSourceDecompile && task.ID != id && task.State == app.TaskDone {
				complete = true
			}
		}
		if complete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retry did not complete: %+v", api.tasks.List())
		}
		time.Sleep(5 * time.Millisecond)
	}
	var history []app.Task
	b, err := os.ReadFile(filepath.Join(e.Home, "tasks.json"))
	if err != nil || json.Unmarshal(b, &history) != nil || len(history) != 2 {
		t.Fatalf("history %s %v", b, err)
	}
	p, err := api.SourceLibraryUnused()
	if err != nil || p.Folders != 0 {
		t.Fatalf("installed library marked unused %+v %v", p, err)
	}
}
