package app_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The three groups of 12 §3.1 and the player's functional mark (12 §3.3 rule 2): seen in
// play, only set up (research not begun), never seen; a mark moves a Mod in or out of the
// functional ones, and clearing it lets detection decide again.
func TestSaveReviewGroups(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Races.xml"),
		"<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef></Defs>\n")
	weapon := func(n string) string {
		return "<ThingDef><defName>" + n + "</defName><category>Item</category><tools><li><label>edge</label></li></tools></ThingDef>"
	}
	e.AddWorkshop("3000000011", testkit.Mod{PackageID: "tester.seen", Name: "Seen", Files: map[string]string{"Defs/A.xml": "<Defs>" + weapon("Sword") + "</Defs>\n"}})
	e.AddWorkshop("3000000012", testkit.Mod{PackageID: "tester.planned", Name: "Planned", Files: map[string]string{"Defs/R.xml": "<Defs><ResearchProjectDef><defName>Rocketry</defName></ResearchProjectDef></Defs>\n"}})
	e.AddWorkshop("3000000013", testkit.Mod{PackageID: "tester.unseen", Name: "Unseen", Files: map[string]string{"Defs/U.xml": "<Defs>" + weapon("Halberd") + "</Defs>\n"}})
	e.AddWorkshop("3000000014", testkit.Mod{PackageID: "tester.library", Name: "Library", Files: map[string]string{"Defs/L.xml": "<Defs>" + weapon("LibThing") + "</Defs>\n"}})
	e.AddWorkshop("3000000015", testkit.Mod{PackageID: "tester.menus", Name: "Menus", Files: map[string]string{"Defs/K.xml": "<Defs><KeyBindingDef><defName>MenuKey</defName></KeyBindingDef></Defs>\n"}})
	mods := []string{"ludeon.rimworld", "tester.seen", "tester.planned", "tester.unseen", "tester.library", "tester.menus"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Hour))
	writeOwnersSave(t, e, "Fort 1.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1,
		Body: `<maps><li><uniqueID>0</uniqueID><things><thing Class="ThingWithComps"><def>Sword</def><id>Sword1</id></thing></things></li></maps>
<researchManager><progress><keys><li>Rocketry</li></keys><values><li>0</li></values></progress></researchManager>`})

	groups := func() map[string]app.ReviewMod {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
		if err != nil {
			t.Fatal(err)
		}
		by := map[string]app.ReviewMod{}
		for _, m := range r.Mods {
			by[m.PackageID] = m
		}
		return by
	}
	by := groups()
	want := map[string]string{"tester.seen": app.ReviewSeen, "tester.planned": app.ReviewConfigured, "tester.unseen": app.ReviewNeverSeen,
		"tester.library": app.ReviewNeverSeen, "tester.menus": app.ReviewFunctional}
	for pkg, g := range want {
		if m := by[pkg]; m.Group() != g {
			t.Errorf("%s: group %q, want %q (%+v)", pkg, m.Group(), g, m)
		}
	}
	if m := by["tester.planned"]; !slices.Equal(m.Configured, []string{"Rocketry"}) || m.Seen != 0 {
		t.Errorf("planned: %+v", m)
	}
	if m := by["tester.menus"]; !m.Detected || m.Marked != "" {
		t.Errorf("menus: %+v", m)
	}

	// the marks: a library with a Def becomes functional, a detected one not functional
	if _, err := e.App.SetFunctional("tester.library", app.MarkFunctional); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.SetFunctional("tester.menus", app.MarkNotFunctional); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.SetFunctional("tester.menus", "maybe"); err == nil {
		t.Error("an unknown mark was accepted")
	}
	by = groups() // from the kept rows: a mark needs no new attribution
	if m := by["tester.library"]; m.Group() != app.ReviewFunctional || m.Marked != app.MarkFunctional || m.Detected {
		t.Errorf("marked library: %+v", m)
	}
	if m := by["tester.menus"]; m.Group() != app.ReviewNeverSeen || m.Marked != app.MarkNotFunctional || !m.Detected {
		t.Errorf("marked menus: %+v", m)
	}
	if _, err := e.App.SetFunctional("tester.library", ""); err != nil {
		t.Fatal(err)
	}
	if m := groups()["tester.library"]; m.Group() != app.ReviewNeverSeen || m.Marked != "" {
		t.Errorf("cleared library: %+v", m)
	}
}
