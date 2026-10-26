package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 04 §1.1 (2026-10-25): lists are shared by every game. Lists made in one game show in
// another; their Core entry is read as that game's Core; each game keeps its own current
// list; removing a game keeps the lists.
func TestPlansSharedAcrossGames(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("plan", "new", "raid", "--from", "current")
	e.MustRun("plan", "new", "chill", "--from", "current")
	mainCore := refID(t, e, "Core", "")

	g2 := secondGame(e, "Game15")
	e.MustRun("env", "add", "old", "--game", g2, "--saves", "own")
	e.MustRun("env", "use", "old")
	scanNow(t, e)

	ps, err := e.App.Plans()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if !slices.Contains(names, "default") || !slices.Contains(names, "raid") || !slices.Contains(names, "chill") {
		t.Fatalf("lists in the second game: %v", names)
	}
	if len(ps) != 3 {
		t.Fatalf("a game without ModsConfig.xml takes a list, it does not make one: %v", names)
	}

	e.MustRun("plan", "use", "raid")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.Plan.Name != "raid" {
		t.Fatalf("current in old: %s", st.Plan.Name)
	}
	if st.List.Index(mainCore) >= 0 {
		t.Fatal("the first game's Core is still in the list as the second game reads it")
	}
	got := joined(active(t, e))
	if !strings.HasPrefix(got, "Core, Harmony, A") {
		t.Fatalf("raid in old: %s", got)
	}
	if st.Ix.ByID[st.List[0].Instance] == nil || st.List[0].Instance == mainCore {
		t.Fatalf("Core is not old's own Core: %+v", st.List[0])
	}

	// Each game remembers its own current list.
	e.MustRun("env", "use", "main")
	if st, _ := e.App.Loadout(); st.Plan.Name != "default" {
		t.Fatalf("main's current list changed: %s", st.Plan.Name)
	}
	if got := joined(active(t, e)); !strings.HasPrefix(got, "Core, Harmony, A") {
		t.Fatalf("default in main: %s", got)
	}

	// Names are unique across games.
	e.MustRun("env", "use", "old")
	if out, code := e.Run("plan", "new", "Raid"); code == 0 || !strings.Contains(out, "already exists") {
		t.Fatalf("a second raid: exit %d\n%s", code, out)
	}

	// Removing the first game keeps every list; its Core becomes a packageId the next
	// game resolves.
	e.MustRun("env", "remove", "main", "--yes")
	if ps, _ := e.App.Store.Plans(); len(ps) != 3 {
		t.Fatalf("lists after removing main: %d", len(ps))
	}
	e.MustRun("plan", "use", "default")
	if got := joined(active(t, e)); !strings.HasPrefix(got, "Core, Harmony, A") {
		t.Fatalf("default after removing main: %s", got)
	}
}

// 04 §1.7, 06 §2.9: every game's own Mods/ is in the mod library, and a list enables such
// a mod in any game: saving links it into this game's Mods/ as _Archo_env_<ID>. A mod this
// game has in its own Mods/ is used instead of another game's copy.
func TestOtherGamesModsUsableAnywhere(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	g2 := secondGame(e, "Game15")
	e.WriteMod(filepath.Join(g2, "Mods", "OldOnly"), testkit.Sample("tester.oldonly", "Old Only"))
	e.WriteMod(filepath.Join(g2, "Mods", "OldB"), testkit.Sample("tester.b", "Old B"))
	e.MustRun("env", "add", "old", "--game", g2, "--saves", "own")
	e.MustRun("env", "use", "old")
	scanNow(t, e)
	oldOnly := refID(t, e, "Old Only", "")
	e.MustRun("env", "use", "main")

	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m := ix.ByID[oldOnly]
	if m == nil || m.Kind != source.Elsewhere {
		t.Fatalf("the other game's Mods/ mod is not in this game's index: %+v", m)
	}
	for _, x := range ix.Mods {
		if x.Name == "Old B" {
			if o := ix.Elsewhere(x); o == nil || o.Name != "B" {
				t.Fatalf("a list naming the other game's B does not use this game's own B: %+v", o)
			}
		}
	}
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if i.Mod != nil && i.Mod.ID == oldOnly && i.Source == app.LibLocal {
				found = true
				// The source says which game's Mods/ (local-<game>); source:local takes all.
				if len(i.DirectoryValues) != 1 || i.DirectoryValues[0] != "local-old" ||
					!app.LocalMatches("local", "local-old") || app.LocalMatches("local-main", "local-old") {
					t.Fatalf("source value of the other game's Mods/: %v", i.DirectoryValues)
				}
			}
		}
	}
	if !found {
		t.Fatal("the mod library does not list the other game's Mods/")
	}

	e.MustRun("enable", "#"+oldOnly[:6])
	e.MustRun("save")
	link := filepath.Join(e.Game, "Mods", "_Archo_env_"+oldOnly)
	if !source.IsLink(link) {
		t.Fatalf("saving did not link the other game's mod into Mods/: %s", link)
	}
	if !strings.Contains(e.ReadModsConfig(), "tester.oldonly") {
		t.Fatal("ModsConfig.xml lacks the linked mod")
	}
	scanNow(t, e)
	if got := joined(active(t, e)); !strings.Contains(got, "Old Only") {
		t.Fatalf("after a scan the list lost the linked mod: %s", got)
	}
}
