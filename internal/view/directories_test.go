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
	shown := 0
	for _, d := range dirs {
		if d.Kind != "lock" || d.Status != "unused" { // an unused lock library is hidden (16 §2.14)
			shown++
		}
	}
	if len(g.Items) != shown {
		t.Fatalf("settings has %d directories, want %d", len(g.Items), shown)
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

// Settings › Folders folds (16 §2.14): every directory row is in one fold, under its root;
// a fine fold starts folded, a missing folder opens its fold, the player's choice wins, and a
// relocated folder goes under its own parent.
func TestDirectoryFolds(t *testing.T) {
	t.Parallel()
	e, _, _ := shellEnv(t)
	moved := filepath.Join(e.Root, "elsewhere", "source")
	if err := os.MkdirAll(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetMeta("cfg:source.dir", moved); err != nil {
		t.Fatal(err)
	}
	g := view.DirectoryGroup(e.App)
	seen := map[int]string{}
	folds := map[string]view.FolderFold{}
	for _, f := range g.Folds {
		folds[f.Key] = f
		if f.Count == "" || len(f.Rows) == 0 {
			t.Fatalf("fold without its count or rows: %+v", f)
		}
		for _, i := range f.Rows {
			if k, ok := seen[i]; ok {
				t.Fatalf("row %d in folds %s and %s", i, k, f.Key)
			}
			seen[i] = f.Key
			it := g.Items[i]
			if f.Root != "" && filepath.Join(f.Root, it.Rel) != filepath.Clean(it.Value) {
				t.Fatalf("row %q under %q shows %q", it.Value, f.Root, it.Rel)
			}
		}
	}
	if len(seen) != len(g.Items) {
		t.Fatalf("%d of %d rows are in folds", len(seen), len(g.Items))
	}
	local, game := folds["local"], folds["game"]
	if local.Root != e.App.LocalDir() || len(local.Rows) < 3 || local.Open {
		t.Fatalf("local fold: %+v", local)
	}
	if len(game.Rows) != 2 {
		t.Fatalf("game fold has the game and Workshop folders: %+v", game)
	}
	var other *view.FolderFold
	for _, f := range g.Folds {
		if f.Root == filepath.Dir(moved) {
			other = &f
		}
	}
	if other == nil || len(other.Rows) != 1 || g.Items[other.Rows[0]].Rel != "source" {
		t.Fatalf("the relocated source library is not under its own parent: %+v", g.Folds)
	}
	// the Workshop folder goes missing: its fold opens; then the player folds it
	if err := os.Rename(e.Workshop, e.Workshop+"-gone"); err != nil {
		t.Fatal(err)
	}
	for _, f := range view.DirectoryGroup(e.App).Folds {
		if f.Key == "game" && (!f.Attention || !f.Open) {
			t.Fatalf("a missing folder should open its fold: %+v", f)
		}
	}
	if err := e.App.Store.SetMeta(view.FoldsKey, `{"game":false,"local":true}`); err != nil {
		t.Fatal(err)
	}
	for _, f := range view.DirectoryGroup(e.App).Folds {
		if f.Key == "game" && f.Open || f.Key == "local" && !f.Open {
			t.Fatalf("the player's fold choice was not kept: %+v", f)
		}
	}
}
