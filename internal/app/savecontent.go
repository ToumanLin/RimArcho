package app

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SaveContentResult is selected-save L2 evidence, using cached L1 and installed Def
// facts only. It is never computed while listing saves (12 §1.7).
type SaveContentResult struct {
	Read bool
	// Stale: the cached read is from an older parser without the typed references
	// (savefile.MinParserVersion); the caller reads the save again (SaveFacts with read).
	Stale     bool
	Note      string
	Defs      int
	Totals    []SaveContentCategory
	Mods      []SaveContentMod
	Unknown   *SaveContentCategory
	Unchecked []string
	// Needs is every Mod of the save, in the save's order, with what removing it costs
	// (12 §4.7).
	Needs []SaveNeed
	// GridUnnamed is, by Def type, how many distinct terrain, biome or compressed-thing hashes in the save's
	// grids no Def of the save's Mods has (12 §1.4).
	GridUnnamed map[string]int

	// For the playthrough review (savereview.go): the save's Mods, and by instance and
	// category every name attributed to it, uncut.
	mods  []SaveMod
	names map[string]map[string][]SaveContentName
	// configNames are the Def names the save only sets up (ConfigRefs), when seen is asked.
	configNames map[string]bool
}

// contentOpts are what the playthrough review asks of the attribution (12 §3).
type contentOpts struct {
	// facts supplies this save's just-read facts without a second cache decode.
	facts *savefile.Facts
	// seen: count only what is there (DefRefs and the grids), not what is only set up
	// (ConfigRefs).
	seen bool
	// src, when not nil, keeps the Defs in memory between saves.
	src *defSource
}

// ContentLimits describes the supported reference forms, not a claim of play depth.
var ContentLimits = []string{
	"Patch-added Defs and Patch changes to existing Defs are not reflected.",
	"Terrain and biomes are read from the maps' and the world's grids by Def hash; so are rock, ore and chunks, less surely.",
	"Past events are read from the storyteller's record of what it fired; finished quests and past storytellers may leave nothing.",
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
	case "quests", "scenarios", "storytellers":
		return true
	}
	return false
}

// SaveContent never opens the save. A missing/currently invalid L1 cache yields Read=false.
func (a *App) SaveContent(sv *Save) (*SaveContentResult, error) {
	return a.saveContent(sv, contentOpts{})
}

func (a *App) saveContent(sv *Save, o contentOpts) (*SaveContentResult, error) {
	oldMM := a.Store.MM
	defer func() { a.Store.MM = oldMM }()
	f := o.facts
	var err error
	if f == nil {
		f, err = a.SaveFacts(sv, false)
	}
	if err != nil {
		return nil, err
	}
	out := &SaveContentResult{Totals: []SaveContentCategory{}, Mods: []SaveContentMod{}}
	if f == nil && sv.Head == nil { // its start could not be read: a full read cannot either
		out.Note = "The save's Mod list could not be read."
		return out, nil
	}
	if f == nil {
		out.Note = "This save has not been read yet."
		return out, nil
	}
	if f.Parser != savefile.ParserVersion {
		out.Stale, out.Note = true, "This save is read again for its content."
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
	cm, err := a.saveContentMods(sv)
	if err != nil {
		return nil, err
	}
	mods, active := cm.mods, cm.active
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
	// The Defs the grids hold by hash count as typed references (12 §1.4).
	activePkg := func(p string) bool { return active[normPkg(p)] }
	grids, err := a.gridRefs(f, insts, activePkg)
	if err != nil {
		return nil, err
	}
	defRefs := mergeDefRefs(f, grids.refs, o, out)
	out.GridUnnamed = grids.left
	names := map[string]bool{}
	for _, refs := range defRefs {
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
	resolver, err := a.contentResolver(o.src, insts, pos, active, lookup)
	if err != nil {
		return nil, err
	}
	entries, unmatched := resolveContentEntries(resolver, defRefs, ready, grids.refs)
	byMod := contentByMod(entries)
	own := contentOwners(entries)
	out.mods, out.names = mods, byMod
	catsOf := out.addCategories(mods, byMod)
	if len(unmatched) > 0 {
		var ns []SaveContentName
		for name, uses := range unmatched {
			ns = append(ns, SaveContentName{Name: name, Uses: uses})
		}
		cat := finishContentCat("unknown", ns)
		out.Unknown = &cat
	}
	if out.Needs, err = a.saveNeeds(f, mods, ready, catsOf, own, cm.deps, grids); err != nil {
		return nil, err
	}
	return out, nil
}

// contentMods are the Mods a save's content is attributed to.
type contentMods struct {
	mods   []SaveMod
	active map[string]bool      // the packageIds that load
	deps   map[string][]scanDep // by instance: what its About.xml requires (12 §4.7)
}

// saveContentMods lists the copies the save's Mods load as, once each, in the save's list.
// It leaves a.Store.MM at the saved game's version.
func (a *App) saveContentMods(sv *Save) (*contentMods, error) {
	cm := &contentMods{active: map[string]bool{}, deps: map[string][]scanDep{}}
	// A pack's save loads the pack's own copies; the library's copy of the same packageId
	// stands in while the pack's has no content facts.
	packCopy := map[string]string{}
	if sv.Pack != "" {
		if p, err := a.ReadPack(sv.Pack); err == nil {
			for _, e := range p.Entries {
				if e.Instance != "" {
					packCopy[normPkg(e.PackageID)] = e.Instance
				}
			}
		}
	}
	if sv.Env != nil {
		a.Store.MM = env.MajorMinor(sv.Env.GameVersion)
	}
	var packInsts []string
	for _, inst := range packCopy {
		packInsts = append(packInsts, inst)
	}
	packReady, err := a.Store.ContentReady(packInsts)
	if err != nil {
		return nil, err
	}
	if err := a.inSaveList(sv, func() error { return a.listContentMods(sv, cm, packCopy, packReady) }); err != nil {
		return nil, err
	}
	return cm, nil
}

// listContentMods fills cm from the save's Mod list as the current loadout resolves it, a
// pack's copy preferred while it has content facts.
func (a *App) listContentMods(sv *Save, cm *contentMods, packCopy map[string]string, packReady map[string]bool) error {
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, pkg := range sv.Head.ModIDs {
		cm.active[normPkg(pkg)] = true
		inst := st.Reg.Resolve(pkg).Instance
		if pc := packCopy[normPkg(pkg)]; pc != "" && (packReady[pc] || inst == "") {
			inst = pc
		}
		if inst != "" && seen[inst] {
			continue
		}
		seen[inst] = true
		name := sv.Head.Name(i)
		if m := st.Ix.ByID[inst]; m != nil {
			name = m.DisplayName()
			cm.active[normPkg(m.PackageID)] = true
		}
		cm.mods = append(cm.mods, SaveMod{Name: name, PackageID: pkg, Instance: inst, Library: inst})
	}
	var installed []*store.Mod
	for _, m := range cm.mods {
		if im := st.Ix.ByID[m.Instance]; im != nil {
			installed = append(installed, im)
		}
	}
	if err := a.Store.LoadDecls(installed); err != nil {
		return err
	}
	for _, im := range installed {
		cm.deps[im.ID] = requiredDeps(im, st.GameMM())
	}
	return nil
}

// mergeDefRefs adds to the save's typed references the grids' (grid) and those that only
// set something up (bills, quest parts, research not begun): the save names those Defs too,
// and loading without them drops what named them. With o.seen the set-up references are
// kept apart in out.configNames instead.
func mergeDefRefs(f *savefile.Facts, grid map[string]map[string]int, o contentOpts, out *SaveContentResult) map[string]map[string]int {
	defRefs := f.DefRefs
	config := f.ConfigRefs
	if o.seen {
		out.configNames = map[string]bool{}
		for _, refs := range config {
			for name := range refs {
				out.configNames[name] = true
			}
		}
		config = nil
	}
	if len(grid)+len(config) > 0 {
		defRefs = map[string]map[string]int{}
		for _, more := range []map[string]map[string]int{f.DefRefs, config, grid} {
			for typ, refs := range more {
				if defRefs[typ] == nil {
					defRefs[typ] = map[string]int{}
				}
				for name, k := range refs {
					defRefs[typ][name] += k
				}
			}
		}
	}
	return defRefs
}

// contentEntry is one Def the save references, as the Def a loaded copy defines.
type contentEntry struct {
	node                      *contentNode
	name, key                 string
	uses                      int
	hidden, inferred, unknown bool
	weather                   bool
	grid                      bool // in a map's or the world's grid
}

// resolveContentEntries resolves the save's references to the Defs of analysed copies (ready),
// by type and name; unmatched are the names no Def explains, with their uses.
func resolveContentEntries(resolver *contentResolver, defRefs map[string]map[string]int, ready map[string]bool, grid map[string]map[string]int) (map[string]*contentEntry, map[string]int) {
	entries := map[string]*contentEntry{}
	unmatched := map[string]int{}
	for typ, refs := range defRefs {
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
					e = &contentEntry{node: node, name: name, key: key, hidden: key == "factions" && truth(fields, "hidden"), inferred: inferred, unknown: !classified}
					entries[id] = e
				}
				e.uses += uses
				e.weather = e.weather || typ == "WeatherDef"
				e.grid = e.grid || grid[typ][name] > 0 || grid[typ][target.name] > 0
			}
		}
	}
	return entries, unmatched
}

// contentByMod groups the entries by copy and category: one name per category of a Mod.
// Defs of several types sharing a name (a skill and a work type both called Mining) are
// listed once, with the most uses.
func contentByMod(entries map[string]*contentEntry) map[string]map[string][]SaveContentName {
	byMod := map[string]map[string][]SaveContentName{}
	at := map[string]int{}
	for _, e := range entries {
		inst := e.node.fact.Instance
		if byMod[inst] == nil {
			byMod[inst] = map[string][]SaveContentName{}
		}
		id := inst + "|" + e.key + "|" + e.name
		if i, ok := at[id]; ok {
			n := &byMod[inst][e.key][i]
			n.Uses = max(n.Uses, e.uses)
			n.Hidden = n.Hidden || e.hidden
			n.Inferred = n.Inferred && e.inferred
			n.Unclassified = n.Unclassified && e.unknown
			continue
		}
		at[id] = len(byMod[inst][e.key])
		byMod[inst][e.key] = append(byMod[inst][e.key], SaveContentName{Name: e.name, Uses: e.uses, Hidden: e.hidden, Inferred: e.inferred, Unclassified: e.unknown})
	}
	return byMod
}

// contentOwners names the copies that own the save's things, factions, weather and grids.
func contentOwners(entries map[string]*contentEntry) needOwners {
	own := needOwners{things: map[string]string{}, factions: map[string]string{}, weather: map[string][]string{}, grid: map[string]bool{}}
	for _, e := range entries {
		inst := e.node.fact.Instance
		if e.node.fact.Type == "ThingDef" {
			own.things[e.name] = inst
		}
		if e.key == "factions" {
			own.factions[e.name] = inst
		}
		if e.grid {
			own.grid[inst] = true
		}
		if e.weather {
			own.weather[inst] = append(own.weather[inst], e.name)
		}
	}
	return own
}

// addCategories adds each Mod's categories and the totals in category order, and returns
// the categories by copy.
func (out *SaveContentResult) addCategories(mods []SaveMod, byMod map[string]map[string][]SaveContentName) map[string][]SaveContentCategory {
	catsOf := map[string][]SaveContentCategory{}
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
		catsOf[m.Instance] = v.Cats
	}
	for _, key := range ContentCategoryOrder {
		if t := totals[key]; t != nil {
			out.Totals = append(out.Totals, *t)
		}
	}
	return catsOf
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
	case strings.HasPrefix(name, "Aptitude"), strings.HasPrefix(name, "ChemicalDependency_"), strings.HasPrefix(name, "Addiction"):
		return "xenotypes", "GeneDef"
	}
	return key, typ
}

func generatedBaseType(name string) string {
	switch {
	case strings.HasPrefix(name, "Techprint_"):
		return "ResearchProjectDef"
	case strings.HasPrefix(name, "Psytrainer_"):
		return "AbilityDef"
	case strings.HasPrefix(name, "Neurotrainer_"), strings.HasPrefix(name, "Aptitude"):
		return "SkillDef"
	case strings.HasPrefix(name, "ChemicalDependency_"), strings.HasPrefix(name, "Addiction"):
		return "ChemicalDef"
	default:
		return "ThingDef"
	}
}

// contentNode identity is local to one selected-save query, including inherited parents.
type contentNode struct {
	fact analyze.DefFact
	pos  int
}
