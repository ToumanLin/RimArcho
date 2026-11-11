package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// What a save needs of each Mod (12 §4.7): a race with colonists, a faction and a class
// break the save, as does a map's weather; a building is content lost; a hairstyle is a
// mark; a Mod with nothing in the save is safe; one that is not installed is not checked.
func TestSaveNeeds(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Races.xml"),
		"<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef></Defs>\n")
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", store.GameMainAssembly))
	dll, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	add := func(id, pkg, name string, files map[string]string) {
		e.AddWorkshop(id, testkit.Mod{PackageID: pkg, Name: name, Files: files})
	}
	add("3000000005", "tester.kurin", "Kurin", map[string]string{
		"Defs/Kurin.xml":      "<Defs><ThingDef><defName>Kurin_Race</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>KurinClan</defName></FactionDef></Defs>\n",
		"Assemblies/ModA.dll": string(dll),
	})
	add("3000000006", "tester.chairs", "Chairs", map[string]string{"Defs/C.xml": "<Defs><ThingDef><defName>Stool</defName><category>Building</category></ThingDef></Defs>\n"})
	add("3000000007", "tester.hair", "Hair", map[string]string{"Defs/H.xml": "<Defs><HairDef><defName>Bob</defName></HairDef></Defs>\n"})
	add("3000000008", "tester.rain", "Rain", map[string]string{"Defs/W.xml": "<Defs><WeatherDef><defName>AcidRain</defName></WeatherDef></Defs>\n"})
	add("3000000009", "tester.menus", "Menus", map[string]string{"Defs/M.xml": "<Defs><KeyBindingDef><defName>Unused</defName></KeyBindingDef></Defs>\n"})
	mods := []string{"ludeon.rimworld", "tester.kurin", "tester.chairs", "tester.hair", "tester.rain", "tester.menus"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Hour))

	body := `<maps><li><uniqueID>0</uniqueID><weatherManager><curWeather>AcidRain</curWeather><lastWeather>Clear</lastWeather></weatherManager><things>
<thing Class="Pawn"><def>Kurin_Race</def><id>Kurin_Race1</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story><hairDef>Bob</hairDef></story></thing>
<thing Class="ThingWithComps"><def>Corpse_Kurin_Race</def><id>Corpse_Kurin_Race3</id><innerContainer><innerList><li>
 <def>Kurin_Race</def><id>Kurin_Race4</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /><healthTracker><healthState>Dead</healthState></healthTracker></li></innerList></innerContainer></thing>
<thing Class="Pawn"><def>Human</def><id>Human5</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="Building"><def>Stool</def><id>Stool6</id></thing>
</things></li></maps>
<components><li Class="ModA.CompBroken" /></components>`
	writeOwnersSave(t, e, "Needs.rws", testkit.SaveSpec{Mods: append(slices.Clone(mods), "tester.gone"), Faction: "Colony", Body: body})
	s, _, err := e.App.FindSave("Needs")
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
	by := map[string]app.SaveNeed{}
	for _, n := range c.Needs {
		by[n.PackageID] = n
	}
	if len(c.Needs) != 7 {
		t.Fatalf("needs: %+v", c.Needs)
	}
	for pkg, want := range map[string]string{
		"tester.kurin": app.NeedBreaks, "tester.chairs": app.NeedLoses, "tester.hair": app.NeedCare,
		"tester.rain": app.NeedBreaks, "tester.menus": app.NeedSafe, "tester.gone": app.NeedUnchecked,
	} {
		if got := by[pkg].Verdict; got != want {
			t.Errorf("%s: %s, want %s (%+v)", pkg, got, want, by[pkg])
		}
	}
	if k := by["tester.kurin"]; k.Colonists != 1 || k.Dead != 1 || !slices.Equal(k.Factions, []string{"Kurin Clan"}) || !slices.Equal(k.Classes, []string{"ModA.CompBroken"}) {
		t.Errorf("kurin: %+v", k)
	}
	if r := by["tester.rain"]; !slices.Equal(r.Weather, []string{"AcidRain"}) {
		t.Errorf("rain: %+v", r)
	}
	if g := by["ludeon.rimworld"]; !g.Game || g.Colonists != 1 {
		t.Errorf("core: %+v", g)
	}
	if g := by["tester.gone"]; g.Why != app.NeedWhyGone {
		t.Errorf("gone: %+v", g)
	}
}
