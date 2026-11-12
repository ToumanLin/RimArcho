package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Saves across Game Configs (12 §4.8): every config's saves are listed whichever is
// current; Can it run checks the chosen save or colony per config; Copy to the current
// config puts only the file in its Saves folder.
func TestSaveChoiceAcrossGameConfigs(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	mods := []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}
	e.WriteSave("Ashfall#§#Landing.rws", testkit.SaveSpec{Mods: mods, ModTime: e.Now().Add(-3 * 3600e9)})
	e.WriteSave("Ashfall#§#Later.rws", testkit.SaveSpec{Mods: mods, ModTime: e.Now().Add(-2 * 3600e9)})
	e.MustRun("env", "add", "second", "--like", "main")

	envs, err := e.App.Store.Envs()
	if err != nil {
		t.Fatal(err)
	}
	var main, second = envs[0], envs[1]
	if main.Name != "main" {
		main, second = second, main
	}
	// The listing is the same whichever Game Config is current.
	count := func() int {
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		return len(l.Saves())
	}
	e.MustRun("env", "use", "second")
	if n := count(); n != 2 {
		t.Fatalf("saves listed with second current: %d", n)
	}
	s, _, err := e.App.FindSave("Ashfall/Later")
	if err != nil {
		t.Fatal(err)
	}
	if !s.SeenBy(main) || s.SeenBy(second) {
		t.Fatalf("seen by main %v, second %v", s.SeenBy(main), s.SeenBy(second))
	}
	if out := e.MustRun("saves", "show", s.Ref); !strings.Contains(out, "saves of: env:main") || !strings.Contains(out, "not listed: the game started with env:second") {
		t.Fatalf("show from second:\n%s", out)
	}

	// Choosing for second: Can it run checks that save, says second's game does not list
	// it, and main keeps no choice.
	if _, err := e.App.ChooseSave(second.ID, s, false); err != nil {
		t.Fatal(err)
	}
	if e.App.SaveChoiceOf(main.ID) != nil {
		t.Fatal("main got second's choice")
	}
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := view.BuildNeeds(e.App, ps.State, ps).LastSave
	if v == nil || v.Chosen != app.ChooseSaveFile || v.Name != "Later" || v.Unseen == "" || v.Gone {
		t.Fatalf("chosen save: %+v", v)
	}

	// A colony: its newest save, whichever it is now.
	if _, err := e.App.ChooseSave(second.ID, s, true); err != nil {
		t.Fatal(err)
	}
	e.WriteSave("Ashfall#§#Newest.rws", testkit.SaveSpec{Mods: mods, ModTime: e.Now().Add(-1 * 3600e9)})
	c, ch, err := e.App.ChosenSave(ps.State)
	if err != nil || ch == nil || ch.Kind != app.ChooseColony || c == nil || c.Save.Name != "Newest" {
		t.Fatalf("chosen colony: %+v %+v %v", c, ch, err)
	}
	if c.Save.Plan == nil || c.Save.Plan.ID != ps.State.Plan.ID {
		t.Fatal("not checked against the current list")
	}

	// Copy into second's Saves folder: the game started with second lists the copy.
	to, err := e.App.CopySaveTo(s, second)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(to) != e.App.SavesDir(second) || filepath.Base(to) != "Ashfall#§#Later.rws" {
		t.Fatalf("copied to %s", to)
	}
	if _, err := e.App.CopySaveTo(s, main); err != app.ErrSameFolder {
		t.Fatalf("copy into its own folder: %v", err)
	}
	again, err := e.App.CopySaveTo(s, second)
	if err != nil || filepath.Base(again) != "Ashfall#§#Later (copy).rws" {
		t.Fatalf("second copy: %s %v", again, err)
	}
	if n := count(); n != 5 {
		t.Fatalf("after copies: %d saves", n)
	}

	// Gone: the chosen file is removed.
	if _, err := e.App.ChooseSave(second.ID, s, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.Path); err != nil {
		t.Fatal(err)
	}
	if v := view.BuildNeeds(e.App, ps.State, ps).LastSave; v == nil || !v.Gone || v.Chosen == "" {
		t.Fatalf("gone: %+v", v)
	}
	// The CLI shows and clears the choice.
	if out := e.MustRun("saves", "choose"); !strings.Contains(out, "Can it run checks save Later") {
		t.Fatalf("choose shows: %s", out)
	}
	e.MustRun("saves", "choose", "--clear")
	if e.App.SaveChoiceOf(second.ID) != nil {
		t.Fatal("not cleared")
	}
}
