package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A fix asked for by name that is done already is done, not an error; missing entries no
// download installs (no Workshop ID, or the Workshop item installs another packageId) are
// final, so fix --launch does not wait for them forever (12 §4.4).
func TestFixSaveAlreadyAndStuck(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	find := func(ref string) *app.Save {
		t.Helper()
		s, _, err := e.App.FindSave(ref)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if r, err := e.App.FixSave(find("Tundra 2"), app.SaveFixOptions{Mods: []string{"tester.gamma"}}, false); err != nil || len(r.Enable) != 1 {
		t.Fatalf("first fix: %+v %v", r, err)
	}
	r, err := e.App.FixSave(find("Tundra 2"), app.SaveFixOptions{Mods: []string{"tester.gamma"}}, false)
	if err != nil || r.Changed() || r.Edit != nil || len(r.Already) != 1 || r.Already[0].PackageID != "tester.gamma" {
		t.Fatalf("the same fix again: %+v %v", r, err)
	}
	if out := e.MustRun("saves", "fix", "Tundra 2", "--mod", "tester.gamma", "--launch", "--dry-run"); !strings.Contains(out, "already done, plan:default loads Gamma") {
		t.Fatalf("fix --mod again:\n%s", out)
	}
	if _, err := e.App.FixSave(find("Tundra 2"), app.SaveFixOptions{Mods: []string{"tester.none"}}, true); err == nil {
		t.Fatal("fixed a Mod the save does not have")
	}

	// A save whose list has two missing entries that no download installs.
	e.AddWorkshop("3000000005", testkit.Sample("tester.newid", "Renamed"))
	e.MustRun("scan")
	p := e.WriteSave("Tundra 3.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta", "tester.lost", "tester.oldid"},
		Names: []string{"Core", "Alpha", "Beta", "Lost", "Old ID"}, SteamIDs: []string{"", "3000000002", "3000000003", "", "3000000005"},
		ModTime: e.Now().Add(-5 * time.Minute)})
	e.MustRun("loadout", "import", p)
	r, err = e.App.FixSave(find("Tundra 3"), app.SaveFixOptions{}, true)
	if err != nil || len(r.Stuck) != 2 || len(r.Waiting) != 0 || r.Stuck[0].Why != app.StuckNoWorkshop ||
		!strings.Contains(r.Stuck[1].Why, "installed as tester.newid") {
		t.Fatalf("stuck: %+v %v", r, err)
	}
	out := e.MustRun("saves", "fix", "Tundra 3", "--launch", "--dry-run")
	if !strings.Contains(out, "launch: would save plan:default to the game and start it") || !strings.Contains(out, "without: 2 Mods of the save that cannot be downloaded") ||
		strings.Contains(out, "not installed yet") {
		t.Fatalf("fix --launch with stuck entries:\n%s", out)
	}
}

// Back up and recycle confirm only what was previewed: the token binds the saves' size and
// time and the backup's folder; two backups do not share a folder; two saves of one name
// from two folders both get copied (12 §4.4).
func TestSaveFilesToken(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	find := func(ref string) *app.Save {
		t.Helper()
		s, _, err := e.App.FindSave(ref)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	saves := []*app.Save{find("Tundra"), find("Broken")}
	prev, err := e.App.BackupSaves(saves, false, "")
	if err != nil || prev.Token == "" || !strings.HasPrefix(prev.Token, filepath.Base(prev.Dir)+".") {
		t.Fatalf("preview %+v %v", prev, err)
	}
	e.Advance(3 * time.Second) // the confirmation comes later: it still uses the preview's folder
	done, err := e.App.BackupSaves(saves, true, prev.Token)
	if err != nil || done.Dir != prev.Dir || len(done.Errs) != 0 {
		t.Fatalf("confirm %+v %v, preview folder %s", done, err, prev.Dir)
	}
	again, err := e.App.BackupSaves(saves, true, prev.Token)
	if err != nil || again.Dir != prev.Dir+"-2" || len(again.Errs) != 0 {
		t.Fatalf("a second backup in the same second: %+v %v", again, err)
	}
	if _, err := e.App.BackupSaves(saves, true, "x/../../evil.0123456789"); !errors.Is(err, app.ErrPreviewChanged) {
		t.Fatalf("a forged token: %v", err)
	}

	// Two saves named Tundra.rws from two folders.
	other := *find("Tundra")
	dir := t.TempDir()
	other.Path, other.Ref = filepath.Join(dir, other.File), "sv:000001"
	other.Folder = &app.SaveFolder{Dir: dir, Pack: "Ice: Age"}
	if err := os.WriteFile(other.Path, []byte("<savegame/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	both, err := e.App.BackupSaves([]*app.Save{find("Tundra"), &other}, true, "")
	if err != nil || len(both.Errs) != 0 {
		t.Fatalf("same names: %+v %v", both, err)
	}
	for _, f := range []string{"Tundra.rws", filepath.Join("Ice_ Age", "Tundra.rws")} {
		if _, err := os.Stat(filepath.Join(both.Dir, f)); err != nil {
			t.Fatalf("same names: %v", err)
		}
	}

	// Recycling: a save written again since its preview is not recycled.
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, filepath.Base(p)); return os.Remove(p) }
	tp, err := e.App.TrashSaves([]*app.Save{find("Tundra 2")}, false, "")
	if err != nil || tp.Token == "" {
		t.Fatalf("trash preview %+v %v", tp, err)
	}
	e.WriteSave("Tundra 2.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-10 * time.Second)})
	if _, err := e.App.TrashSaves([]*app.Save{find("Tundra 2")}, true, tp.Token); !errors.Is(err, app.ErrPreviewChanged) || len(trashed) != 0 {
		t.Fatalf("recycled a save changed since its preview: %v %v", err, trashed)
	}
	out := e.MustRun("saves", "trash", "Tundra 2")
	i := strings.Index(out, "--token ")
	if i < 0 {
		t.Fatalf("the preview names no token:\n%s", out)
	}
	tok := strings.Fields(out[i+len("--token "):])[0]
	if out := e.MustRun("saves", "trash", "Tundra 2", "--yes", "--token", tok); !strings.HasPrefix(out, "moved to the recycle bin: 1 save") || len(trashed) != 1 {
		t.Fatalf("trash with the token:\n%s", out)
	}

	// The game running refuses the preview too.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 9} }
	if _, err := e.App.TrashSaves([]*app.Save{find("Broken")}, false, ""); !errors.Is(err, app.ErrGameRunning) {
		t.Fatalf("recycle preview while the game runs: %v", err)
	}
	e.App.GameProbe = nil

	// A rename that changes only the letter case.
	if to, err := e.App.RenameSave(find("Tundra"), "TUNDRA"); err != nil || filepath.Base(to) != "TUNDRA.rws" {
		t.Fatalf("case-only rename: %s %v", to, err)
	}
	ents, _ := os.ReadDir(e.SavesDir())
	found := false
	for _, de := range ents {
		found = found || de.Name() == "TUNDRA.rws"
	}
	if !found {
		t.Fatal("the case-only rename left the old name")
	}
}

// Colony names stay with their colony (12 §4.1): a world colony takes the name set while
// its saves were grouped by file name, another world saved under that name does not, an
// empty name clears every key the colony answers to, and saves index moves the name to the
// world's key and removes names whose saves are gone.
func TestColonyNamesFollowTheColony(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	now := e.Now()
	w1 := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, World: "Ennawrin", Seed: "hollow", PRV: 3, Faction: "First"}
	w1.ModTime = now.Add(-6 * time.Hour)
	e.WriteSave("Rim 1.rws", w1)
	w1.ModTime, w1.Ticks = now.Add(-5*time.Hour), 120000
	e.WriteSave("Rim 2.rws", w1)
	list := func() *app.SaveList {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	colonyOf := func(l *app.SaveList, save string) *app.Colony {
		t.Helper()
		s, err := l.Find(save)
		if err != nil {
			t.Fatal(err)
		}
		return s.Colony
	}
	read := func() {
		t.Helper()
		l := list()
		if _, err := e.App.IndexSaves(l.IndexOrder(nil), nil); err != nil {
			t.Fatal(err)
		}
	}
	c := colonyOf(list(), "Rim 1")
	if c.By != "name" {
		t.Fatalf("before the full read: %+v", c)
	}
	if err := e.App.RenameColony(c, "North"); err != nil {
		t.Fatal(err)
	}
	read()
	w2 := testkit.SaveSpec{Mods: w1.Mods, World: "Other", Seed: "fresh", PRV: 9, Faction: "Second", ModTime: now.Add(-4 * time.Hour)}
	e.WriteSave("Rim 3.rws", w2)
	read()
	l := list()
	if c := colonyOf(l, "Rim 1"); c.By != "world" || c.Name != "North" || c.GameName != "First" {
		t.Fatalf("the world colony: %+v", c)
	}
	if c := colonyOf(l, "Rim 3"); c.Name != "Second" || c.GameName != "" {
		t.Fatalf("another world saved as Rim took the name: %+v", c)
	}
	if out := e.MustRun("saves", "list", "--colony", "First"); !strings.Contains(out, "North  2 saves") {
		t.Fatalf("--colony by the saves' name:\n%s", out)
	}
	// clearing clears the file-name key too: the old name does not come back
	if err := e.App.RenameColony(colonyOf(l, "Rim 1"), ""); err != nil {
		t.Fatal(err)
	}
	if c := colonyOf(list(), "Rim 1"); c.Name != "First" {
		t.Fatalf("after clearing: %+v", c)
	}
	if names, _ := e.App.Store.ColonyNames(); len(names) != 0 {
		t.Fatalf("names left: %v", names)
	}

	// Named by file name again, then saves index settles it on the world's key.
	if err := e.App.Store.SetColonyName(env0Key(t, e)+"|name:rim", "North"); err != nil {
		t.Fatal(err)
	}
	e.MustRun("saves", "index")
	names, _ := e.App.Store.ColonyNames()
	if len(names) != 1 {
		t.Fatalf("after saves index: %v", names)
	}
	for k, n := range names {
		if !strings.Contains(k, "|world:hollow|") || n != "North" {
			t.Fatalf("after saves index: %v", names)
		}
	}
	if c := colonyOf(list(), "Rim 1"); c.Name != "North" {
		t.Fatalf("after settling: %+v", c)
	}
	// The colony saved again: until the full read its saves group by file name, so no listed
	// colony answers to the world key, yet the name stays (a listing taken before the read,
	// or a parser bump that unreads every save, must not drop it).
	w1.ModTime, w1.Ticks = now.Add(-3*time.Hour), 130000
	e.WriteSave("Rim 1.rws", w1)
	w1.ModTime, w1.Ticks = now.Add(-2*time.Hour), 140000
	e.WriteSave("Rim 2.rws", w1)
	if _, err := e.App.SettleColonyNames(list()); err != nil {
		t.Fatal(err)
	}
	if names, _ := e.App.Store.ColonyNames(); len(names) != 1 {
		t.Fatalf("settled against saves not read yet: %v", names)
	}
	e.MustRun("saves", "index")
	if c := colonyOf(list(), "Rim 1"); c.Name != "North" || c.By != "world" {
		t.Fatalf("after saving again and saves index: %+v", c)
	}
	// its saves gone, the name goes too
	for _, f := range []string{"Rim 1.rws", "Rim 2.rws"} {
		if err := os.Remove(filepath.Join(e.SavesDir(), f)); err != nil {
			t.Fatal(err)
		}
	}
	e.MustRun("saves", "index")
	if names, _ := e.App.Store.ColonyNames(); len(names) != 0 {
		t.Fatalf("a name of saves that are gone: %v", names)
	}
}

// env0Key is the colony key prefix of the fixture's Saves folder.
func env0Key(t *testing.T, e *testkit.Env) string {
	t.Helper()
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Find("Tundra")
	if err != nil {
		t.Fatal(err)
	}
	k := s.Colony.Key
	return k[:strings.Index(k, "|")]
}

// saves show --current compares with the colony's previous save as without it (12 §4.3).
func TestSavesShowCurrentKeepsPrevious(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	w := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, World: "Ennawrin", Seed: "hollow", PRV: 3, Faction: "First",
		ModTime: e.Now().Add(-6 * time.Hour)}
	e.WriteSave("Rim 1.rws", w)
	w.ModTime, w.Ticks = e.Now().Add(-5*time.Hour), 120000
	e.WriteSave("Rim 2.rws", w)
	e.MustRun("saves", "index")
	plain := e.MustRun("saves", "show", "Rim 2")
	cur := e.MustRun("saves", "show", "Rim 2", "--current")
	if !strings.Contains(plain, "since Rim 1") || !strings.Contains(cur, "since Rim 1") || strings.Contains(cur, "latest Rim 2") {
		t.Fatalf("show:\n%s\nshow --current:\n%s", plain, cur)
	}
}
