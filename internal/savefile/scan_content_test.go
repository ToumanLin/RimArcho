package savefile

import (
	"strings"
	"testing"
)

func TestContentReferenceForms(t *testing.T) {
	x := `<savegame><game>
<thing><def>Shared</def><stuff>Steel</stuff><kindDef>Soldier</kindDef>
<genes><xenotype>Baseliner</xenotype><endogenes><li><def>Strong</def></li></endogenes></genes>
<geneSet><genes><li>PackGene</li></genes></geneSet>
<story><childhood>Child</childhood><adulthood>Adult</adulthood><hairDef>Hair</hairDef><bodyType>Body</bodyType><headType>Head</headType><furDef>Fur</furDef></story>
<style><beardDef>Beard</beardDef><faceTattoo>Face</faceTattoo><bodyTattoo>Tattoo</bodyTattoo></style>
<bills><li><recipe>Surgery</recipe></li></bills><styleDef>Spiky_Lamp</styleDef><category>NotAStyle</category></thing>
<ideos><li><thingStyleCategories><li><category>Spiky</category></li></thingStyleCategories></li></ideos>
<researchManager><currentProj>Electricity</currentProj><progress><keys><li>Electricity</li><li>Untouched</li><li>Done</li></keys><values><li>0</li><li>0</li><li>500.5</li></values></progress><techprints><keys><li>NoValue</li></keys></techprints><knowledge><keys><li>Brain</li><li>Void</li></keys><values><li>1</li><li>0</li></values></knowledge></researchManager>
<world><grid><layers><values><li><tileBiome>c9U=</tileBiome></li></values></layers></grid><ideoManager><ideos><li><memes><li>Rancher</li></memes><precepts><li><def>Precept</def><sourcePattern>Festival</sourcePattern></li></precepts></li></ideos></ideoManager></world>
<questManager><quests><li><root>Quest</root><parts><li><incident>QuestRaid</incident><questScript>SubQuest</questScript></li></parts></li></quests></questManager>
<world><storyState><recentRandomQuests><li>RandomQuest</li></recentRandomQuests><recentRandomDecrees><li>Decree</li></recentRandomDecrees><recentRandomIncidents><li>RandomIncident</li></recentRandomIncidents><lastFireTicks><keys><li>PastFlare</li></keys><values><li>5</li></values></lastFireTicks></storyState></world>
<parts><li><incident>NotAQuest</incident><questScript>NotAQuest</questScript></li></parts>
<scenario><playerFaction><factionDef>Player</factionDef></playerFaction></scenario>
<maps><li><weatherManager><curWeather>AcidRain</curWeather><lastWeather>Clear</lastWeather></weatherManager><terrainGrid><topGrid>GxsbGwAAOTA=</topGrid><underGridDeflate>szRgYAAA</underGridDeflate></terrainGrid><compressedThingMap>T+0AAE/t</compressedThingMap></li></maps>
<taleManager><tales><li><surroundings><biome>Desert</biome></surroundings></li></tales></taleManager>
<allowedDefs><li>FilterOnly</li><li><def>NestedFilter</def></li></allowedDefs>
<history><archive><archivables><li><hyperlinkThingDefs><li>HelpOnly</li><li><def>NestedHelp</def></li></hyperlinkThingDefs></li></archivables></archive></history>
<misc><root>ObjectReference</root><li>Arbitrary</li><hairDef>WrongContext</hairDef><biome>GridNotDecoded</biome><curWeather>WrongWeather</curWeather><def>UnknownType</def></misc>
</game></savegame>`
	f, err := Scan(strings.NewReader(x))
	if err != nil || f.Malformed != nil {
		t.Fatalf("%v %+v", err, f.Malformed)
	}
	for typ, ns := range map[string][]string{
		"": {"Shared", "UnknownType"}, "ThingDef": {"Steel"}, "PawnKindDef": {"Soldier"}, "XenotypeDef": {"Baseliner"}, "GeneDef": {"Strong", "PackGene"},
		"BackstoryDef": {"Child", "Adult"}, "HairDef": {"Hair"}, "BodyTypeDef": {"Body"}, "HeadTypeDef": {"Head"}, "FurDef": {"Fur"}, "BeardDef": {"Beard"}, "TattooDef": {"Face", "Tattoo"},
		"ResearchProjectDef": {"Electricity", "Brain", "Done"}, "MemeDef": {"Rancher"}, "PreceptDef": {"Precept"}, "RitualPatternDef": {"Festival"}, "QuestScriptDef": {"Quest"}, "FactionDef": {"Player"}, "BiomeDef": {"Desert"}, "ThingStyleDef": {"Spiky_Lamp"}, "StyleCategoryDef": {"Spiky"}, "WeatherDef": {"AcidRain", "Clear"},
	} {
		for _, n := range ns {
			if f.DefRefs[typ][n] == 0 {
				t.Errorf("missing %s/%s: %v", typ, n, f.DefRefs)
			}
		}
	}
	// grids of Defs by short hash: plain and deflated base64 of little-endian uint16s, 0 none
	if g := f.GridDefs["ThingDef"]; g[60751] != 2 || len(g) != 1 {
		t.Errorf("compressed things: %v", f.GridDefs)
	}
	if g := f.GridDefs["TerrainDef"]; g[6939] != 2 || g[12345] != 2 || len(g) != 2 || len(f.GridDefs["BiomeDef"]) != 1 || f.GridDefs["BiomeDef"][54643] != 1 {
		t.Errorf("grids: %v", f.GridDefs)
	}
	// what the storyteller fired, and what quest parts fire
	for typ, ns := range map[string][]string{"IncidentDef": {"PastFlare", "RandomIncident"}, "QuestScriptDef": {"RandomQuest", "Decree"}} {
		for _, n := range ns {
			if f.DefRefs[typ][n] != 1 {
				t.Errorf("%s/%s: %v", typ, n, f.DefRefs[typ])
			}
		}
	}
	// what only sets something up or may come: bills, what quest parts fire, research
	// with no progress
	for typ, ns := range map[string][]string{"RecipeDef": {"Surgery"}, "IncidentDef": {"QuestRaid"}, "QuestScriptDef": {"SubQuest"}, "ResearchProjectDef": {"Electricity", "Untouched", "Void"}} {
		for _, n := range ns {
			if f.ConfigRefs[typ][n] != 1 {
				t.Errorf("config %s/%s: %v", typ, n, f.ConfigRefs[typ])
			}
		}
		if len(f.ConfigRefs[typ]) != len(ns) {
			t.Errorf("config %s: %v", typ, f.ConfigRefs[typ])
		}
	}
	if f.DefRefs["ResearchProjectDef"]["Electricity"] != 1 || len(f.DefRefs["ResearchProjectDef"]) != 3 {
		t.Errorf("research: the current project, and those with progress: %v", f.DefRefs["ResearchProjectDef"])
	}
	for _, n := range []string{"FilterOnly", "NestedFilter", "HelpOnly", "NestedHelp", "ObjectReference", "Arbitrary", "WrongContext", "NotAQuest", "NotAStyle", "5", "Surgery", "QuestRaid", "SubQuest", "Untouched", "Void", "NoValue", "500.5", "GridNotDecoded", "WrongWeather"} {
		for _, refs := range f.DefRefs {
			if refs[n] != 0 {
				t.Errorf("unexpected %s", n)
			}
		}
	}
	if f.Defs["NestedFilter"] != 1 || f.Defs["NestedHelp"] != 1 {
		t.Fatal("legacy Defs must remain unchanged")
	}
}

func TestContentReferenceEdges(t *testing.T) {
	x := `<savegame><def>TopLevel</def><meta><kindDef>MetaKind</kindDef></meta><game>
<pawn><story><traits><allTraits><li><def>Brave</def></li></allTraits></traits></story><abilities><abilities><li><def>Skip</def></li></abilities></abilities>
<kindDef IsNull="True">NullKind</kindDef><kindDef></kindDef><stuff>null</stuff></pawn>
<gameConditionManager><activeConditions><li><def>Eclipse</def></li></activeConditions></gameConditionManager>
<storyteller><def>Cassandra</def><incidentQueue><queue><li><firingInc><def>RaidEnemy</def></firingInc></li></queue></incidentQueue></storyteller>
<researchManager><progress><keys><li>null</li><li></li><li>Real</li></keys><values><li>1</li><li>1</li><li>1</li><li>1</li></values></progress></researchManager>
</game></savegame>`
	f, err := Scan(strings.NewReader(x))
	if err != nil || f.Malformed != nil {
		t.Fatalf("%v %+v", err, f.Malformed)
	}
	for typ, n := range map[string]string{"TraitDef": "Brave", "AbilityDef": "Skip", "GameConditionDef": "Eclipse", "StorytellerDef": "Cassandra", "IncidentDef": "RaidEnemy", "ResearchProjectDef": "Real"} {
		if f.DefRefs[typ][n] != 1 || len(f.DefRefs[typ]) != 1 {
			t.Errorf("%s/%s: %v", typ, n, f.DefRefs[typ])
		}
	}
	// outside game, IsNull, empty and null values, and research values with no key count nothing
	for _, refs := range []map[string]map[string]int{f.DefRefs, f.ConfigRefs} {
		for typ, ns := range refs {
			for _, n := range []string{"TopLevel", "MetaKind", "NullKind", "", "null"} {
				if ns[n] != 0 {
					t.Errorf("unexpected %s/%q: %v", typ, n, ns)
				}
			}
		}
	}
	if len(f.ConfigRefs) != 0 || len(f.DefRefs["PawnKindDef"]) != 0 || len(f.DefRefs["ThingDef"]) != 0 || len(f.DefRefs[""]) != 0 {
		t.Errorf("refs: %v config: %v", f.DefRefs, f.ConfigRefs)
	}
}
