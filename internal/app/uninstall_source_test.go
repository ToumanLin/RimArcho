package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestUninstallRelocatedSourceLibrary(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	root := filepath.Join(f.Root, "Moved source")
	f.WriteFile(filepath.Join(f.App.SourceDir(), "game", "1.6", "Assembly-CSharp", "Pawn.cs"), "generated source")
	if _, err := f.App.MoveDirectory("source", root); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(f.Root, "keep.txt")
	f.WriteFile(outside, "player data")
	p, err := f.App.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := p.Step(app.UnSource)
	if s == nil || len(s.Items) != 1 || s.Items[0].Path != root || s.Items[0].Do != app.DoDelete || s.Bytes() != int64(len("generated source")) {
		t.Fatalf("source plan: %+v", s)
	}
	pg := view.UninstallView(p, false)
	found := false
	for _, row := range pg.Confirmation.Changes {
		if row.Title == "source library" {
			found = true
			if row.Count != 1 || row.Size == "" || row.Size == "0 B" || row.Method != "delete directly" {
				t.Fatalf("confirmation: %+v", row)
			}
		}
	}
	if !found || !exists(root) {
		t.Fatal("missing confirmation or preview changed the library")
	}
	if out := f.MustRun("uninstall"); !strings.Contains(out, "delete the generated source library") || !strings.Contains(out, root) {
		t.Fatalf("CLI source summary: %s", out)
	}
	if bad, err := f.App.PlanUninstall(app.UninstallOptions{PacksTo: filepath.Join(root, "kept packs")}); err != nil || bad.Blocked() == nil {
		t.Fatalf("retained packs inside deleted source library: %+v %v", bad, err)
	}
	f.run(t, app.UninstallOptions{})
	if exists(root) || readFile(t, outside) != "player data" {
		t.Fatal("uninstall failed to remove only the recorded library")
	}
}

func TestUninstallSourceLibraryCoveredByData(t *testing.T) {
	t.Parallel()
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "home", true: "local"}[local], func(t *testing.T) {
			f := uninstallFixture(t)
			root := f.App.SourceDir()
			if local {
				root = filepath.Join(f.local, "source")
				f.MkdirAll(root)
				if err := f.App.SetDirectory("source", root); err != nil {
					t.Fatal(err)
				}
			}
			f.WriteFile(filepath.Join(root, "Generated.cs"), "generated source")
			p, err := f.App.PlanUninstall(app.UninstallOptions{})
			if err != nil || p.Step(app.UnSource) != nil {
				t.Fatalf("library listed twice: %+v %v", p, err)
			}
			covered := false
			for _, path := range p.AfterExit.Dirs {
				covered = covered || source.Within(root, path)
			}
			if !covered {
				t.Fatal("data deletion does not cover the library")
			}
		})
	}
}

func TestUninstallRefusesSourceLibraryLinks(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"root", "ancestor", "child"} {
		t.Run(kind, func(t *testing.T) {
			f := uninstallFixture(t)
			root := filepath.Join(f.Root, "Moved source")
			f.WriteFile(filepath.Join(root, "Generated.cs"), "generated source")
			if err := f.App.SetDirectory("source", root); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(f.Root, "outside")
			f.WriteFile(filepath.Join(outside, "Keep.cs"), "player data")
			link := root
			switch kind {
			case "ancestor":
				root = filepath.Join(root, "nested")
				f.MkdirAll(root)
				if err := f.App.SetDirectory("source", root); err != nil {
					t.Fatal(err)
				}
				f.MkdirAll(filepath.Join(outside, "nested"))
			case "child":
				link = filepath.Join(root, "linked")
			}
			if kind != "child" {
				if err := os.RemoveAll(link); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.CreateLink(link, outside); err != nil {
				t.Fatal(err)
			}
			defer source.RemoveLink(link)
			if p, err := f.App.PlanUninstall(app.UninstallOptions{}); err != nil || p.Blocked() == nil || !strings.Contains(p.Blocked().Error(), "link") {
				t.Fatalf("linked library not blocked on the page: %+v %v", p, err)
			}
			if _, err := f.App.Uninstall(app.UninstallOptions{}, nil); err == nil {
				t.Fatal("linked library uninstalled")
			}
			if readFile(t, filepath.Join(outside, "Keep.cs")) != "player data" || f.after != nil || len(f.trashed) != 0 {
				t.Fatal("refused uninstall changed files")
			}
		})
	}
}

func TestUninstallRechecksSourceLibraryBeforeDeletion(t *testing.T) {
	t.Parallel()
	for _, changed := range []string{"link", "record"} {
		t.Run(changed, func(t *testing.T) {
			f := uninstallFixture(t)
			root := filepath.Join(f.Root, "Moved source")
			f.WriteFile(filepath.Join(root, "Generated.cs"), "generated source")
			if err := f.App.SetDirectory("source", root); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(f.Root, "outside")
			f.WriteFile(filepath.Join(outside, "Keep.cs"), "player data")
			p, err := f.App.Uninstall(app.UninstallOptions{}, func(p app.UninstallProgress) {
				if p.Step != app.UnSource {
					return
				}
				if changed == "record" {
					if err := f.App.SetDirectory("source", outside); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.RemoveAll(root); err != nil {
						t.Fatal(err)
					}
					if err := app.CreateLink(root, outside); err != nil {
						t.Fatal(err)
					}
					deferCleanup := root
					t.Cleanup(func() { source.RemoveLink(deferCleanup) })
				}
			})
			if err != nil || p.Step(app.UnSource).Items[0].Err == "" || f.after != nil {
				t.Fatalf("changed root was not refused: %+v %v", p, err)
			}
			if readFile(t, filepath.Join(outside, "Keep.cs")) != "player data" {
				t.Fatal("outside target changed")
			}
		})
	}
}
