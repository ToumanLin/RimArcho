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
