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
	// Ask the filesystem whether two case variants can name different saves.
	upper := find("Tundra")
	lowerPath := filepath.Join(filepath.Dir(upper.Path), "tundra.rws")
	f, createErr := os.OpenFile(lowerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if createErr == nil {
		if _, err := f.WriteString("other save"); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if _, err := e.App.RenameSave(upper, "tundra"); err == nil {
			t.Fatal("renamed over a different save with a case-only name")
		}
		if b, err := os.ReadFile(lowerPath); err != nil || string(b) != "other save" {
			t.Fatalf("case-only collision changed the other save: %q %v", b, err)
		}
		if err := os.Remove(lowerPath); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsExist(createErr) {
		t.Fatal(createErr)
	}
	if to, err := e.App.RenameSave(upper, "tundra"); err != nil {
		t.Fatal("rename to own case variant:", err)
	} else {
		upper.Path, upper.File = to, filepath.Base(to)
		if _, err := e.App.RenameSave(upper, "Tundra"); err != nil {
			t.Fatal("restore case:", err)
		}
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
	prev, err := e.App.BackupSaves(saves, false, "")
	if err != nil || prev.Done || prev.Bytes == 0 {
		t.Fatalf("backup preview: %+v %v", prev, err)
	}
	if _, err := os.Stat(prev.Dir); err == nil {
		t.Fatal("preview wrote the backup folder")
	}
	done, err := e.App.BackupSaves(saves, true, "")
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
	if r, err := e.App.TrashSaves([]*app.Save{find("Broken")}, false, ""); err != nil || r.Done || len(trashed) != 0 {
		t.Fatalf("trash preview: %+v %v", r, err)
	}
	if r, err := e.App.TrashSaves([]*app.Save{find("Broken")}, true, ""); err != nil || !r.Done || len(r.Errs) != 0 || !slices.Equal(trashed, []string{"Broken.rws"}) {
		t.Fatalf("trash: %+v %v %v", r, err, trashed)
	}
}

func TestSavesFullRead(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	now := e.Now()
	world := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, World: "Ennawrin", Seed: "hollow", PRV: -7,
		Faction: "New Arrivals", Ticks: 25*60000 + 1234, ModTime: now.Add(-4 * time.Hour)}
	e.WriteSave("Rimward.rws", world)
	world.Faction, world.Ticks, world.ModTime = "Iron Pact", 40*60000, now.Add(-3*time.Hour-30*time.Minute)
	e.WriteSave("Landed 2.rws", world)

	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range l.Saves() {
		if s.Summary != nil {
			t.Fatalf("%s: summary before any full read", s.Name)
		}
	}
	rw, _ := l.Find("Rimward")
	if order := l.IndexOrder([]*app.Save{rw}); order[0] != rw || order[1].Name != "Broken" || len(order) != len(l.Saves()) {
		t.Fatalf("index order starts %s, %s", order[0].Name, order[1].Name)
	}
	var lines []string
	if err := e.App.RunSavesIndex("", []string{"Rimward"}, func(line string) { lines = append(lines, line) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) != len(l.Saves()) {
		t.Fatalf("progress %q", lines)
	}
	if done, total, ref, _, ok := app.ParseSavesIndexLine(lines[len(lines)-1]); !ok || done != total || !strings.HasPrefix(ref, "sv:") {
		t.Fatalf("last line %q", lines[len(lines)-1])
	}

	l, err = e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	cols := map[string][]string{}
	for _, c := range l.Colonies {
		for _, s := range c.Saves {
			cols[c.Name+" ("+c.By+")"] = append(cols[c.Name+" ("+c.By+")"], s.Name)
			if s.Summary == nil {
				t.Fatalf("%s: no summary after indexing", s.Name)
			}
		}
	}
	// One world, two file names: one colony named after the newest save's faction.
	if got := cols["Iron Pact (world)"]; !slices.Equal(got, []string{"Landed 2", "Rimward"}) {
		t.Fatalf("colonies %v", cols)
	}
	s, _ := l.Find("Rimward")
	if s.Summary.Day != 25 || s.Summary.World != "Ennawrin" {
		t.Fatalf("summary %+v", s.Summary)
	}
	if b, _ := l.Find("Broken"); !b.Summary.Malformed {
		t.Fatal("the cut-off save is not malformed")
	}
	f, err := e.App.SaveFacts(s, false)
	if err != nil || f == nil || f.Player.Name != "New Arrivals" || f.Day() != 25 {
		t.Fatalf("facts %+v %v", f, err)
	}

	// Indexing again reads nothing; a save written since then has no summary, joins its
	// world colony by name, and is read inline when asked for.
	r, err := e.App.IndexSaves(l.IndexOrder(nil), nil)
	if err != nil || r.Read != 0 || r.Cached != len(l.Saves()) {
		t.Fatalf("second run %+v %v", r, err)
	}
	world.Faction, world.Ticks, world.ModTime = "Iron Pact", 41*60000, now.Add(-2*time.Minute)
	e.WriteSave("Rimward 3.rws", world)
	l, _ = e.App.ListSaves()
	s, _ = l.Find("Rimward 3")
	if s.Summary != nil || s.Colony.Name != "Iron Pact" {
		t.Fatalf("new save: summary %+v colony %s", s.Summary, s.Colony.Name)
	}
	if f, err := e.App.SaveFacts(s, true); err != nil || f == nil || s.Summary == nil || s.Summary.Day != 41 {
		t.Fatalf("inline read: %+v %v", s.Summary, err)
	}

	if err := os.Remove(s.Path); err != nil {
		t.Fatal(err)
	}
	l, _ = e.App.ListSaves()
	if n, err := e.App.PruneSaveCache(l); err != nil || n != 1 {
		t.Fatalf("pruned %d %v", n, err)
	}
}

// The Mod list against the save's list (12 §4.3 Mod list): the save's Mods the list lacks
// (not InList) and the list's Mods the save lacks (ListOnly), each with the installed copy
// it links to in the Mod library.
func TestSaveListDifferences(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	diff := func(name string) []string {
		t.Helper()
		c := checkNamed(t, e, name)
		var out []string
		mark := func(sign string, m app.SaveMod) {
			if m.Library != "" {
				sign += "L"
			}
			out = append(out, sign+" "+m.PackageID)
		}
		for _, m := range c.ListOnly {
			mark("+", m)
		}
		for _, m := range c.Mods {
			if !m.InList {
				mark("-", m)
			}
		}
		return out
	}
	if ch := diff("Tundra"); len(ch) != 0 {
		t.Fatalf("same Mods as the list: %q", ch)
	}
	if ch := diff("Tundra 2"); !slices.Equal(ch, []string{"-L tester.gamma", "- tester.gone"}) {
		t.Fatalf("Tundra 2: %q", ch)
	}
	// The list has a Mod the save lacks; installed, it links.
	e.WriteSave("Tundra 3.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.beta"}, ModTime: e.Now().Add(-5 * time.Minute)})
	if ch := diff("Tundra 3"); !slices.Equal(ch, []string{"+L tester.alpha"}) {
		t.Fatalf("Tundra 3: %q", ch)
	}
}

// A colony renamed in RimArcho (12 §4.1): the listing shows the player's name, keeps the
// saves' name beside it, finds the colony by either, and an empty name gives the saves'
// name back. The save files are not touched.
func TestRenameColony(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	colony := func(ref string) *app.Colony {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		c, err := l.FindColony(ref)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	out := e.MustRun("saves", "rename-colony", "Tundra", "North Base")
	if !strings.Contains(out, "renamed colony Tundra → North Base") {
		t.Fatalf("rename: %s", out)
	}
	c := colony("north base")
	if c.Name != "North Base" || c.GameName != "Tundra" || len(c.Saves) != 3 || colony("Tundra") != nil && colony("Tundra").Key != c.Key {
		t.Fatalf("renamed: %+v", c)
	}
	if _, err := os.Stat(c.Latest().Path); err != nil {
		t.Fatalf("the save file moved: %v", err)
	}
	e.MustRun("saves", "rename-colony", "North Base")
	if c = colony("Tundra"); c.Name != "Tundra" || c.GameName != "" {
		t.Fatalf("reset: %+v", c)
	}
	if err := e.App.RenameColony(c, strings.Repeat("x", app.ColonyNameMax+1)); err == nil {
		t.Fatal("a name past the limit")
	}
}

// saves show and fix --current check and fix against the current game's current list
// instead of the list the save matched (12 §4.3); their commands keep --current.
func TestSavesAgainstCurrentList(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	e.MustRun("plan", "new", "other", "--from", "current")
	e.MustRun("plan", "use", "other")
	e.MustRun("disable", "pkg:tester.beta")
	out := e.MustRun("saves", "show", "Tundra", "--current")
	if !strings.Contains(out, "checked against: the current list plan:other of env:main (--current)") ||
		!strings.Contains(out, "saves fix sv:") || !strings.Contains(out, "--current") || !strings.Contains(out, "list: plan:default") {
		t.Fatalf("show --current:\n%s", out)
	}
	if out := e.MustRun("saves", "show", "Tundra"); strings.Contains(out, "--current") || strings.Contains(out, "checked against") {
		t.Fatalf("show without --current:\n%s", out)
	}
	out = e.MustRun("saves", "fix", "Tundra", "--current", "--dry-run")
	if !strings.Contains(out, "plan:other") {
		t.Fatalf("fix --current:\n%s", out)
	}
}
