package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestStartupDirectoryPromptCanSkipAndRelocate(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	moved := filepath.Join(e.Root, "relocated-workshop")
	if err := os.Rename(e.Workshop, moved); err != nil {
		t.Fatal(err)
	}
	m := New(Options{App: e.App}).(*model)
	m.Init()
	if m.panel == nil || len(m.panel.choices) < 2 {
		t.Fatal("startup did not offer missing directories and skip")
	}
	m.panelKey("esc")
	if m.panel != nil {
		t.Fatal("the directory prompt could not be skipped")
	}
	m.promptDirectories()
	m.panelKey("enter")
	if m.prompt == nil {
		t.Fatal("choosing a directory did not open the new-path input")
	}
	submit := m.prompt.submit
	m.prompt = nil
	submit(m, moved)
	dirs, err := e.App.ManagedDirectories()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if d.Kind == "workshop" && (d.Path != moved || d.Status != "ok") {
			t.Fatalf("directory still missing: %+v", d)
		}
	}
}
