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
<bills><li><recipe>Surgery</recipe></li></bills></thing>
<researchManager><currentProj>Electricity</currentProj><progress><keys><li>Electricity</li></keys><values><li>0</li></values></progress><techprints><keys><li>Brain</li></keys></techprints><knowledge><keys><li>Void</li></keys></knowledge></researchManager>
<world><ideoManager><ideos><li><memes><li>Rancher</li></memes><precepts><li><def>Precept</def><sourcePattern>Festival</sourcePattern></li></precepts></li></ideos></ideoManager></world>
<questManager><quests><li><root>Quest</root></li></quests></questManager>
<scenario><playerFaction><factionDef>Player</factionDef></playerFaction></scenario>
<taleManager><tales><li><surroundings><biome>Desert</biome></surroundings></li></tales></taleManager>
<allowedDefs><li>FilterOnly</li><li><def>NestedFilter</def></li></allowedDefs>
<history><archive><archivables><li><hyperlinkThingDefs><li>HelpOnly</li><li><def>NestedHelp</def></li></hyperlinkThingDefs></li></archivables></archive></history>
<misc><root>ObjectReference</root><li>Arbitrary</li><hairDef>WrongContext</hairDef><biome>GridNotDecoded</biome><def>UnknownType</def></misc>
</game></savegame>`
	f, err := Scan(strings.NewReader(x))
	if err != nil || f.Malformed != nil {
		t.Fatalf("%v %+v", err, f.Malformed)
	}
	for typ, ns := range map[string][]string{
		"": {"Shared", "UnknownType"}, "ThingDef": {"Steel"}, "PawnKindDef": {"Soldier"}, "XenotypeDef": {"Baseliner"}, "GeneDef": {"Strong", "PackGene"},
		"BackstoryDef": {"Child", "Adult"}, "HairDef": {"Hair"}, "BodyTypeDef": {"Body"}, "HeadTypeDef": {"Head"}, "FurDef": {"Fur"}, "BeardDef": {"Beard"}, "TattooDef": {"Face", "Tattoo"},
		"RecipeDef": {"Surgery"}, "ResearchProjectDef": {"Electricity", "Brain", "Void"}, "MemeDef": {"Rancher"}, "PreceptDef": {"Precept"}, "RitualPatternDef": {"Festival"}, "QuestScriptDef": {"Quest"}, "FactionDef": {"Player"}, "BiomeDef": {"Desert"},
	} {
		for _, n := range ns {
			if f.DefRefs[typ][n] == 0 {
				t.Errorf("missing %s/%s: %v", typ, n, f.DefRefs)
			}
		}
	}
	if f.DefRefs["ResearchProjectDef"]["Electricity"] != 2 {
		t.Error("current and zero-progress reference must both count")
	}
	for _, n := range []string{"FilterOnly", "NestedFilter", "HelpOnly", "NestedHelp", "ObjectReference", "Arbitrary", "WrongContext", "GridNotDecoded"} {
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
