package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// A colony chosen before its saves are read in full is still found once the full read
// groups them by their world, and the choice moves to the world's key; a world colony
// saved again under a new file name is still the choice before that save is read.
func TestSaveChoiceColonyKeyChanges(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	now := e.Now()
	w := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}, World: "Ennawrin", Seed: "hollow", PRV: 3, Faction: "First"}
	w.ModTime = now.Add(-6 * time.Hour)
	e.WriteSave("Rim 1.rws", w)
	w.ModTime, w.Ticks = now.Add(-5*time.Hour), 120000
	e.WriteSave("Rim 2.rws", w)
	envs, err := e.App.Store.Envs()
	if err != nil {
		t.Fatal(err)
	}
	id := envs[0].ID
	find := func(name string) *app.Save {
		t.Helper()
		s, _, err := e.App.FindSave(name)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := find("Rim 2")
	if s.Colony == nil || s.Colony.By != "name" {
		t.Fatalf("before the full read: %+v", s.Colony)
	}
	before := s.Colony.Key
	if _, err := e.App.ChooseSave(id, s, true); err != nil {
		t.Fatal(err)
	}
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.IndexSaves(l.IndexOrder(nil), nil); err != nil {
		t.Fatal(err)
	}
	s = find("Rim 2")
	if s.Colony.By != "world" || s.Colony.Key == before {
		t.Fatalf("after the full read: %+v", s.Colony)
	}
	ch := e.App.SaveChoiceOf(id)
	if !ch.Chose(s, true) {
		t.Fatal("the Saves page does not show the world colony as chosen")
	}
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, ch, err := e.App.ChosenSave(ps.State)
	if err != nil || c == nil || c.Save.Name != "Rim 2" {
		t.Fatalf("chosen colony after the full read: %+v %+v %v", c, ch, err)
	}
	if got := e.App.SaveChoiceOf(id); got.Colony != s.Colony.Key {
		t.Fatalf("the choice kept the file-name key: %+v", got)
	}
	if v := view.BuildNeeds(e.App, ps.State, ps).LastSave; v == nil || v.Gone {
		t.Fatalf("Can it run: %+v", v)
	}

	// Saved again over both files, not read yet: they group by file name, and the
	// colony's newest save is still the one checked.
	w.ModTime, w.Ticks = now.Add(-2*time.Hour), 130000
	e.WriteSave("Rim 1.rws", w)
	w.ModTime, w.Ticks = now.Add(-1*time.Hour), 140000
	e.WriteSave("Rim 2.rws", w)
	if s := find("Rim 2"); s.Colony.By != "name" {
		t.Fatalf("saved again: %+v", s.Colony)
	}
	c, _, err = e.App.ChosenSave(ps.State)
	if err != nil || c == nil || c.Save.Name != "Rim 2" {
		t.Fatalf("chosen colony saved again: %+v %v", c, err)
	}
}

// Can it run's cache key follows the chosen colony's files, not only its folder: a save
// written over in place need not change the folder's modification time. A removed Game
// Config's choice goes with it.
func TestSaveChoiceStampAndRemoval(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	mods := []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}
	e.WriteSave("Ashfall#§#Landing.rws", testkit.SaveSpec{Mods: mods, ModTime: e.Now().Add(-3 * time.Hour)})
	e.MustRun("env", "add", "second", "--like", "main")
	second, err := e.App.Store.EnvByName("second")
	if err != nil || second == nil {
		t.Fatal(err)
	}
	if e.App.SaveChoiceStamp(second.ID) != "" {
		t.Fatal("a stamp with no choice")
	}
	s, _, err := e.App.FindSave("Ashfall/Landing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.ChooseSave(second.ID, s, true); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.Path)
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	before := e.App.SaveChoiceStamp(second.ID)
	e.WriteSave("Ashfall#§#Landing.rws", testkit.SaveSpec{Mods: mods, Ticks: 90000, ModTime: e.Now().Add(-1 * time.Hour)})
	if err := os.Chtimes(dir, di.ModTime(), di.ModTime()); err != nil {
		t.Fatal(err)
	}
	if after := e.App.SaveChoiceStamp(second.ID); after == before {
		t.Fatal("the stamp missed a save written over in place")
	}
	if _, err := e.App.EnvRemove("second"); err != nil {
		t.Fatal(err)
	}
	if v := e.App.Store.Meta("saves.check." + second.ID); v != "" {
		t.Fatalf("the removed config's choice stayed: %q", v)
	}
}
