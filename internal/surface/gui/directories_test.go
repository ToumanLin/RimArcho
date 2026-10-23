package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestDirectoryAPIRecoversMovedWorkshop(t *testing.T) {
	e, api, _ := fixture(t)
	dirs, err := api.Directories()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, d := range dirs {
		if d.Kind == "workshop" {
			id = d.ID
		}
	}
	if id == "" {
		t.Fatal("workshop not listed")
	}
	api.Board() // warm the board before the files move
	moved := filepath.Join(e.Root, "moved-workshop")
	if err := os.Rename(e.Workshop, moved); err != nil {
		t.Fatal(err)
	}
	dirs, err = api.Directories()
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, d := range dirs {
		missing = missing || d.ID == id && d.Status == "missing"
	}
	if !missing {
		t.Fatal("directory check relied on the database cache")
	}
	n := api.Needs()
	found := false
	for _, item := range n.Items {
		for _, action := range item.Actions {
			found = found || action.Kind == view.ActLocateDirectory && action.Refs[0] == id
		}
	}
	if !found {
		t.Fatal("readiness page omitted missing directory")
	}
	if _, err := api.Inspect([]string{"folder", "check"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Inspect([]string{"folder", "remove", "src:workshop"}); err == nil {
		t.Fatal("folder details permitted a write command")
	}
	if _, err := api.ApplySetting([]string{"folder", "set", id, "{}"}, moved); err != nil {
		t.Fatal(err)
	}
	dirs, err = api.Directories()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if d.ID == id && (d.Path != moved || d.Status != "ok") {
			t.Fatalf("relocation: %+v", d)
		}
	}
}
