package app

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// SaveContentResult is selected-save L2 evidence, using cached L1 and installed Def
// facts only. It is never computed while listing saves (12 §1.7).
type SaveContentResult struct {
	Read      bool
	Note      string
	Defs      int
	Totals    []SaveContentCategory
	Mods      []SaveContentMod
	Unknown   *SaveContentCategory
	Unchecked []string
}

// ContentLimits describes the supported reference forms, not a claim of play depth.
var ContentLimits = []string{
	"Patch-added Defs and Patch changes to existing Defs are not reflected.",
	"Terrain grids and world biomes are not decoded; only named references are included.",
	"Past events, quests and storytellers may leave no surviving references; indexed history maps are not decoded.",
	"The original ScenarioDef cannot usually be recovered from a save's scenario name and parts.",
}

type SaveContentMod struct {
	Name, PackageID, Library string
	Defs                     int
	Cats                     []SaveContentCategory
}

type SaveContentCategory struct {
	Key                 string
	Count, Uses, Hidden int
	Names               []SaveContentName
	Under               bool
}

type SaveContentName struct {
	Name                           string
	Uses                           int
	Hidden, Inferred, Unclassified bool
}

var ContentCategoryOrder = []string{"factions", "races", "animals", "mechanoids", "creatures", "buildings", "weapons", "apparel", "plants", "items", "terrain", "research", "events", "quests", "xenotypes", "culture", "abilities", "hediffs", "traits", "recipes", "appearance", "biomes", "storytellers", "scenarios", "other"}

func contentUnder(k string) bool {
	switch k {
	case "events", "quests", "scenarios", "storytellers", "biomes", "terrain":
		return true
	}
	return false
}

// SaveContent never opens the save. A missing/currently invalid L1 cache yields Read=false.
func (a *App) SaveContent(sv *Save) (*SaveContentResult, error) {
	oldMM := a.Store.MM
	defer func() { a.Store.MM = oldMM }()
	f, err := a.SaveFacts(sv, false)
	if err != nil {
		return nil, err
	}
	out := &SaveContentResult{Totals: []SaveContentCategory{}, Mods: []SaveContentMod{}}
	if f == nil {
		out.Note = "This save has not been read yet."
		return out, nil
	}
	out.Read = true
	if f.Malformed != nil {
		out.Note = "These references are partial because the save is malformed."
	}
	if sv.Head == nil {
		out.Note = "The save's Mod list could not be read."
		return out, nil
	}
	var mods []SaveMod
	active := map[string]bool{}
	err = a.inSaveList(sv, func() error {
		st, err := a.Loadout()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for i, pkg := range sv.Head.ModIDs {
			active[normPkg(pkg)] = true
			inst := st.Reg.Resolve(pkg).Instance
			if inst != "" && seen[inst] {
				continue
			}
			seen[inst] = true
			name := sv.Head.Name(i)
			if m := st.Ix.ByID[inst]; m != nil {
				name = m.Name
				active[normPkg(m.PackageID)] = true
			}
			mods = append(mods, SaveMod{Name: name, PackageID: pkg, Instance: inst, Library: inst})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Loadout restores the caller's environment, so select the saved game's version
	// explicitly, just as the ownership query does.
	if sv.Env != nil {
		a.Store.MM = env.MajorMinor(sv.Env.GameVersion)
	}
	var insts []string
	pos := map[string]int{}
	for i, m := range mods {
		active[normPkg(m.PackageID)] = true
		if m.Instance != "" {
			insts = append(insts, m.Instance)
			pos[m.Instance] = i
		}
	}
	ready, err := a.Store.ContentReady(insts)
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		// a Mod that is not installed is not "not analysed": its Defs are not found
		if m.Instance != "" && !ready[m.Instance] {
			out.Unchecked = append(out.Unchecked, m.Name)
		}
	}
	names := map[string]bool{}
	for _, refs := range f.DefRefs {
		for name := range refs {
			names[name] = true
		}
	}
	out.Defs = len(names)
	// Base closure is bounded by shrinking generated names. Extra flesh facts supply
	// race predicates; they are not themselves counted as save references.
	lookup := map[string]bool{"Normal": true, "Mechanoid": true}
	for n := range names {
		for depth := 0; n != "" && depth < 32; depth++ {
			lookup[n] = true
			n = impliedBase(n)
		}
	}
	resolver, err := a.contentResolver(insts, pos, active, lookup)
	if err != nil {
		return nil, err
	}
	type entry struct {
		node                      *contentNode
		name, key                 string
		uses                      int
		hidden, inferred, unknown bool
	}
	entries := map[string]*entry{}
	unmatched := map[string]int{}
	for typ, refs := range f.DefRefs {
		for name, uses := range refs {
			// Minified things serialize their contained object separately.
			if name == "MinifiedThing" || name == "MinifiedTree" {
				continue
			}
			targets := resolver.targets(name, typ)
			if len(targets) == 0 {
				unmatched[name] += uses
				continue
			}
			for _, target := range targets {
				node := target.node
				if !ready[node.fact.Instance] {
					continue
				}
				name := target.name
				generated := target.generated
				fields, ok := resolver.resolve(node, 0)
				if isMinified(fields) {
					continue
				}
				key, inferred, classified := resolver.category(node, fields, ok)
				t := node.fact.Type
				if generated {
					key, t = generatedCategory(name, key, t)
				}
				id := t + "/" + name
				e := entries[id]
				if e == nil || node.pos > e.node.pos {
					e = &entry{node: node, name: name, key: key, hidden: key == "factions" && truth(fields, "hidden"), inferred: inferred, unknown: !classified}
					entries[id] = e
				}
				e.uses += uses
			}
		}
	}
	byMod := map[string]map[string][]SaveContentName{}
	for _, e := range entries {
		inst := e.node.fact.Instance
		if byMod[inst] == nil {
			byMod[inst] = map[string][]SaveContentName{}
		}
		byMod[inst][e.key] = append(byMod[inst][e.key], SaveContentName{Name: e.name, Uses: e.uses, Hidden: e.hidden, Inferred: e.inferred, Unclassified: e.unknown})
	}
	totals := map[string]*SaveContentCategory{}
	for _, m := range mods {
		groups := byMod[m.Instance]
		if len(groups) == 0 {
			continue
		}
		v := SaveContentMod{Name: m.Name, PackageID: m.PackageID, Library: m.Library}
		for _, key := range ContentCategoryOrder {
			ns := groups[key]
			if len(ns) == 0 {
				continue
			}
			cat := finishContentCat(key, ns)
			v.Defs += cat.Count
			v.Cats = append(v.Cats, cat)
			t := totals[key]
			if t == nil {
				t = &SaveContentCategory{Key: key, Under: contentUnder(key)}
				totals[key] = t
			}
			t.Count += cat.Count
			t.Uses += cat.Uses
			t.Hidden += cat.Hidden
		}
		out.Mods = append(out.Mods, v)
	}
	for _, key := range ContentCategoryOrder {
		if t := totals[key]; t != nil {
			out.Totals = append(out.Totals, *t)
		}
	}
	if len(unmatched) > 0 {
		var ns []SaveContentName
		for name, uses := range unmatched {
			ns = append(ns, SaveContentName{Name: name, Uses: uses})
		}
		cat := finishContentCat("unknown", ns)
		out.Unknown = &cat
	}
	return out, nil
}

func finishContentCat(key string, ns []SaveContentName) SaveContentCategory {
	c := SaveContentCategory{Key: key, Count: len(ns), Under: contentUnder(key)}
	for _, n := range ns {
		c.Uses += n.Uses
		if n.Hidden {
			c.Hidden++
		}
	}
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].Uses != ns[j].Uses {
			return ns[i].Uses > ns[j].Uses
		}
		return ns[i].Name < ns[j].Name
	})
	c.Names = ns[:min(len(ns), 100)]
	return c
}

func contentWrapper(name string) (string, bool) {
	for _, p := range []string{"Blueprint_Install_", "Blueprint_", "Frame_", "Corpse_"} {
		if strings.HasPrefix(name, p) {
			return name[len(p):], true
		}
	}
	return name, false
}

func generatedCategory(name, key, typ string) (string, string) {
	switch {
	case strings.HasPrefix(name, "Make_"), strings.HasPrefix(name, "Administer_"):
		return "recipes", "RecipeDef"
	case strings.HasSuffix(name, "_Rough"), strings.HasSuffix(name, "_RoughHewn"), strings.HasSuffix(name, "_Smooth"):
		return "terrain", "TerrainDef"
	case strings.HasPrefix(name, "Techprint_"), strings.HasPrefix(name, "Meat_"), strings.HasPrefix(name, "Psytrainer_"), strings.HasPrefix(name, "Neurotrainer_"):
		return "items", "ThingDef"
	}
	return key, typ
}

func generatedBaseType(name string) string {
	switch {
	case strings.HasPrefix(name, "Techprint_"):
		return "ResearchProjectDef"
	case strings.HasPrefix(name, "Psytrainer_"):
		return "AbilityDef"
	case strings.HasPrefix(name, "Neurotrainer_"):
		return "SkillDef"
	default:
		return "ThingDef"
	}
}

// contentNode identity is local to one selected-save query, including inherited parents.
type contentNode struct {
	fact analyze.DefFact
	pos  int
}
