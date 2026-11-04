package view_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestDirectorySettingsAndWarning(t *testing.T) {
	t.Parallel()
	e, _, _ := shellEnv(t)
	dirs, err := e.App.ManagedDirectories()
	if err != nil {
		t.Fatal(err)
	}
	g := view.DirectoryGroup(e.App)
	if len(g.Items) != len(dirs) {
		t.Fatalf("settings has %d directories, want %d", len(g.Items), len(dirs))
	}
	for _, it := range g.Items {
		if it.Value == "" || it.Directory == "" || len(it.Argv) != 4 || len(it.MoveArgv) != 4 {
			t.Fatalf("directory lacks its actual path or relocation controls: %+v", it)
		}
		if err := view.SettingCommand(it.Argv); err != nil {
			t.Fatal(err)
		}
		if err := view.SettingCommand(it.MoveArgv); err != nil {
			t.Fatal(err)
		}
	}
	moved := filepath.Join(e.Root, "moved-workshop")
	if err := os.Rename(e.Workshop, moved); err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, st, ps)
	var id string
	for _, it := range n.Items {
		for _, act := range it.Actions {
			if act.Kind != view.ActLocateDirectory {
				continue
			}
			if it.Section != view.SectionConfirm || it.Status != view.StatusWarn {
				t.Fatalf("directory warning should be a non-blocking confirmation: %+v", it)
			}
			if act.Lines[0] == e.Workshop {
				id = act.Refs[0]
			}
		}
	}
	if id == "" {
		t.Fatal("a folder moved outside RimArcho has no relocation warning")
	}
	if err := e.App.SetDirectory(id, moved); err != nil {
		t.Fatal(err)
	}
	n = view.BuildNeeds(e.App, st, ps)
	for _, it := range n.Items {
		for _, act := range it.Actions {
			if act.Kind == view.ActLocateDirectory && act.Refs[0] == id {
				t.Fatal("warning remains after relocation")
			}
		}
	}
}
