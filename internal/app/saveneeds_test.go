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
	const nl = "\n"
	add := func(id, pkg, name string, files map[string]string, deps ...string) {
		e.AddWorkshop(id, testkit.Mod{PackageID: pkg, Name: name, Files: files, Deps: deps})
	}
	// libraries with nothing in the save: Lib is required by a Mod that breaks the save and
	// requires Base; Menu Lib is required only by a Mod safe to remove
	key := func(n string) map[string]string {
		return map[string]string{"Defs/K.xml": "<Defs><KeyBindingDef><defName>" + n + "</defName></KeyBindingDef></Defs>\n"}
	}
	add("3000000001", "tester.base", "Base", key("BaseKey"))
	add("3000000002", "tester.lib", "Lib", key("LibKey"), "tester.base")
	// Terrain: Floors' is in a map's grid (by its short hash, 6939), Tiles' is not, and the
	// grid has a hash (12345) no Def of the save's Mods has, which may be Tiles' under another
	// name. The save has no biome grid, so nothing of the biome Mod is in it.
	add("3000000011", "tester.floors", "Floors", map[string]string{"Defs/F.xml": "<Defs><TerrainDef><defName>FancyFloor</defName></TerrainDef></Defs>" + nl})
	// Terrain the game generates: smooth stone of Rocks' natural rock (the field is its
	// parent's) and Carpets' template in a structure colour, both in the grid by hash.
	add("3000000013", "tester.rocks", "Rocks", map[string]string{"Defs/R.xml": `<Defs><ThingDef Name="GlimBase" Abstract="True"><building><isNaturalRock>true</isNaturalRock></building></ThingDef><ThingDef ParentName="GlimBase"><defName>Glimstone</defName></ThingDef><ThingDef ParentName="GlimBase"><defName>GlimOre</defName><building><isResourceRock>true</isResourceRock></building></ThingDef></Defs>` + nl})
	// A compressed thing: Ore's resource rock is in a map's compressed things, one place
	// after its own hash (60750), as when a ThingDef not seen here took that one.
	add("3000000015", "tester.ore", "Ore", map[string]string{"Defs/O.xml": `<Defs><ThingDef Name="OreBase" Abstract="True"><building><isNaturalRock>true</isNaturalRock><isResourceRock>true</isResourceRock></building></ThingDef><ThingDef ParentName="OreBase"><defName>MineableGlim</defName><category>Building</category></ThingDef></Defs>` + nl})
	add("3000000014", "tester.carpets", "Carpets", map[string]string{"Defs/C.xml": "<Defs><TerrainTemplateDef><defName>ShagCarpet</defName></TerrainTemplateDef><ColorDef><defName>Structure_Teal</defName></ColorDef></Defs>" + nl})
	add("3000000012", "tester.tiles", "Tiles", map[string]string{"Defs/F.xml": "<Defs><TerrainDef><defName>PlainTile</defName></TerrainDef></Defs>" + nl})
	add("3000000004", "tester.swamp", "Swamp", map[string]string{"Defs/B.xml": "<Defs><BiomeDef><defName>DeepSwamp</defName></BiomeDef></Defs>\n"})
	add("3000000003", "tester.menulib", "Menu Lib", key("MenuLibKey"))
	add("3000000005", "tester.kurin", "Kurin", map[string]string{
		"Defs/Kurin.xml":      "<Defs><ThingDef><defName>Kurin_Race</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>KurinClan</defName></FactionDef></Defs>\n",
		"Assemblies/ModA.dll": string(dll),
	}, "tester.lib")
	add("3000000006", "tester.chairs", "Chairs", map[string]string{"Defs/C.xml": "<Defs><ThingDef><defName>Stool</defName><category>Building</category></ThingDef></Defs>\n"})
	add("3000000007", "tester.hair", "Hair", map[string]string{"Defs/H.xml": "<Defs><HairDef><defName>Bob</defName></HairDef></Defs>\n"})
	add("3000000008", "tester.rain", "Rain", map[string]string{"Defs/W.xml": "<Defs><WeatherDef><defName>AcidRain</defName></WeatherDef></Defs>\n"})
	// a research project of the Mod's own Def class, which the save references as a
	// ResearchProjectDef; Menus has a Def of that name and an unrelated game type
	add("3000000010", "tester.tribe", "Tribe", map[string]string{"Defs/T.xml": "<Defs><TesterTribe.TribeResearchDef><defName>TribeFire</defName></TesterTribe.TribeResearchDef></Defs>" + nl})
	menus := key("Unused")
	menus["Defs/H.xml"] = "<Defs><HeadTypeDef><defName>TribeFire</defName></HeadTypeDef></Defs>" + nl
	add("3000000009", "tester.menus", "Menus", menus, "tester.menulib", "ludeon.rimworld")
	mods := []string{"ludeon.rimworld", "tester.base", "tester.lib", "tester.menulib", "tester.swamp", "tester.floors", "tester.tiles", "tester.rocks", "tester.carpets", "tester.ore", "tester.kurin", "tester.chairs", "tester.hair", "tester.rain", "tester.menus", "tester.tribe"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Hour))

	body := `<maps><li><uniqueID>0</uniqueID><weatherManager><curWeather>AcidRain</curWeather><lastWeather>Clear</lastWeather></weatherManager><terrainGrid><topGrid>GxsbGwAAOTA9iJDF</topGrid></terrainGrid><compressedThingMap>T+0AAE/t</compressedThingMap><things>
<thing Class="Pawn"><def>Kurin_Race</def><id>Kurin_Race1</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story><hairDef>Bob</hairDef></story></thing>
<thing Class="ThingWithComps"><def>Corpse_Kurin_Race</def><id>Corpse_Kurin_Race3</id><innerContainer><innerList><li>
 <def>Kurin_Race</def><id>Kurin_Race4</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /><healthTracker><healthState>Dead</healthState></healthTracker></li></innerList></innerContainer></thing>
<thing Class="Pawn"><def>Human</def><id>Human5</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="Building"><def>Stool</def><id>Stool6</id></thing>
</things></li></maps>
<components><li Class="ModA.CompBroken" /></components>
<researchManager><progress><keys><li>TribeFire</li></keys><values><li>10</li></values></progress></researchManager>`
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
	if len(c.Needs) != 17 {
		t.Fatalf("needs: %+v", c.Needs)
	}
	for pkg, want := range map[string]string{
		"tester.kurin": app.NeedBreaks, "tester.chairs": app.NeedLoses, "tester.hair": app.NeedCare,
		"tester.rain": app.NeedBreaks, "tester.menus": app.NeedSafe, "tester.gone": app.NeedUnchecked,
		"tester.lib": app.NeedBreaks, "tester.base": app.NeedBreaks, "tester.menulib": app.NeedCare, "tester.swamp": app.NeedSafe, "tester.floors": app.NeedLoses, "tester.tiles": app.NeedUnchecked,
		"tester.rocks": app.NeedLoses, "tester.carpets": app.NeedLoses, "tester.ore": app.NeedLoses,
		"tester.tribe": app.NeedCare,
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
	if l, b := by["tester.lib"], by["tester.base"]; !slices.Equal(l.NeededBy, []string{"Kurin"}) || !slices.Equal(b.NeededBy, []string{"Lib"}) {
		t.Errorf("needed by: %+v %+v", l, b)
	}
	if g := by["ludeon.rimworld"]; len(g.NeededBy) != 0 {
		t.Errorf("core: %+v", g)
	}
	if g := by["tester.tiles"]; g.Why != app.NeedWhyGrid || c.GridUnnamed["TerrainDef"] != 1 {
		t.Errorf("tiles: %+v %v", g, c.GridUnnamed)
	}
	if g := by["tester.gone"]; g.Why != app.NeedWhyGone {
		t.Errorf("gone: %+v", g)
	}
}

// A hash in the grids that no Def has makes only the copies it may belong to "could not
// check" (12 §4.7): those with terrain of their own for an unnamed terrain hash, and for an
// unnamed compressed thing those with a compressible Def whose own hash is a few places
// before it. A Mod whose only link to terrain is its natural rock's stone floors, and one
// whose rock hashes far from the unnamed thing, are safe. A starting pawn the player did
// not choose is a world pawn of its race's Mod, not a colonist.
func TestSaveNeedsUnnamedGridOwners(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Races.xml"),
		"<Defs><ThingDef><defName>Human</defName><race><intelligence>Humanlike</intelligence></race></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef></Defs>\n")
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", store.GameMainAssembly))
	rock := func(name string) map[string]string {
		return map[string]string{"Defs/R.xml": "<Defs><ThingDef><defName>" + name + "</defName><building><isNaturalRock>true</isNaturalRock></building></ThingDef></Defs>\n"}
	}
	// hashes: NearRock 2159 (the save has 2164, five places on), FarRock 13096, Duskstone
	// 38979; the terrain grid's 12345 is no Def's
	e.AddWorkshop("3000000021", testkit.Mod{PackageID: "tester.near", Name: "Near", Files: rock("NearRock")})
	e.AddWorkshop("3000000022", testkit.Mod{PackageID: "tester.far", Name: "Far", Files: rock("FarRock")})
	e.AddWorkshop("3000000023", testkit.Mod{PackageID: "tester.dusk", Name: "Dusk", Files: rock("Duskstone")})
	e.AddWorkshop("3000000024", testkit.Mod{PackageID: "tester.tiles", Name: "Tiles", Files: map[string]string{"Defs/F.xml": "<Defs><TerrainDef><defName>PlainTile</defName></TerrainDef></Defs>\n"}})
	e.AddWorkshop("3000000025", testkit.Mod{PackageID: "tester.kurin", Name: "Kurin", Files: map[string]string{"Defs/K.xml": "<Defs><ThingDef><defName>Kurin_Race</defName><race><intelligence>Humanlike</intelligence></race></ThingDef></Defs>\n"}})
	mods := []string{"ludeon.rimworld", "tester.near", "tester.far", "tester.dusk", "tester.tiles", "tester.kurin"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	body := `<maps><li><uniqueID>0</uniqueID><terrainGrid><topGrid>OTA5MA==</topGrid></terrainGrid><compressedThingMap>dAh0CA==</compressedThingMap><things>
<thing Class="Pawn"><def>Human</def><id>Human5</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
</things></li></maps>
<world><worldPawns><pawnsAlive><li><def>Kurin_Race</def><id>Kurin_Race7</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></li></pawnsAlive></worldPawns></world>`
	writeOwnersSave(t, e, "Grids.rws", testkit.SaveSpec{Mods: mods, Faction: "Colony", Body: body})
	s, _, err := e.App.FindSave("Grids")
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
	if c.GridUnnamed["TerrainDef"] != 1 || c.GridUnnamed["ThingDef"] != 1 {
		t.Fatalf("unnamed: %v", c.GridUnnamed)
	}
	for pkg, want := range map[string]string{
		"tester.near": app.NeedUnchecked, "tester.tiles": app.NeedUnchecked,
		"tester.far": app.NeedSafe, "tester.dusk": app.NeedSafe,
	} {
		if got := by[pkg]; got.Verdict != want {
			t.Errorf("%s: %s, want %s (%+v)", pkg, got.Verdict, want, got)
		}
	}
	if k := by["tester.kurin"]; k.World != 1 || k.Colonists != 0 || k.Verdict != app.NeedBreaks {
		t.Errorf("kurin: %+v", k)
	}
}
