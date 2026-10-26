package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestGitSourcePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, kind, label, directory string
		uploadedRecord               bool
	}{
		{"workshop", "workshop", "workshop", "workshop", false},
		{"workshop with uploaded record", "workshop", "workshop", "workshop", true},
		{"steamcmd", "workshop", "SteamCMD", "steamcmd", false},
		{"steamcmd with uploaded record", "workshop", "SteamCMD", "steamcmd", true},
		{"local", "git", "Git", "local", false},
		{"extra", "git", "Git", "other", false},
		{"subscription", "git", "Git", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			g := testkit.NewGitRepo(t, "")
			e.AddEnv(true)
			var dir string
			switch {
			case strings.HasPrefix(tc.name, "workshop"):
				dir = filepath.Join(e.Workshop, "5100")
				e.Subscribe("5100")
			case strings.HasPrefix(tc.name, "steamcmd"):
				dir = filepath.Join(e.SteamCMD, "5100")
			case tc.name == "local":
				dir = filepath.Join(e.Game, "Mods", "Git Mod")
			case tc.name == "extra":
				root := filepath.Join(e.Root, "Extra")
				e.MkdirAll(root)
				e.MustRun("folder", "add", "extra", root)
				dir = filepath.Join(root, "Git Mod")
			case tc.name == "subscription":
				op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
				if err != nil {
					t.Fatal(err)
				}
				dir = op.Items[0].Folder
			}
			if tc.name != "subscription" {
				e.CopyTree(g.Work, dir)
			}
			if tc.uploadedRecord {
				if err := subscribe.Write(dir, &subscribe.Record{Kind: "git", URL: g.Bare, RefType: "branch", Ref: "main", Installed: g.First}); err != nil {
					t.Fatal(err)
				}
			}
			// Steam uploads commonly contain untracked files beside a stale repository.
			e.WriteFile(filepath.Join(dir, "untracked.txt"), "uploaded content")
			e.MustRun("scan")
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			m, err := st.Ix.ResolveOne("pkg:tester.gitmod")
			if err != nil {
				t.Fatal(err)
			}
			if m.GitHead != g.First {
				t.Fatalf("HEAD fingerprint = %q, want %q", m.GitHead, g.First)
			}
			if got := view.SourceKind(m); got != tc.kind {
				t.Errorf("source kind = %q, want %q", got, tc.kind)
			}
			if got := view.CopyCaption(e.App, st, m); got != tc.label {
				t.Errorf("copy caption = %q, want %q", got, tc.label)
			}
			it := view.ModItem(st, m)
			for _, kind := range []string{"git", "workshop", "steamcmd"} {
				want := kind == tc.kind || kind == "steamcmd" && m.Kind == source.SteamCMD
				if got := view.ParseQuery("type:"+kind).Match(&it, view.MatchFacts{}); got != want {
					t.Errorf("type:%s = %v, want %v", kind, got, want)
				}
			}
			lib := mustLibrary(t, e)
			row := libRow(t, libPage(t, e, ""), "Git Mod")
			if row.Source != tc.label || row.Insts[0].Label != tc.label {
				t.Errorf("library source = %q, label = %q, want %q", row.Source, row.Insts[0].Label, tc.label)
			}
			for _, filter := range []string{"workshop", "steamcmd", "git", tc.directory} {
				want := filter == tc.directory || filter == "git" && tc.name == "subscription"
				r, err := lib.Filter(app.LibQuery{Include: []string{"source:" + filter}})
				if err != nil || (len(r.Matches) == 1) != want {
					t.Fatalf("source:%s: %+v, %v; want match %v", filter, r, err, want)
				}
			}
			if m.Kind.WorkshopLike() {
				op, err := e.App.CheckSubscriptions(nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range op.Items {
					if it.Record.Kind != "steamcmd" || !strings.Contains(it.Result, "skipped") {
						t.Errorf("workshop git check: %+v", it)
					}
				}
				if m.Kind == source.SteamCMD {
					op, err := e.App.UpdateSubscriptions([]string{st.Ix.Ref(m.ID)}, true)
					if err != nil || len(op.Items) != 1 || op.Items[0].Record.Kind != "steamcmd" || !strings.Contains(op.Items[0].Result, "would download Workshop 5100") {
						t.Fatalf("SteamCMD update: %+v, %v", op, err)
					}
				}
			}
		})
	}
}
