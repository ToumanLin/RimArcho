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
	var typ string
	config := false // sets something up or names what may come, not what is there
	switch n {
	case "def":
		typ = s.defLeafType()
	case "li":
		var key string
		var ok bool
		if typ, key, config, ok = s.liType(v); !ok {
			return
		}
		if key != "" {
			v = []byte(key)
		}
	default:
		typ, config = s.labelType(n)
	}
	if n != "def" && typ == "" {
		return
	}
	if !s.inGameState() {
		return
	}
	s.countContentRef(typ, config, v)
}

// endsWith reports whether the open elements end with parts, innermost last.
func (s *scanner) endsWith(parts ...string) bool {
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

// under reports whether an element above the current one is named p.
func (s *scanner) under(p string) bool {
	for i := 0; i < s.depth-1; i++ {
		if s.nameStr[s.stack[i].name] == p {
			return true
		}
	}
	return false
}

// defLeafType names the Def type of a <def> leaf by where it sits.
func (s *scanner) defLeafType() string {
	switch {
	case s.endsWith("storyteller", "def"):
		return "StorytellerDef"
	case s.endsWith("allFactions", "li", "def"):
		return "FactionDef"
	case s.endsWith("endogenes", "li", "def"), s.endsWith("xenogenes", "li", "def"):
		return "GeneDef"
	case s.endsWith("allTraits", "li", "def"):
		return "TraitDef"
	case s.endsWith("hediffs", "li", "def"):
		return "HediffDef"
	case s.endsWith("abilities", "li", "def"):
		return "AbilityDef"
	case s.endsWith("precepts", "li", "def"):
		return "PreceptDef"
	case s.endsWith("activeConditions", "li", "def"):
		return "GameConditionDef"
	case s.endsWith("firingInc", "def"):
		return "IncidentDef"
	}
	return ""
}

// liType names the Def type of a list item, pairing research keys with their values: a
// research value stands for its project, key; ok is false when the item counts nothing.
func (s *scanner) liType(v []byte) (typ, key string, config, ok bool) {
	switch {
	case s.endsWith("researchManager", "progress", "keys", "li"), s.endsWith("researchManager", "techprints", "keys", "li"), s.endsWith("researchManager", "knowledge", "keys", "li"):
		s.researchKey(v)
		return "", "", false, false
	case s.endsWith("researchManager", "progress", "values", "li"), s.endsWith("researchManager", "techprints", "values", "li"), s.endsWith("researchManager", "knowledge", "values", "li"):
		key, config, ok = s.researchValue(v)
		return "ResearchProjectDef", key, config, ok
	case s.endsWith("geneSet", "genes", "li"):
		typ = "GeneDef"
	case s.endsWith("ideos", "li", "memes", "li"):
		typ = "MemeDef"
	// What the storyteller has fired, on the world and on each map (StoryState): the
	// tick every incident last fired at, kept for good, and the last five random quests,
	// decrees and incidents. They are the only trace of an event that is over.
	case s.endsWith("storyState", "lastFireTicks", "keys", "li"), s.endsWith("storyState", "recentRandomIncidents", "li"):
		typ = "IncidentDef"
	case s.endsWith("storyState", "recentRandomQuests", "li"), s.endsWith("storyState", "recentRandomDecrees", "li"):
		typ = "QuestScriptDef"
	}
	return typ, "", false, true
}

// researchKey records a research project key, starting over at a new dictionary.
func (s *scanner) researchKey(v []byte) {
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
}

// researchValue returns the project a research value belongs to; no progress is config.
func (s *scanner) researchValue(v []byte) (key string, config, ok bool) {
	i := s.resVals
	s.resVals++
	if i >= len(s.resKeys) || s.resKeys[i] == "" {
		return "", false, false
	}
	n, err := strconv.ParseFloat(string(bytes.TrimSpace(v)), 64)
	return s.resKeys[i], err != nil || n <= 0, true
}

// labelType names the Def type of the other content labels by where they sit.
func (s *scanner) labelType(n string) (typ string, config bool) {
	switch n {
	case "kindDef":
		typ = "PawnKindDef"
	case "stuff":
		typ = "ThingDef"
	case "currentProj":
		if s.endsWith("researchManager", "currentProj") {
			typ = "ResearchProjectDef"
		}
	case "xenotype":
		if s.endsWith("genes", "xenotype") {
			typ = "XenotypeDef"
		}
	case "sourcePattern":
		if s.endsWith("precepts", "li", "sourcePattern") {
			typ = "RitualPatternDef"
		}
	case "root":
		if s.endsWith("questManager", "quests", "li", "root") {
			typ = "QuestScriptDef"
		}
	case "incident":
		// the incident a quest part or a scenario part fires
		if s.endsWith("parts", "li", "incident") && (s.under("questManager") || s.under("scenario")) {
			typ, config = "IncidentDef", true
		}
	case "questScript":
		if s.endsWith("parts", "li", "questScript") && s.under("questManager") {
			typ, config = "QuestScriptDef", true
		}
	case "styleDef":
		// the style a thing was made in
		typ = "ThingStyleDef"
	case "category":
		// the style categories of an ideoligion
		if s.endsWith("thingStyleCategories", "li", "category") {
			typ = "StyleCategoryDef"
		}
	case "factionDef":
		if s.endsWith("scenario", "playerFaction", "factionDef") {
			typ = "FactionDef"
		}
	case "recipe":
		if s.endsWith("bills", "li", "recipe") {
			typ, config = "RecipeDef", true
		}
	case "biome":
		if s.under("taleManager") && s.endsWith("surroundings", "biome") {
			typ = "BiomeDef"
		}
	case "curWeather", "lastWeather":
		// a map's weather: the game cannot draw a map whose weather Def is gone
		if s.endsWith("weatherManager", n) {
			typ = "WeatherDef"
		}
	default:
		if s.endsWith("story", n) {
			typ = storyTypes[n]
		}
		if s.endsWith("style", n) {
			typ = styleTypes[n]
		}
	}
	return typ, config
}

// inGameState reports whether the leaf is game state rather than a filter or help list.
func (s *scanner) inGameState() bool {
	if s.depth < 3 || s.nameStr[s.stack[1].name] != "game" {
		return false
	}
	for i := 2; i < s.depth-1; i++ {
		switch s.nameStr[s.stack[i].name] {
		case "allowedDefs", "disallowedDefs", "hyperlinkThingDefs", "hyperlinkHediffDefs", "thingFilter", "fixedIngredientFilter", "filter":
			return false
		}
	}
	return true
}

// countContentRef counts the named Def v under typ, in ConfigRefs when config.
func (s *scanner) countContentRef(typ string, config bool, v []byte) {
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
