package cmds_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestGoldenSavesContent(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Content.xml"), `<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><ThingDef><defName>Gun</defName><category>Item</category><tools><li/></tools></ThingDef></Defs>`)
	e.AddWorkshop("3100000000", testkit.Mod{PackageID: "tester.content", Name: "Content Mod", Files: map[string]string{"Defs/Content.xml": `<Defs><FactionDef><defName>Clan</defName><hidden>True</hidden></FactionDef><RecipeDef><defName>Recipe</defName></RecipeDef></Defs>`}})
	mods := []string{"ludeon.rimworld", "tester.content"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.WriteSave("Content.rws", testkit.SaveSpec{Mods: mods, Body: `<things><li><def>Gun</def></li><li><def>Corpse_Human</def></li><li><def>Clan</def></li><li><def>Gone</def></li><li><bills><li><recipe>Recipe</recipe></li></bills></li></things>`})
	runSavesGolden(t, "saves_content", e, []step{
		cmd("saves", "show", "Content", "--content"),
		act(func(e *testkit.Env) { e.MustRun("analyze") }),
		cmd("saves", "show", "Content", "--content"),
		cmd("saves", "show", "Content", "--content", "--all"),
		// can I remove this Mod? by packageId, by name, and one the save does not have
		cmd("saves", "show", "Content", "--mod", "tester.content"),
		cmd("saves", "show", "Content", "--mod", "Core"),
		cmd("saves", "show", "Content", "--mod", "tester.nothere"),
	})
}

// saves review (12 §3): before and after the saves are read, one Mod in full, one colony.
func TestGoldenSavesReview(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Content.xml"), `<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef></Defs>`)
	weapon := func(n string) string {
		return "<ThingDef><defName>" + n + "</defName><category>Item</category><tools><li><label>edge</label></li></tools></ThingDef>"
	}
	e.AddWorkshop("3100000001", testkit.Mod{PackageID: "tester.arms", Name: "Arms", Files: map[string]string{"Defs/A.xml": "<Defs>" + weapon("Sword") + weapon("Axe") + weapon("Club") +
		"<ResearchProjectDef><defName>Smithing</defName></ResearchProjectDef><QuestScriptDef><defName>ArmsRace</defName></QuestScriptDef><RecipeDef><defName>MakeSword</defName></RecipeDef></Defs>"}})
	e.AddWorkshop("3100000002", testkit.Mod{PackageID: "tester.hats", Name: "Hats", Files: map[string]string{"Defs/H.xml": "<Defs><ThingDef><defName>Cap</defName><category>Item</category><apparel><layers><li>Overhead</li></layers></apparel></ThingDef></Defs>"}})
	e.AddWorkshop("3100000003", testkit.Mod{PackageID: "tester.menus", Name: "Menus", Files: map[string]string{"Defs/K.xml": "<Defs><KeyBindingDef><defName>MenuKey</defName></KeyBindingDef></Defs>"}})
	mods := []string{"ludeon.rimworld", "tester.arms", "tester.hats", "tester.menus"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.WriteSave("Fort 1.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1, Body: `<things><li><def>Sword</def></li></things>`})
	e.WriteSave("Fort 2.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1, Body: `<things><li><def>Axe</def><bills><li><recipe>MakeSword</recipe></li></bills></li></things>`})
	e.WriteSave("Camp.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.hats"}, Faction: "Camp", Seed: "beta", PRV: 2})
	runSavesGolden(t, "saves_review", e, []step{
		cmd("saves", "review"),
		cmd("saves", "review", "--read"),
		cmd("saves", "review", "--mod", "tester.arms"),
		cmd("saves", "review", "--colony", "Camp"),
		cmd("saves", "review", "--colony", "Nowhere"),
	})
}

func TestSaveContentTextBudget(t *testing.T) {
	e := testkit.New(t)
	mods := []string{"ludeon.rimworld"}
	var body strings.Builder
	for i := range 45 {
		pkg := fmt.Sprintf("tester.content%d", i)
		name := fmt.Sprintf("Content%d", i)
		mods = append(mods, pkg)
		e.AddWorkshop(fmt.Sprint(3100000000+i), testkit.Mod{PackageID: pkg, Name: name, Files: map[string]string{"Defs/Content.xml": fmt.Sprintf("<Defs><RecipeDef><defName>Recipe%d</defName></RecipeDef></Defs>", i)}})
		fmt.Fprintf(&body, "<bills><li><recipe>Recipe%d</recipe></li></bills>", i)
	}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.WriteSave("Many.rws", testkit.SaveSpec{Mods: mods, Body: body.String()})
	text := e.MustRun("saves", "show", "Many", "--content")
	if n := strings.Count(strings.TrimSpace(text), "\n") + 1; n > 40 {
		t.Fatalf("%d lines:\n%s", n, text)
	}
	if !strings.Contains(text, "--content --all") || !strings.Contains(text, "limits:") {
		t.Fatalf("missing truncation/limits:\n%s", text)
	}
	text = e.MustRun("saves", "show", "Many", "--content", "--all")
	if !strings.Contains(text, "Content44: 1 recipe") {
		t.Fatalf("--all omitted Mods:\n%s", text)
	}
}

// --mod answers for a save that has only game content too (no "removing a Mod" split).
func TestSavesShowModGameOnly(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Content.xml"), `<Defs><ThingDef><defName>Gun</defName><category>Item</category><tools><li/></tools></ThingDef></Defs>`)
	mods := []string{"ludeon.rimworld"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.WriteSave("Plain.rws", testkit.SaveSpec{Mods: mods, Body: `<things><li><def>Gun</def></li></things>`})
	text := e.MustRun("saves", "show", "Plain", "--mod", "ludeon.rimworld")
	if !strings.Contains(text, "Core (ludeon.rimworld): part of the game") {
		t.Fatalf("no verdict for the game's own Mod:\n%s", text)
	}
}
