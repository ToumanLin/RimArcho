package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// 16 §2.18: the survey lists a list file another manager left; moving it in runs as a
// background task with an item per part, and the next survey shows it as imported. A
// second run updates the list it became.
func TestMigrationTask(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	file := filepath.Join(e.Root, "RimPy", "ModLists", "Colony.xml")
	os.MkdirAll(filepath.Dir(file), 0o755)
	write := func(ids ...string) {
		os.WriteFile(file, []byte("<ModsConfigData><activeMods><li>"+strings.Join(ids, "</li><li>")+"</li></activeMods></ModsConfigData>"), 0o644)
	}
	write("ludeon.rimworld", "tester.b")
	extra := []app.ManagerImport{{Manager: "RimPy", Kind: "list", Path: file}}
	find := func() app.MigrationPart {
		ms, err := api.ImportSurvey(extra)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			for _, g := range m.Groups {
				for _, p := range g.Parts {
					if p.Path == file {
						return p
					}
				}
			}
		}
		t.Fatalf("the list is not in the survey: %+v", ms)
		return app.MigrationPart{}
	}
	p := find()
	if p.State != app.PartNew || p.List == nil || p.List.Mods != 2 {
		t.Fatalf("before: %+v", p)
	}
	run := func(mode string) *app.Task {
		id, err := api.StartMigration([]MigrationPickIn{{ID: p.ID, Mode: mode, Name: "RimPy: Colony"}}, "RimPy")
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if tk := api.TaskByID(id); tk != nil && tk.Ended() {
				return tk
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("the task did not end")
		return nil
	}
	tk := run("")
	if tk.State != app.TaskDone || tk.Kind != "migrate" || len(tk.Items) != 1 || tk.Items[0].State != app.ItemOK || !strings.Contains(tk.Items[0].Note, "Colony") {
		t.Fatalf("task: %+v", tk)
	}
	p = find()
	if p.State != app.PartListed || p.List.Name != "Colony" || p.List.Changed {
		t.Fatalf("after: %+v %+v", p, p.List)
	}
	// the file changes; "update" replaces the list's content without opening it
	write("ludeon.rimworld", "tester.b", "brrainz.harmony")
	if !find().List.Changed {
		t.Fatal("a changed file should be marked")
	}
	if tk := run("update"); tk.State != app.TaskDone {
		t.Fatalf("update: %+v", tk)
	}
	if find().List.Changed {
		t.Fatal("after the update the file is current again")
	}
	plans, err := e.App.Plans()
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range plans {
		if pl.Name == "Colony" && pl.Entries != 3 {
			t.Fatalf("the updated list has %d entries", pl.Entries)
		}
		if pl.Name == "Colony" && pl.Open {
			t.Fatal("updating a list must not open it")
		}
	}
}
