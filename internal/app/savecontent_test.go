package app_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func contentFixture(t testing.TB) *testkit.Env {
	e := testkit.New(t)
	mods := []string{"ludeon.rimworld", "tester.a", "tester.b", "tester.c"}
	defs := []string{
		`<ThingDef Name="Base" Abstract="True"><category>Item</category><tools><li/></tools></ThingDef>
<ThingDef Name="NoDefName" Abstract="True"><category>Building</category></ThingDef>
<ThingDef Abstract="True"><defName>AbstractOnly</defName><category>Item</category></ThingDef>
<ThingDef><defName>Shared</defName><category>Item</category></ThingDef>
<FactionDef><defName>Shared</defName><hidden>True</hidden></FactionDef>
<PawnKindDef><defName>Typed</defName><race>Human</race></PawnKindDef>
<ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef>
<ThingDef><defName>Wolf</defName><race/></ThingDef>
<ThingDef><defName>Mech</defName><race><fleshType>Mechanoid</fleshType></race></ThingDef>
<FleshTypeDef><defName>DroneFlesh</defName><isOrganic>False</isOrganic></FleshTypeDef>
<ThingDef><defName>Drone</defName><race><fleshType>DroneFlesh</fleshType></race></ThingDef>
<ThingDef><defName>ToolUser</defName><race><intelligence>ToolUser</intelligence></race></ThingDef>
<ThingDef ParentName="Base"><defName>ToolsOnly</defName></ThingDef>
<ThingDef ParentName="Base"><defName>Clothes</defName><apparel/><verbs><li/></verbs></ThingDef>
<ThingDef><defName>Table</defName><category>Building</category></ThingDef>
<ThingDef><defName>Rock</defName><category>Building</category><building><isNaturalRock>True</isNaturalRock></building></ThingDef>
<ThingDef><defName>Plant</defName><thingClass>Plant</thingClass></ThingDef>
<ThingDef><defName>CustomPlant</defName><thingClass>Example.CustomPlant</thingClass><plant/></ThingDef>
<ThingDef><defName>CustomNamedPlant</defName><thingClass>Example.Plant_Custom</thingClass><plant/></ThingDef>
<FactionDef><defName>Meat_Wolf</defName></FactionDef>
<ThingDef><defName>Mini</defName><thingClass>MinifiedThing</thingClass><category>Item</category></ThingDef>
<ThingDef Name="LoopA" ParentName="LoopB" Abstract="True"/><ThingDef Name="LoopB" ParentName="LoopA" Abstract="True"/>
<ThingDef ParentName="LoopA"><defName>Cyclic</defName></ThingDef>
<ThingDef ParentName="Absent"><defName>MissingParent</defName></ThingDef>
<ThingDef MayRequire="missing.pkg"><defName>Guarded</defName><category>Item</category></ThingDef>
<ThingDef MayRequireAnyOf="missing.pkg,tester.b"><defName>AnyGuard</defName><category>Item</category></ThingDef>
<ThingDef><defName>NestedGuard</defName><category>Item</category><tools><li MayRequire="missing.pkg"/></tools></ThingDef>
<ResearchProjectDef><defName>Project</defName></ResearchProjectDef><AbilityDef><defName>Power</defName></AbilityDef><SkillDef><defName>Craft</defName></SkillDef><TerrainDef><defName>Soil</defName></TerrainDef>`,
		`<ThingDef ParentName="Base"><defName>ChildB</defName><tools/></ThingDef>
<ThingDef ParentName="Base"><defName>ResetTools</defName><tools Inherit="False"/></ThingDef>
<ThingDef ParentName="NoDefName"><defName>AbstractParent</defName></ThingDef>
<ThingDef><defName>Typed</defName><category>Item</category></ThingDef>`,
		`<ThingDef Name="Base" Abstract="True"><category>Building</category></ThingDef>
<ThingDef><defName>Shared</defName><category>Building</category></ThingDef>
<ThingDef MayRequire="missing.pkg"><defName>ToolsOnly</defName><category>Building</category></ThingDef>`,
	}
	for i, d := range defs {
		e.AddWorkshop(fmt.Sprint(3100000000+i), testkit.Mod{PackageID: mods[i+1], Name: []string{"Alpha", "Beta", "Gamma"}[i], Files: map[string]string{"Defs/Content.xml": "<Defs>" + d + "</Defs>"}})
	}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	return e
}

func contentOf(t testing.TB, e *testkit.Env, body string, mods ...string) *app.SaveContentResult {
	t.Helper()
	if len(mods) == 0 {
		mods = []string{"ludeon.rimworld", "tester.a", "tester.b", "tester.c"}
	}
	e.WriteSave("Content.rws", testkit.SaveSpec{Mods: mods, Body: body})
	s, _, err := e.App.FindSave("Content")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.App.SaveFacts(s, true); err != nil {
		t.Fatal(err)
	}
	c, err := e.App.SaveContent(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func contentEntries(c *app.SaveContentResult) map[string]string {
	out := map[string]string{}
	for _, m := range c.Mods {
		for _, cat := range m.Cats {
			for _, n := range cat.Names {
				out[m.Name+"/"+cat.Key+"/"+n.Name] = fmt.Sprint(n.Uses)
			}
		}
	}
	return out
}

func TestSaveContentCandidates(t *testing.T) {
	e := contentFixture(t)
	c := contentOf(t, e, `<things><li><def>Shared</def></li><li><def>Shared</def></li><li><kindDef>Typed</kindDef></li><li><def>AbstractOnly</def></li></things>`)
	got := contentEntries(c)
	for key, want := range map[string]string{"Alpha/factions/Shared": "2", "Gamma/buildings/Shared": "2", "Alpha/races/Human": "1"} {
		if got[key] != want {
			t.Errorf("%s=%q want %q; %v", key, got[key], want, got)
		}
	}
	if len(got) != 3 || c.Defs != 3 {
		t.Fatalf("typed reference counted wrong candidates: %+v", c)
	}
	if c.Unknown == nil || c.Unknown.Count != 1 || c.Unknown.Names[0].Name != "AbstractOnly" {
		t.Fatalf("abstract: %+v", c.Unknown)
	}
	if c.Mods[0].Cats[0].Hidden != 1 {
		t.Fatal("hidden faction not marked")
	}
	// Override by type follows the save, rather than the current plan.
	c = contentOf(t, e, `<x><def>Shared</def></x>`, "ludeon.rimworld", "tester.c", "tester.a", "tester.b")
	got = contentEntries(c)
	if got["Alpha/items/Shared"] != "1" || got["Alpha/factions/Shared"] != "1" || len(got) != 2 {
		t.Fatalf("save order: %v", got)
	}
	// An untyped name retains every type, including the two defined by different Mods; the
	// pawn kind counts as its race (12 §2.2).
	c = contentOf(t, e, `<x><def>Typed</def></x>`)
	got = contentEntries(c)
	if len(got) != 2 || got["Beta/items/Typed"] != "1" || got["Alpha/races/Human"] != "1" {
		t.Fatalf("untyped: %v", got)
	}
}

func TestSaveContentPredicatesAndInheritance(t *testing.T) {
	e := contentFixture(t)
	names := []string{"ChildB", "ResetTools", "AbstractParent", "ToolsOnly", "Clothes", "Human", "Wolf", "Mech", "Drone", "ToolUser", "Table", "Rock", "Plant", "CustomPlant", "CustomNamedPlant", "Cyclic", "MissingParent", "Guarded", "AnyGuard", "NestedGuard"}
	var body strings.Builder
	body.WriteString("<things>")
	for _, n := range names {
		fmt.Fprintf(&body, "<li><def>%s</def></li>", n)
	}
	body.WriteString("</things>")
	c := contentOf(t, e, body.String())
	got := contentEntries(c)
	for key, want := range map[string]string{"Beta/weapons/ChildB": "1", "Beta/items/ResetTools": "1", "Beta/buildings/AbstractParent": "1", "Alpha/weapons/ToolsOnly": "1", "Alpha/apparel/Clothes": "1", "Alpha/races/Human": "1", "Alpha/animals/Wolf": "1", "Alpha/mechanoids/Mech": "1", "Alpha/creatures/Drone": "1", "Alpha/creatures/ToolUser": "1", "Alpha/buildings/Table": "1", "Alpha/other/Rock": "1", "Alpha/plants/Plant": "1", "Alpha/plants/CustomPlant": "1", "Alpha/other/Cyclic": "1", "Alpha/other/MissingParent": "1", "Alpha/items/AnyGuard": "1", "Alpha/items/NestedGuard": "1"} {
		if got[key] != want {
			t.Errorf("%s=%q want %q; %v", key, got[key], want, got)
		}
	}
	for _, m := range c.Mods {
		for _, cat := range m.Cats {
			for _, n := range cat.Names {
				if (n.Name == "Cyclic" || n.Name == "MissingParent") && !n.Unclassified {
					t.Error("missing/cyclic inheritance silently classified")
				}
				if (n.Name == "CustomPlant" || n.Name == "CustomNamedPlant") && !n.Inferred {
					t.Error("custom plant must be inferred")
				}
			}
		}
	}
	if got["Alpha/plants/CustomNamedPlant"] != "1" {
		t.Fatalf("custom plant classification: %v", got)
	}
	if c.Unknown == nil || c.Unknown.Count != 1 || c.Unknown.Names[0].Name != "Guarded" {
		t.Fatalf("MayRequire: %+v", c.Unknown)
	}
}

func TestSaveContentGeneratedAndCache(t *testing.T) {
	e := contentFixture(t)
	body := `<things><li><def>Corpse_Human</def><inner><def>Human</def></inner></li><li><def>Blueprint_Table</def></li><li><def>Blueprint_Install_Table</def></li><li><def>Frame_Table</def></li><li><def>Mini</def><inner><def>Table</def></inner></li><li><def>Techprint_Project</def></li><li><def>Meat_Wolf</def></li><li><def>Psytrainer_Power</def></li><li><def>Neurotrainer_Craft</def></li><li><def>Make_Table</def></li><li><def>Administer_ToolsOnly</def></li><li><def>Table_Rough</def></li><li><def>Table_RoughHewn</def></li><li><def>Table_Smooth</def></li><li><stuff>Blueprint_Soil</stuff></li><li><def>Meat_Shared</def></li></things>`
	c := contentOf(t, e, body)
	got := contentEntries(c)
	if got["Alpha/races/Human"] != "2" || got["Alpha/buildings/Table"] != "4" {
		t.Fatalf("wrapper base: %v", got)
	}
	if got["Alpha/items/Techprint_Project"] != "1" || got["Alpha/recipes/Make_Table"] != "1" || got["Alpha/terrain/Table_Rough"] != "1" {
		t.Fatalf("generated categories: %v", got)
	}
	for key := range got {
		if strings.Contains(key, "/Mini") {
			t.Fatal("minified wrapper counted")
		}
	}
	if got["Alpha/terrain/Soil"] != "1" || got["Gamma/items/Meat_Shared"] != "1" {
		t.Fatalf("typed terrain wrapper/generated base type: %v", got)
	}
	if got["Alpha/items/Meat_Wolf"] != "1" || got["Alpha/factions/Meat_Wolf"] != "1" {
		t.Fatalf("untyped generated name lost a type candidate: %v", got)
	}
	if c.Unknown != nil {
		t.Fatalf("generated unmatched: %+v", c.Unknown)
	}
	s, _, err := e.App.FindSave("Content")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.Path); err != nil {
		t.Fatal(err)
	}
	if cached, err := e.App.SaveContent(s); err != nil || !cached.Read || len(contentEntries(cached)) != len(got) {
		t.Fatalf("L2 must not read save: %v %v", cached, err)
	}
	e.WriteSave("Unread.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}})
	s, _, err = e.App.FindSave("Unread")
	if err != nil {
		t.Fatal(err)
	}
	if unread, err := e.App.SaveContent(s); err != nil || unread.Read || unread.Note == "" {
		t.Fatalf("unread: %+v %v", unread, err)
	}
}

func TestSaveContentUnchecked(t *testing.T) {
	e := contentFixture(t)
	c := contentOf(t, e, `<x><def>ToolsOnly</def></x>`)
	if len(c.Unchecked) != 0 {
		t.Fatalf("fresh extraction not ready: %v", c.Unchecked)
	}
	if _, err := e.App.Store.DB.Exec(`UPDATE fact_set SET def_content_version=0`); err != nil {
		t.Fatal(err)
	}
	s, _, err := e.App.FindSave("Content")
	if err != nil {
		t.Fatal(err)
	}
	c, err = e.App.SaveContent(s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Unchecked, []string{"Core", "Alpha", "Beta", "Gamma"}) {
		t.Fatalf("per Mod readiness: %v", c.Unchecked)
	}
	if len(c.Mods) != 0 || len(c.Totals) != 0 || c.Unknown != nil {
		t.Fatalf("old facts must not become classified totals or unmatched sources: %+v", c)
	}
}

func TestSaveContentNameBoundsAndPartial(t *testing.T) {
	e := testkit.New(t)
	var defs, body strings.Builder
	defs.WriteString("<Defs>")
	for i := range 130 {
		fmt.Fprintf(&defs, "<RecipeDef><defName>Recipe%03d</defName></RecipeDef>", i)
		fmt.Fprintf(&body, "<x><def>Recipe%03d</def><def>Gone%03d</def></x>", i, i)
	}
	defs.WriteString("</Defs>")
	body.WriteString("<x><def>Recipe129</def><def>Recipe129</def></x>")
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Content.xml"), defs.String())
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	c := contentOf(t, e, body.String(), "ludeon.rimworld")
	cat := c.Mods[0].Cats[0]
	if cat.Count != 130 || cat.Uses != 132 || len(cat.Names) != 100 || cat.Names[0].Name != "Recipe129" || cat.Names[1].Name != "Recipe000" {
		t.Fatalf("names/counts: %+v", cat)
	}
	if c.Unknown == nil || c.Unknown.Count != 130 || len(c.Unknown.Names) != 100 || len(c.Totals[0].Names) != 0 {
		t.Fatalf("unknown/totals bounds: %+v", c)
	}
	c = contentOf(t, e, `<x><def>Recipe000</def></wrong>`, "ludeon.rimworld")
	if !c.Read || c.Note == "" || len(c.Mods) != 1 || c.Mods[0].Defs != 1 {
		t.Fatalf("partial facts: %+v", c)
	}
}

func TestSaveContentOfficialAndSavePackages(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Content.xml"), `<Defs><ThingDef><defName>CoreItem</defName><category>Item</category></ThingDef></Defs>`)
	e.WriteMod(filepath.Join(e.Game, "Data", "Royalty"), testkit.Mod{PackageID: "ludeon.rimworld.royalty", Name: "Royalty", Files: map[string]string{"Defs/Content.xml": `<Defs><AbilityDef MayRequire="ludeon.rimworld.royalty"><defName>DLCAbility</defName></AbilityDef></Defs>`}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	c := contentOf(t, e, `<x><def>CoreItem</def><def>DLCAbility</def></x>`, "ludeon.rimworld", "ludeon.rimworld.royalty")
	got := contentEntries(c)
	if got["Core/items/CoreItem"] != "1" || got["Royalty/abilities/DLCAbility"] != "1" || len(c.Unchecked) != 0 {
		t.Fatalf("Core/DLC and save packages: %v %+v", got, c)
	}
	c = contentOf(t, e, `<x><def>DLCAbility</def></x>`, "ludeon.rimworld")
	if len(c.Mods) != 0 || c.Unknown == nil || c.Unknown.Count != 1 {
		t.Fatalf("DLC outside save counted: %+v", c)
	}
}

// A cache row of an older parser that means the same (savefile.MinParserVersion) keeps the
// save's summary and play facts, so a parser change that only adds facts does not unread
// every save; the content asks for a new read, which SaveFacts with read does (12 §1.7).
func TestSaveContentOlderParserRow(t *testing.T) {
	e := contentFixture(t)
	contentOf(t, e, `<things><li><def>Shared</def></li></things>`)
	s, _, err := e.App.FindSave("Content")
	if err != nil {
		t.Fatal(err)
	}
	key := env.PathKey(s.Path)
	c, err := e.App.Store.SaveFacts(key)
	if err != nil || c == nil {
		t.Fatalf("cached row %v %v", c, err)
	}
	var f savefile.Facts
	if err := json.Unmarshal(c.Facts, &f); err != nil {
		t.Fatal(err)
	}
	f.Parser, f.DefRefs = savefile.MinParserVersion, nil
	if c.Facts, err = json.Marshal(&f); err != nil {
		t.Fatal(err)
	}
	c.Parser = savefile.MinParserVersion
	if err := e.App.Store.PutSaveCache(c); err != nil {
		t.Fatal(err)
	}

	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	s, err = l.Find(s.Ref)
	if err != nil || s.Summary == nil {
		t.Fatalf("an older parser's row lost the summary: %+v %v", s, err)
	}
	if old, err := e.App.SaveFacts(s, false); err != nil || old == nil || old.Parser != savefile.MinParserVersion {
		t.Fatalf("cached facts without read: %+v %v", old, err)
	}
	r, err := e.App.SaveContent(s)
	if err != nil || !r.Stale || r.Read {
		t.Fatalf("content from an older row: %+v %v", r, err)
	}
	if nf, err := e.App.SaveFacts(s, true); err != nil || nf == nil || nf.Parser != savefile.ParserVersion || len(nf.DefRefs) == 0 {
		t.Fatalf("read again: %+v %v", nf, err)
	}
	if r, err = e.App.SaveContent(s); err != nil || r.Stale || !r.Read || len(r.Mods) == 0 {
		t.Fatalf("content after the read: %+v %v", r, err)
	}
}
