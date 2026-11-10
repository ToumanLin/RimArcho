package savefile

import "bytes"

// contentRef keeps object/state and surviving history evidence, separately from Defs
// (12 §1.4). Match suffixes without constructing paths for every leaf in a large save.
func (s *scanner) contentRef(f *frame, v []byte) {
	n := s.nameStr[f.name]
	if n != "def" && !contentLabels[n] {
		return
	}
	if f.flags&fNull != 0 {
		return
	}
	path := func(parts ...string) bool {
		if len(parts) > s.depth {
			return false
		}
		for i, p := range parts {
			if s.nameStr[s.stack[s.depth-len(parts)+i].name] != p {
				return false
			}
		}
		return true
	}
	under := func(p string) bool {
		for i := 0; i < s.depth-1; i++ {
			if s.nameStr[s.stack[i].name] == p {
				return true
			}
		}
		return false
	}
	typ := ""
	switch n {
	case "def":
		switch {
		case path("storyteller", "def"):
			typ = "StorytellerDef"
		case path("allFactions", "li", "def"):
			typ = "FactionDef"
		case path("endogenes", "li", "def"), path("xenogenes", "li", "def"):
			typ = "GeneDef"
		case path("allTraits", "li", "def"):
			typ = "TraitDef"
		case path("hediffs", "li", "def"):
			typ = "HediffDef"
		case path("abilities", "li", "def"):
			typ = "AbilityDef"
		case path("precepts", "li", "def"):
			typ = "PreceptDef"
		case path("activeConditions", "li", "def"):
			typ = "GameConditionDef"
		case path("firingInc", "def"):
			typ = "IncidentDef"
		}
	case "kindDef":
		typ = "PawnKindDef"
	case "stuff":
		typ = "ThingDef"
	case "currentProj":
		if path("researchManager", "currentProj") {
			typ = "ResearchProjectDef"
		}
	case "li":
		switch {
		case path("researchManager", "progress", "keys", "li"), path("researchManager", "techprints", "keys", "li"), path("researchManager", "knowledge", "keys", "li"):
			typ = "ResearchProjectDef"
		case path("geneSet", "genes", "li"):
			typ = "GeneDef"
		case path("ideos", "li", "memes", "li"):
			typ = "MemeDef"
		}
	case "xenotype":
		if path("genes", "xenotype") {
			typ = "XenotypeDef"
		}
	case "sourcePattern":
		if path("precepts", "li", "sourcePattern") {
			typ = "RitualPatternDef"
		}
	case "root":
		if path("questManager", "quests", "li", "root") {
			typ = "QuestScriptDef"
		}
	case "factionDef":
		if path("scenario", "playerFaction", "factionDef") {
			typ = "FactionDef"
		}
	case "recipe":
		if path("bills", "li", "recipe") {
			typ = "RecipeDef"
		}
	case "biome":
		if under("taleManager") && path("surroundings", "biome") {
			typ = "BiomeDef"
		}
	default:
		if path("story", n) {
			typ = storyTypes[n]
		}
		if path("style", n) {
			typ = styleTypes[n]
		}
	}
	if n != "def" && typ == "" {
		return
	}
	if s.depth < 3 || s.nameStr[s.stack[1].name] != "game" {
		return
	}
	for i := 2; i < s.depth-1; i++ {
		switch s.nameStr[s.stack[i].name] {
		case "allowedDefs", "disallowedDefs", "hyperlinkThingDefs", "hyperlinkHediffDefs", "thingFilter", "fixedIngredientFilter", "filter":
			return
		}
	}
	v = bytes.TrimSpace(v)
	if len(v) == 0 || string(v) == "null" {
		return
	}
	name := s.x.str(v)
	if s.f.DefRefs == nil {
		s.f.DefRefs = map[string]map[string]int{}
	}
	if s.f.DefRefs[typ] == nil {
		s.f.DefRefs[typ] = map[string]int{}
	}
	s.f.DefRefs[typ][name]++
}

var storyTypes = map[string]string{"childhood": "BackstoryDef", "adulthood": "BackstoryDef", "hairDef": "HairDef", "bodyType": "BodyTypeDef", "headType": "HeadTypeDef", "furDef": "FurDef"}
var styleTypes = map[string]string{"beardDef": "BeardDef", "faceTattoo": "TattooDef", "bodyTattoo": "TattooDef"}
var contentLabels = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{"kindDef", "stuff", "currentProj", "li", "xenotype", "sourcePattern", "root", "factionDef", "recipe", "biome"} {
		m[n] = true
	}
	for n := range storyTypes {
		m[n] = true
	}
	for n := range styleTypes {
		m[n] = true
	}
	return m
}()
