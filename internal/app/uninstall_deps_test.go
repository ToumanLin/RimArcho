package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestUninstallNamesOwnedToolsAndLeavesConfiguredTools(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "own", true: "configured"}[configured], func(t *testing.T) {
			e := testkit.New(t)
			e.App.TextureEncoder = &texEnc{}
			t.Setenv("PATH", "")
			for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
				t.Setenv(key, "")
			}
			paths := map[string]string{deps.Todds: filepath.Join(e.App.LocalDir(), deps.Todds, "todds.exe"), deps.WezTerm: filepath.Join(e.App.Home, deps.WezTerm, "wezterm-gui.exe")}
			for name, path := range paths {
				if configured {
					path = filepath.Join(e.Root, "external", name, filepath.Base(path))
					paths[name] = path
				}
				e.WriteFile(path, "tool")
				if configured {
					key := "textures.todds"
					if name == deps.WezTerm {
						key = "wezterm.path"
					}
					e.MustRun("config", "set", key, path)
				}
			}
			p, err := e.App.PlanUninstall(app.UninstallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for name, path := range paths {
				label := name
				if name == deps.WezTerm {
					label = "WezTerm"
				}
				found := false
				for _, it := range p.Step(app.UnData).Items {
					found = found || strings.Contains(it.Note, "includes the downloaded "+label)
				}
				if found == configured {
					t.Fatalf("tool annotation %s: found=%v configured=%v", name, found, configured)
				}
				if configured {
					for _, step := range p.Steps {
						for _, it := range step.Items {
							if it.Do != app.DoLeave && it.Do != app.DoKeep && source.Within(path, it.Path) {
								t.Fatalf("external %s included in removal %+v", path, it)
							}
						}
					}
					for _, dir := range p.AfterExit.Dirs {
						if source.Within(path, dir) {
							t.Fatalf("external path deleted via %s", dir)
						}
					}
					for _, file := range p.AfterExit.Files {
						if file == path {
							t.Fatalf("external file deleted: %s", file)
						}
					}
				}
			}
			if !configured {
				view.SetLang("zh")
				t.Cleanup(func() { view.SetLang("en") })
				pg := view.UninstallView(p, false)
				for _, step := range pg.Steps {
					if step.Key == app.UnData {
						var notes []string
						for _, it := range step.Items {
							notes = append(notes, it.Note)
						}
						note := strings.Join(notes, " ")
						if !strings.Contains(note, "包含下载的 todds") || !strings.Contains(note, "包含下载的 WezTerm") {
							t.Fatalf("untranslated notes %s", note)
						}
					}
				}
			}
		})
	}
}
