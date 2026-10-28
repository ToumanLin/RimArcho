package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// VM run B8: a list with Harmony disabled launched without a word; the game failed to load
// the mod that needs it, reset ModsConfig.xml to the base game, and RimArcho loaded that
// as the list, again without a word.
func b8Env(t *testing.T) *testkit.Env {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.needs", Name: "Needs Harmony", Deps: []string{"brrainz.harmony"}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.needs")
	e.AddEnv(false)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("scan")
	return e
}

func TestLaunchAsksWhenRequiredModsAreMissing(t *testing.T) {
	e := b8Env(t)
	e.MustRun("disable", "pkg:brrainz.harmony")
	file := e.ReadModsConfig()
	o := view.RunAsking(e.App, view.ListLaunch)
	if len(o.Confirm) != 2 || !strings.Contains(o.Confirm[1], "Needs Harmony needs Harmony") || o.Refused != nil {
		t.Fatalf("question %+v", o)
	}
	if len(e.Launched) != 0 || e.ReadModsConfig() != file {
		t.Fatal("asking wrote or launched")
	}
	// launch anyway
	if o := view.Run(e.App, view.ListLaunch); o.Refused != nil || len(e.Launched) != 1 {
		t.Fatalf("anyway %+v, launched %v", o, e.Launched)
	}
	// nothing missing: no question
	e.Launched = nil
	e.MustRun("enable", "pkg:brrainz.harmony")
	if o := view.RunAsking(e.App, view.ListLaunch); o.Confirm != nil || len(e.Launched) != 1 {
		t.Fatalf("asked with nothing missing %+v", o)
	}
}

func TestOutsideChangeThatDropsModsIsSaid(t *testing.T) {
	e := b8Env(t)
	card := func() *view.Need {
		t.Helper()
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range view.BuildNeeds(e.App, st, ps).Items {
			if strings.Contains(it.Title, "left the list") {
				return &it
			}
		}
		return nil
	}
	// the game resets its list to the base game
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	if out := e.MustRun("scan"); !strings.Contains(out, "2 mods left the list (v1 has them)") {
		t.Fatalf("scan:\n%s", out)
	}
	c := card()
	if c == nil || c.Title != "ModsConfig.xml was changed outside RimArcho: 2 mods left the list" || !strings.Contains(c.Why, "Harmony, Needs Harmony") || len(c.Actions) != 3 {
		t.Fatalf("card %+v", c)
	}
	if c.Actions[0].Kind != view.ActView || strings.Join(c.Actions[0].Argv, " ") != "loadout diff v1 v2" {
		t.Fatalf("see the difference %+v", c.Actions[0])
	}
	// put back v1: the list has both again, and the card goes
	if _, err := view.Do(e.App, c.Actions[1], 0); err != nil {
		t.Fatal(err)
	}
	if got := enabled(t, e); !strings.Contains(got, "Harmony") || !strings.Contains(got, "Needs Harmony") || card() != nil {
		t.Fatalf("after putting back: %s, card %+v", got, card())
	}
	// again, kept this time
	e.MustRun("launch")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.MustRun("scan")
	c = card()
	if c == nil {
		t.Fatal("no card the second time")
	}
	if _, err := view.Do(e.App, c.Actions[2], 0); err != nil || card() != nil {
		t.Fatalf("keep: %v, card %+v", err, card())
	}
	// a change that only adds says nothing
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	if out := e.MustRun("scan"); strings.Contains(out, "left the list") || card() != nil {
		t.Fatalf("an addition was said as dropped:\n%s", out)
	}
}
