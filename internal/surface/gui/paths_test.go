package gui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSettingFilesystemPathMarkers(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	groups := append(api.Settings(), api.Environment().Groups...)
	var values, notes int
	for _, g := range groups {
		for _, item := range g.Items {
			if filepath.IsAbs(item.Value) {
				values++
				if !item.ValuePath {
					t.Errorf("%s / %s: value path has no marker: %q", g.Title, item.Label, item.Value)
				}
			}
			if filepath.IsAbs(item.Note) {
				notes++
				marked := false
				for _, path := range item.NotePaths {
					marked = marked || path != "" && strings.HasPrefix(item.Note, path)
				}
				if !marked {
					t.Errorf("%s / %s: note path has no marker: %q", g.Title, item.Label, item.Note)
				}
			}
			for _, path := range item.NotePaths {
				if path != "" && !strings.Contains(item.Note, path) {
					t.Errorf("unused note path %q in %+v", path, item)
				}
			}
		}
	}
	if values == 0 || notes == 0 {
		t.Fatalf("fixture did not exercise both path locations: %d values, %d notes", values, notes)
	}
	env := api.Environment()
	if len(env.Envs) == 0 || env.Envs[0].Path != e.Game {
		t.Fatalf("game row lost its complete path: %+v", env.Envs)
	}
	for _, item := range view.DependencyGroup(e.App).Items {
		if len(item.NotePaths) != 1 {
			t.Fatalf("dependency mixes its path into unmarked facts: %+v", item)
		}
	}
}

func TestTechnicalFileCopyTargets(t *testing.T) {
	// Not parallel. The Defs section lists files once the background analysis has finished,
	// which lags on a slow runner: wait for it.
	e, api, _ := fixture(t)
	mod := testkit.Sample("tester.paths", "Paths")
	mod.Files = map[string]string{"Defs/path test.xml": "<Defs><ThingDef><defName>PathTest</defName></ThingDef></Defs>"}
	e.AddWorkshop("1002", mod)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range st.Ix.Mods {
		if m.PackageID != "tester.paths" {
			continue
		}
		want, err := filepath.EvalSymlinks(filepath.Join(e.Workshop, "1002", "Defs", "path test.xml"))
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(200 * time.Millisecond) {
			sections, err := api.Sections(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			computing := false
			for _, sec := range sections {
				for _, path := range sec.Paths {
					if path.Text == "Defs/path test.xml" && path.Path == want {
						return
					}
				}
				for _, line := range sec.Lines {
					computing = computing || strings.Contains(line, "still computing")
				}
			}
			if !computing || time.Now().After(deadline) {
				t.Fatalf("technical file lost its absolute copy target: %+v", sections)
			}
		}
	}
	t.Fatal("path test mod not scanned")
}
