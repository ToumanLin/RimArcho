package app_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The playthrough review (12 §3): what a Mod adds against what a colony's saves show, all
// of the colony's saves together. A bill, a filter and research not begun are not seen;
// recipes are not counted; a Mod with nothing a save can show is functional; a save
// without a current read is not counted until it is read.
func TestSaveReview(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Races.xml"),
		"<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef></Defs>\n")
	weapon := func(n string) string {
		return "<ThingDef><defName>" + n + "</defName><category>Item</category><tools><li><label>edge</label></li></tools></ThingDef>"
	}
	e.AddWorkshop("3000000001", testkit.Mod{PackageID: "tester.arms", Name: "Arms", Files: map[string]string{"Defs/A.xml": "<Defs>" + weapon("Sword") + weapon("Axe") + weapon("Club") +
		"<ResearchProjectDef><defName>Smithing</defName></ResearchProjectDef><ResearchProjectDef><defName>Forging</defName></ResearchProjectDef><RecipeDef><defName>MakeSword</defName></RecipeDef></Defs>\n"}})
	e.AddWorkshop("3000000002", testkit.Mod{PackageID: "tester.floors", Name: "Floors", Files: map[string]string{"Defs/F.xml": "<Defs><TerrainDef><defName>FancyFloor</defName></TerrainDef></Defs>\n"}})
	e.AddWorkshop("3000000003", testkit.Mod{PackageID: "tester.menus", Name: "Menus", Files: map[string]string{"Defs/K.xml": "<Defs><KeyBindingDef><defName>MenuKey</defName></KeyBindingDef><RecipeDef><defName>MenuRecipe</defName></RecipeDef></Defs>\n"}})
	mods := []string{"ludeon.rimworld", "tester.arms", "tester.floors", "tester.menus"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Hour))

	thing := func(def string) string {
		return `<maps><li><uniqueID>0</uniqueID><things><thing Class="ThingWithComps"><def>` + def + `</def><id>` + def + `1</id></thing></things></li></maps>`
	}
	fort := func(file, body string) {
		writeOwnersSave(t, e, file, testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1, Body: body})
	}
	// the older save has the sword; the newer one the axe, the floor in its terrain grid
	// (FancyFloor's hash, 6939), a bill and a filter naming things, research begun and not
	fort("Fort 1.rws", thing("Sword"))
	fort("Fort 2.rws", thing("Axe")+`<maps><li><uniqueID>1</uniqueID><terrainGrid><topGrid>GxsbGw==</topGrid></terrainGrid><things>
<thing Class="Building"><def>Human</def><id>Human9</id><bills><li><recipe>MakeSword</recipe></li></bills><settings><filter><allowedDefs><li>Club</li></allowedDefs></filter></settings></thing></things></li></maps>
<researchManager><progress><keys><li>Smithing</li><li>Forging</li></keys><values><li>0</li><li>120</li></values></progress></researchManager>`)
	// another colony, without Arms
	writeOwnersSave(t, e, "Camp.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.floors", "tester.menus"}, Faction: "Camp", Seed: "beta", PRV: 2})

	review := func(read bool) *app.SaveReview {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.App.SaveReview(l.Colonies, read, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	// nothing is read yet, and the review reads nothing unless asked
	if r := review(false); r.Saves != 0 || r.Unread != 3 || len(r.Mods) != 0 {
		t.Fatalf("unread: %+v", r)
	}
	r := review(true)
	if r.Colonies != 2 || r.Saves != 3 || r.Unread != 0 {
		t.Fatalf("counts: %+v", r)
	}
	by := map[string]app.ReviewMod{}
	for _, m := range r.Mods {
		by[m.PackageID] = m
	}
	cat := func(m app.ReviewMod, key string) app.ReviewCat {
		for _, c := range m.Cats {
			if c.Key == key {
				return c
			}
		}
		return app.ReviewCat{}
	}
	arms := by["tester.arms"]
	if arms.Colonies != 1 || arms.Saves != 2 || arms.Adds != 5 || arms.Seen != 3 || arms.Functional || len(arms.Cats) != 2 {
		t.Errorf("arms: %+v", arms)
	}
	if w := cat(arms, "weapons"); w.Adds != 3 || w.Seen != 2 || !slices.Equal(w.NotSeen, []string{"Club"}) {
		t.Errorf("weapons: %+v", w)
	}
	if w := cat(arms, "research"); w.Adds != 2 || w.Seen != 1 || !slices.Equal(w.NotSeen, []string{"Smithing"}) {
		t.Errorf("research: %+v", w)
	}
	floors := by["tester.floors"]
	if w := cat(floors, "terrain"); floors.Colonies != 2 || floors.Saves != 3 || w.Adds != 1 || w.Seen != 1 {
		t.Errorf("floors: %+v", floors)
	}
	if m := by["tester.menus"]; !m.Functional || m.Adds != 0 || len(m.Cats) != 0 || m.Colonies != 2 {
		t.Errorf("menus: %+v", m)
	}
	if g := by["ludeon.rimworld"]; !g.Game {
		t.Errorf("core: %+v", g)
	}
	// Mods with something to see first, the lowest share seen first; functional ones last
	var order []string
	for _, m := range r.Mods {
		if !m.Game {
			order = append(order, m.Name)
		}
	}
	if !slices.Equal(order, []string{"Arms", "Floors", "Menus"}) {
		t.Errorf("order: %v", order)
	}
	// the same attribution as the selected save's, which asks the database
	s, _, err := e.App.FindSave("Fort 2")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.App.SaveContent(s)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, m := range c.Mods {
		got[m.PackageID] = m.Defs
	}
	// with what is only set up counted too: the axe, both research projects, the recipe
	if got["tester.arms"] != 4 || got["tester.floors"] != 1 {
		t.Errorf("content: %v", got)
	}
}
