package savefile

import (
	"bytes"
	"strconv"
)

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
	config := false // sets something up or names what may come, not what is there
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
			// A project counts by its value, which follows: the game may list projects
			// nobody has touched, with 0.
			if d := s.stack[s.depth-3].name; s.resVals > 0 || d != s.resDict {
				s.resKeys, s.resVals, s.resDict = s.resKeys[:0], 0, d
			}
			if v = bytes.TrimSpace(v); len(v) > 0 && string(v) != "null" {
				s.resKeys = append(s.resKeys, s.x.str(v))
			} else {
				s.resKeys = append(s.resKeys, "")
			}
			return
		case path("researchManager", "progress", "values", "li"), path("researchManager", "techprints", "values", "li"), path("researchManager", "knowledge", "values", "li"):
			i := s.resVals
			s.resVals++
			if i >= len(s.resKeys) || s.resKeys[i] == "" {
				return
			}
			n, err := strconv.ParseFloat(string(bytes.TrimSpace(v)), 64)
			typ, v, config = "ResearchProjectDef", []byte(s.resKeys[i]), err != nil || n <= 0
		case path("geneSet", "genes", "li"):
			typ = "GeneDef"
		case path("ideos", "li", "memes", "li"):
			typ = "MemeDef"
		// What the storyteller has fired, on the world and on each map (StoryState): the
		// tick every incident last fired at, kept for good, and the last five random quests,
		// decrees and incidents. They are the only trace of an event that is over.
		case path("storyState", "lastFireTicks", "keys", "li"), path("storyState", "recentRandomIncidents", "li"):
			typ = "IncidentDef"
		case path("storyState", "recentRandomQuests", "li"), path("storyState", "recentRandomDecrees", "li"):
			typ = "QuestScriptDef"
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
	case "incident":
		// the incident a quest part or a scenario part fires
		if path("parts", "li", "incident") && (under("questManager") || under("scenario")) {
			typ, config = "IncidentDef", true
		}
	case "questScript":
		if path("parts", "li", "questScript") && under("questManager") {
			typ, config = "QuestScriptDef", true
		}
	case "styleDef":
		// the style a thing was made in
		typ = "ThingStyleDef"
	case "category":
		// the style categories of an ideoligion
		if path("thingStyleCategories", "li", "category") {
			typ = "StyleCategoryDef"
		}
	case "factionDef":
		if path("scenario", "playerFaction", "factionDef") {
			typ = "FactionDef"
		}
	case "recipe":
		if path("bills", "li", "recipe") {
			typ, config = "RecipeDef", true
		}
	case "biome":
		if under("taleManager") && path("surroundings", "biome") {
			typ = "BiomeDef"
		}
	case "curWeather", "lastWeather":
		// a map's weather: the game cannot draw a map whose weather Def is gone
		if path("weatherManager", n) {
			typ = "WeatherDef"
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
	refs := &s.f.DefRefs
	if config {
		refs = &s.f.ConfigRefs
	}
	if *refs == nil {
		*refs = map[string]map[string]int{}
	}
	if (*refs)[typ] == nil {
		(*refs)[typ] = map[string]int{}
	}
	(*refs)[typ][name]++
}

var storyTypes = map[string]string{"childhood": "BackstoryDef", "adulthood": "BackstoryDef", "hairDef": "HairDef", "bodyType": "BodyTypeDef", "headType": "HeadTypeDef", "furDef": "FurDef"}
var styleTypes = map[string]string{"beardDef": "BeardDef", "faceTattoo": "TattooDef", "bodyTattoo": "TattooDef"}
var contentLabels = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{"kindDef", "stuff", "currentProj", "li", "xenotype", "sourcePattern", "root", "factionDef", "recipe", "biome", "curWeather", "lastWeather", "incident", "questScript", "styleDef", "category"} {
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
