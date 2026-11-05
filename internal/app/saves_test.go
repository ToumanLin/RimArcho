package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// savesFixture: Core, Alpha and Beta enabled, Gamma installed but not enabled; a launch of
// the list an hour ago. Saves: one with the same Mods, one after the launch with another
// order plus Gamma and a Workshop Mod that is not installed, its autosave, a RimSaves save
// and one cut off by a crash.
func savesFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	now := e.Now()
	e.RecordLaunch(now.Add(-time.Hour))
	same := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, ModTime: now.Add(-2 * time.Hour)}
	e.WriteSave("Tundra.rws", same)
	later := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "Tester.Beta", "tester.alpha", "tester.gamma", "tester.gone"},
		Names: []string{"Core", "Beta", "Alpha", "Gamma", "Gone Mod"}, SteamIDs: []string{"", "3000000003", "3000000002", "", "99"},
		ModTime: now.Add(-30 * time.Minute)}
	e.WriteSave("Tundra 2.rws", later)
	later.ModTime = now.Add(-20 * time.Minute)
	e.WriteSave("Autosave-1.rws", later)
	same.ModTime = now.Add(-3 * time.Hour)
	e.WriteSave("Ashfall#§#Landing.rws", same)
	e.WriteSave("Broken.rws", testkit.SaveSpec{Mods: same.Mods, CutOff: true, ModTime: now.Add(-10 * time.Minute)})
	return e
}

func TestSavesListGroupAndMatch(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for _, c := range l.Colonies {
		var names []string
		for _, s := range c.Saves {
			names = append(names, s.Name)
		}
		cols = append(cols, c.Name+": "+strings.Join(names, ", "))
	}
	want := []string{"Broken: Broken", "Tundra: Autosave-1, Tundra 2, Tundra", "Ashfall: Landing"}
	if !slices.Equal(cols, want) {
		t.Fatalf("colonies:\n%s\nwant:\n%s", strings.Join(cols, "\n"), strings.Join(want, "\n"))
	}
	refRe := regexp.MustCompile(`^sv:[0-9a-f]{6}$`)
	match := map[string]string{}
	for _, s := range l.Saves() {
		if !refRe.MatchString(s.Ref) {
			t.Fatalf("%s: ref %q", s.Name, s.Ref)
		}
		match[s.Name] = s.Match + " " + s.ListName()
	}
	if match["Tundra 2"] != app.MatchLaunch+" default" || match["Tundra"] != app.MatchMods+" default" || match["Landing"] != app.MatchMods+" default" {
		t.Fatalf("matches: %v", match)
	}
	if !l.Colonies[1].Saves[0].Auto || l.Colonies[0].Saves[0].Head.Complete {
		t.Fatal("autosave or cut-off flags")
	}
	for _, ref := range []string{"Tundra 2", "Tundra 2.rws", "Ashfall/Landing", l.Colonies[2].Saves[0].Ref} {
		if _, err := l.Find(ref); err != nil {
			t.Fatalf("find %q: %v", ref, err)
		}
	}
	if _, err := l.Find("Nope"); err == nil {
		t.Fatal("found a save that does not exist")
	}
}

func TestCheckSave(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	check := func(name string) *app.SaveCheck {
		t.Helper()
		s, _, err := e.App.FindSave(name)
		if err != nil {
			t.Fatal(err)
		}
		c, err := e.App.CheckSave(s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	kinds := func(c *app.SaveCheck) []string {
		var out []string
		for _, is := range c.Issues {
			k := is.Level + " " + is.Kind
			for _, m := range is.Mods {
				k += " " + m.PackageID
			}
			out = append(out, k)
		}
		return out
	}

	c := check("Tundra")
	if c.Level != "" || len(c.Issues) != 0 || c.Quick == nil || c.Quick.Action != app.FixLaunch {
		t.Fatalf("same Mods: level %q, issues %v, quick %+v", c.Level, kinds(c), c.Quick)
	}

	c = check("Tundra 2")
	want := []string{"warning disabled tester.gamma", "warning not-installed tester.gone", "info reordered tester.beta"}
	if !slices.Equal(kinds(c), want) {
		t.Fatalf("issues %q, want %q", kinds(c), want)
	}
	if c.Level != app.LevelWarning || c.Quick.Action != app.FixAll || c.Quick.Command != "saves fix "+c.Save.Ref+" --launch" {
		t.Fatalf("verdict %q quick %+v", c.Level, c.Quick)
	}
	if got := c.Issues[1].Fixes[0]; got.Action != app.FixSubscribe || got.Command != "saves fix "+c.Save.Ref+" --mod tester.gone" {
		t.Fatalf("not-installed fix %+v", got)
	}

	c = check("Broken")
	if c.Level != app.LevelError || c.Issues[0].Kind != app.IssueCutOff || c.Quick.Action != app.FixRecycle {
		t.Fatalf("cut off: level %q issues %v quick %+v", c.Level, kinds(c), c.Quick)
	}
}

func TestFixSave(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	before := e.MustRun("loadout", "export")
	s, _, err := e.App.FindSave("Tundra 2")
	if err != nil {
		t.Fatal(err)
	}
	dry, err := e.App.FixSave(s, app.SaveFixOptions{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Enable) != 1 || len(dry.Subscribe) != 1 || dry.Edit != nil || e.MustRun("loadout", "export") != before {
		t.Fatalf("dry run: %+v", dry)
	}
	r, err := e.App.FixSave(s, app.SaveFixOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if ids := r.WorkshopIDs(); !slices.Equal(ids, []string{"99"}) || r.Edit == nil {
		t.Fatalf("fix: subscribe %v edit %v", ids, r.Edit)
	}
	order := func() []string {
		var out []string
		for _, line := range strings.Split(e.MustRun("loadout", "export"), "\n") {
			if f := strings.Fields(line); len(f) > 0 && !strings.HasPrefix(line, "#") {
				out = append(out, f[0])
			}
		}
		return out
	}
	if got := order(); !slices.Equal(got, []string{"ludeon.rimworld", "tester.alpha", "tester.gamma", "tester.gone", "tester.beta"}) {
		t.Fatalf("after fix: %v", got)
	}
	// The Workshop Mod is now a missing entry: still not installed, so the save still loses
	// its content, and fixing again only subscribes again (no list change, no undo step).
	c, err := e.App.CheckSave(s)
	if err != nil {
		t.Fatal(err)
	}
	if c.Level != app.LevelWarning || c.Issues[0].Kind != app.IssueNotInstalled || c.Issues[0].Fixes[0].Label != "Subscribe" || c.Quick.Action != app.FixAll {
		t.Fatalf("after fix: level %q issues %+v quick %+v", c.Level, c.Issues, c.Quick)
	}
	again, err := e.App.FixSave(s, app.SaveFixOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed() || again.Edit != nil || len(again.Waiting) != 1 || !slices.Equal(again.WorkshopIDs(), []string{"99"}) {
		t.Fatalf("fix again: %+v", again)
	}
	if _, err := e.App.FixSave(s, app.SaveFixOptions{Order: true}, false); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []string{"ludeon.rimworld", "tester.beta", "tester.alpha", "tester.gamma", "tester.gone"}) {
		t.Fatalf("after order: %v", got)
	}
	e.MustRun("undo")
	e.MustRun("undo")
	if e.MustRun("loadout", "export") != before {
		t.Fatal("undo did not restore the list")
	}
	if _, err := e.App.FixSave(s, app.SaveFixOptions{Mods: []string{"tester.none"}}, true); err == nil {
		t.Fatal("fixed a Mod the save does not have")
	}
}

func TestSaveFileActions(t *testing.T) {
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
	to, err := e.App.RenameSave(find("Ashfall/Landing"), "First day")
	if err != nil || filepath.Base(to) != "Ashfall#§#First day.rws" {
		t.Fatalf("rename: %s %v", to, err)
	}
	if _, err := e.App.RenameSave(find("Tundra"), "Tundra 2"); err == nil {
		t.Fatal("renamed over an existing save")
	}
	if _, err := e.App.RenameSave(find("Tundra"), `a\b`); err == nil {
		t.Fatal("accepted a path in a name")
	}
	cp, err := e.App.CopySave(find("Tundra"))
	if err != nil || filepath.Base(cp) != "Tundra (copy).rws" {
		t.Fatalf("copy: %s %v", cp, err)
	}
	if cp2, _ := e.App.CopySave(find("Tundra")); filepath.Base(cp2) != "Tundra (copy 2).rws" {
		t.Fatalf("second copy: %s", cp2)
	}

	saves := []*app.Save{find("Tundra"), find("Broken")}
	prev, err := e.App.BackupSaves(saves, false)
	if err != nil || prev.Done || prev.Bytes == 0 {
		t.Fatalf("backup preview: %+v %v", prev, err)
	}
	if _, err := os.Stat(prev.Dir); err == nil {
		t.Fatal("preview wrote the backup folder")
	}
	done, err := e.App.BackupSaves(saves, true)
	if err != nil || !done.Done || len(done.Errs) != 0 {
		t.Fatalf("backup: %+v %v", done, err)
	}
	if _, err := os.Stat(filepath.Join(done.Dir, "Broken.rws")); err != nil {
		t.Fatal(err)
	}
	if orig, _ := os.ReadFile(find("Tundra").Path); len(orig) == 0 {
		t.Fatal("backup changed the original")
	}

	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, filepath.Base(p)); return os.Remove(p) }
	if r, err := e.App.TrashSaves([]*app.Save{find("Broken")}, false); err != nil || r.Done || len(trashed) != 0 {
		t.Fatalf("trash preview: %+v %v", r, err)
	}
	if r, err := e.App.TrashSaves([]*app.Save{find("Broken")}, true); err != nil || !r.Done || len(r.Errs) != 0 || !slices.Equal(trashed, []string{"Broken.rws"}) {
		t.Fatalf("trash: %+v %v %v", r, err, trashed)
	}
}
