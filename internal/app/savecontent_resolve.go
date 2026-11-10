package app

import (
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

type contentFields map[string]analyze.DefField
type contentResolution struct {
	fields contentFields
	ok     bool
}
type contentResolver struct {
	defs     map[string][]*contentNode
	parents  map[string][]*contentNode
	memo     map[*contentNode]contentResolution
	visiting map[*contentNode]bool
	active   func(string) bool
}

func (a *App) contentResolver(insts []string, pos map[string]int, packages map[string]bool, names map[string]bool) (*contentResolver, error) {
	r := &contentResolver{defs: map[string][]*contentNode{}, parents: map[string][]*contentNode{}, memo: map[*contentNode]contentResolution{}, visiting: map[*contentNode]bool{}, active: func(p string) bool { return packages[normPkg(p)] }}
	pending := [2]map[string]bool{names, {}}
	seen := [2]map[string]bool{{}, {}}
	for depth := 0; depth < 64; depth++ {
		progress := false
		for kind := range 2 {
			var todo []string
			for name := range pending[kind] {
				if !seen[kind][name] {
					seen[kind][name] = true
					todo = append(todo, name)
				}
			}
			pending[kind] = map[string]bool{}
			if len(todo) == 0 {
				continue
			}
			progress = true
			sort.Strings(todo)
			fs, err := a.Store.ContentDefs(insts, todo, kind == 1)
			if err != nil {
				return nil, err
			}
			for _, f := range fs {
				if !f.Loaded(r.active) {
					continue
				}
				n := &contentNode{fact: f, pos: pos[f.Instance]}
				if kind == 1 {
					r.parents[f.Name] = append(r.parents[f.Name], n)
				} else {
					r.defs[f.DefName] = append(r.defs[f.DefName], n)
				}
				if f.Parent != "" {
					pending[1][f.Parent] = true
				}
				if f.Content != nil {
					for _, field := range f.Content.Fields {
						if field.Name != "race" {
							continue
						}
						if f.Type == "PawnKindDef" && field.Value != "" {
							pending[0][field.Value] = true
						}
						for _, child := range field.Children {
							if child.Name == "fleshType" && child.Value != "" {
								pending[0][child.Value] = true
							}
						}
					}
				}
			}
		}
		if !progress {
			break
		}
	}
	return r, nil
}

func (r *contentResolver) winners(name, typ string) []*contentNode {
	byType := map[string]*contentNode{}
	for _, n := range r.defs[name] {
		f := n.fact
		if f.Abstract || typ != "" && f.Type != typ {
			continue
		}
		if old := byType[f.Type]; old == nil || n.pos >= old.pos {
			byType[f.Type] = n
		}
	}
	var types []string
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	var out []*contentNode
	for _, t := range types {
		out = append(out, byType[t])
	}
	return out
}

type contentTarget struct {
	node      *contentNode
	name      string
	generated bool
}

// targets distinguishes generated ThingDefs from unrelated types sharing a name.
// Buildable wrappers may refer to either a ThingDef or a TerrainDef.
func (r *contentResolver) targets(name, typ string) []contentTarget {
	direct := r.winners(name, typ)
	var out []contentTarget
	base, wrapper := contentWrapper(name)
	if wrapper && (typ == "" || typ == "ThingDef") {
		for _, n := range direct {
			if n.fact.Type != "ThingDef" {
				out = append(out, contentTarget{node: n, name: name})
			}
		}
		for _, n := range r.winners(base, "") {
			if n.fact.Type == "ThingDef" || n.fact.Type == "TerrainDef" && !strings.HasPrefix(name, "Corpse_") {
				out = append(out, contentTarget{node: n, name: base})
			}
		}
		return out
	}
	for _, n := range direct {
		// A pawn kind is part of its race, not content of its own (12 §2.2): the race
		// counts as present, from the Mod defining the race.
		if n.fact.Type == "PawnKindDef" {
			if race := r.kindRace(n); race != nil {
				out = append(out, contentTarget{node: race, name: race.fact.DefName})
				continue
			}
		}
		out = append(out, contentTarget{node: n, name: name})
	}
	if base := impliedBase(name); base != "" && !wrapper {
		_, generatedType := generatedCategory(name, "", "")
		hasGeneratedType := false
		for _, n := range direct {
			hasGeneratedType = hasGeneratedType || n.fact.Type == generatedType
		}
		if !hasGeneratedType && (typ == "" || typ == generatedType) {
			for _, n := range r.winners(base, generatedBaseType(name)) {
				out = append(out, contentTarget{node: n, name: name, generated: true})
			}
		}
	}
	return out
}

// kindRace is the race ThingDef a pawn kind names (inherited through its parents), or nil.
func (r *contentResolver) kindRace(n *contentNode) *contentNode {
	f, ok := r.resolve(n, 0)
	if !ok {
		return nil
	}
	if defs := r.winners(value(f, "race"), "ThingDef"); len(defs) > 0 {
		return defs[0]
	}
	return nil
}

func (r *contentResolver) resolve(n *contentNode, depth int) (contentFields, bool) {
	if v, ok := r.memo[n]; ok {
		return v.fields, v.ok
	}
	if depth >= 64 || r.visiting[n] || n.fact.Content == nil || n.fact.Content.Version != analyze.DefContentVersion || n.fact.Content.Incomplete {
		return nil, false
	}
	r.visiting[n] = true
	defer delete(r.visiting, n)
	fields := contentFields{}
	if name := n.fact.Parent; name != "" {
		var best *contentNode
		for _, p := range r.parents[name] {
			if p.pos <= n.pos && (best == nil || p.pos > best.pos) {
				best = p
			}
		}
		if best == nil {
			r.memo[n] = contentResolution{}
			return nil, false
		}
		pf, ok := r.resolve(best, depth+1)
		if !ok {
			r.memo[n] = contentResolution{}
			return nil, false
		}
		for k, v := range pf {
			fields[k] = v
		}
	}
	fields = r.overlay(fields, n.fact.Content.Fields)
	r.memo[n] = contentResolution{fields, true}
	return fields, true
}

func (r *contentResolver) overlay(base contentFields, children []analyze.DefField) contentFields {
	for _, f := range children {
		if !f.Loaded(r.active) {
			continue
		}
		old := base[f.Name]
		if f.Null {
			delete(base, f.Name)
			continue
		}
		if f.Reset {
			old = analyze.DefField{}
		}
		if f.Name == "verbs" || f.Name == "tools" {
			kids := append([]analyze.DefField{}, old.Children...)
			for _, c := range f.Children {
				if c.Loaded(r.active) {
					kids = append(kids, c)
				}
			}
			f.Children = kids
		} else if len(f.Children) > 0 || len(old.Children) > 0 {
			merged := contentFields{}
			for _, c := range old.Children {
				merged[c.Name] = c
			}
			r.overlay(merged, f.Children)
			f.Children = nil
			var names []string
			for k := range merged {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				f.Children = append(f.Children, merged[k])
			}
		} else if f.Value == "" && !f.Reset {
			f.Value = old.Value
		}
		base[f.Name] = f
	}
	return base
}

func value(f contentFields, k string) string { return f[k].Value }
func truth(f contentFields, k string) bool   { return strings.EqualFold(value(f, k), "true") }
func block(f contentFields, k string) contentFields {
	out := contentFields{}
	for _, c := range f[k].Children {
		out[c.Name] = c
	}
	return out
}
func isMinified(f contentFields) bool {
	c := strings.TrimPrefix(value(f, "thingClass"), "Verse.")
	return c == "MinifiedThing" || c == "MinifiedTree"
}

func (r *contentResolver) category(n *contentNode, f contentFields, ok bool) (key string, inferred, classified bool) {
	if !ok {
		return "other", false, false
	}
	if n.fact.Type == "PawnKindDef" {
		if defs := r.winners(value(f, "race"), "ThingDef"); len(defs) > 0 {
			race, valid := r.resolve(defs[0], 0)
			return r.category(defs[0], race, valid)
		}
		return "other", false, false
	}
	if k := contentTypes[n.fact.Type]; k != "" {
		return k, false, true
	}
	if n.fact.Type != "ThingDef" {
		return "other", false, true
	}
	if _, yes := f["race"]; yes {
		race := block(f, "race")
		intel := value(race, "intelligence")
		flesh := value(race, "fleshType")
		if flesh == "" {
			flesh = "Normal"
		}
		intelligence := 0
		switch intel {
		case "", "Animal":
		case "ToolUser":
			intelligence = 1
		case "Humanlike":
			intelligence = 2
		default:
			var err error
			intelligence, err = strconv.Atoi(intel)
			if err != nil {
				return "other", false, false
			}
		}
		if intelligence >= 2 {
			return "races", false, true
		}
		if flesh == "Mechanoid" {
			return "mechanoids", false, true
		}
		entity := r.active("ludeon.rimworld.anomaly") && (flesh == "EntityMechanical" || flesh == "EntityFlesh" || flesh == "Fleshbeast")
		if intelligence >= 1 || entity {
			return "creatures", false, true
		}
		organic := flesh == "Normal"
		if defs := r.winners(flesh, "FleshTypeDef"); len(defs) > 0 {
			ff, valid := r.resolve(defs[0], 0)
			if !valid {
				return "other", false, false
			}
			// FleshTypeDef.isOrganic defaults true.
			organic = !strings.EqualFold(value(ff, "isOrganic"), "false")
		} else if flesh != "Normal" {
			return "other", false, false
		}
		if organic && !entity {
			return "animals", false, true
		}
		return "creatures", false, true
	}
	if _, yes := f["apparel"]; yes {
		return "apparel", false, true
	}
	class := value(f, "thingClass")
	short := class[strings.LastIndex(class, ".")+1:]
	builtin := class == short || strings.HasPrefix(class, "Verse.") || strings.HasPrefix(class, "RimWorld.")
	if builtin && (short == "Plant" || strings.HasPrefix(short, "Plant_")) {
		return "plants", false, true
	}
	switch value(f, "category") {
	case "Building":
		b := block(f, "building")
		if !truth(b, "isNaturalRock") && !truth(b, "isResourceRock") {
			return "buildings", false, true
		}
	case "Item":
		if len(f["verbs"].Children) > 0 || len(f["tools"].Children) > 0 {
			return "weapons", false, true
		}
		return "items", false, true
	}
	if _, yes := f["plant"]; yes {
		if class != "" && !strings.HasPrefix(class, "Verse.") {
			return "plants", true, true
		}
		return "other", false, false
	}
	return "other", false, true
}

var contentTypes = map[string]string{
	"FactionDef": "factions", "TerrainDef": "terrain", "ResearchProjectDef": "research",
	"IncidentDef": "events", "GameConditionDef": "events", "QuestScriptDef": "quests", "XenotypeDef": "xenotypes", "GeneDef": "xenotypes",
	"MemeDef": "culture", "PreceptDef": "culture", "RitualPatternDef": "culture", "CultureDef": "culture", "StyleCategoryDef": "culture", "ThingStyleDef": "culture",
	"AbilityDef": "abilities", "HediffDef": "hediffs", "TraitDef": "traits", "RecipeDef": "recipes", "HairDef": "appearance", "BeardDef": "appearance", "TattooDef": "appearance",
	"BodyTypeDef": "appearance", "HeadTypeDef": "appearance", "FurDef": "appearance", "BackstoryDef": "appearance", "BiomeDef": "biomes", "StorytellerDef": "storytellers", "ScenarioDef": "scenarios",
}
