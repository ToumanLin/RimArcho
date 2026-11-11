package view

import (
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

type SaveContent struct {
	Read      bool             `json:"read"`
	Stale     bool             `json:"stale,omitempty"` // read again for its content; a saves-read event follows
	Note      string           `json:"note,omitempty"`
	Defs      int              `json:"defs"`
	Totals    []SaveContentCat `json:"totals"`
	Mods      []SaveContentMod `json:"mods"`
	Unknown   *SaveContentCat  `json:"unknown,omitempty"`
	Unchecked []string         `json:"unchecked,omitempty"`
	Limits    []string         `json:"limits,omitempty"`
	Needs     *SaveNeeds       `json:"needs,omitempty"`
}

type SaveContentMod struct {
	Name      string           `json:"name"`
	PackageID string           `json:"packageId"`
	Library   string           `json:"library,omitempty"`
	Defs      int              `json:"defs"`
	Cats      []SaveContentCat `json:"cats"`
}

type SaveContentCat struct {
	Key    string   `json:"key"`
	Label  string   `json:"label"`
	Count  int      `json:"count"`
	Uses   int      `json:"uses"`
	Names  []string `json:"names,omitempty"`
	Under  bool     `json:"under,omitempty"`
	Hidden int      `json:"hidden,omitempty"`
}

// SaveContentOf translates a selected-save content result without additional queries.
func SaveContentOf(c *app.SaveContentResult) *SaveContent {
	v := &SaveContent{Read: c.Read, Stale: c.Stale, Note: T(c.Note), Defs: c.Defs, Totals: []SaveContentCat{}, Mods: []SaveContentMod{}, Unchecked: c.Unchecked}
	if !c.Read {
		return v
	}
	for _, text := range app.ContentLimits {
		v.Limits = append(v.Limits, T(text))
	}
	for _, cat := range c.Totals {
		x := saveContentCat(cat)
		x.Names = nil
		v.Totals = append(v.Totals, x)
	}
	for _, m := range c.Mods {
		x := SaveContentMod{Name: m.Name, PackageID: m.PackageID, Library: m.Library, Defs: m.Defs, Cats: []SaveContentCat{}}
		for _, cat := range m.Cats {
			x.Cats = append(x.Cats, saveContentCat(cat))
		}
		v.Mods = append(v.Mods, x)
	}
	if c.Unknown != nil {
		x := saveContentCat(*c.Unknown)
		v.Unknown = &x
	}
	v.Needs = saveNeedsOf(c.Needs)
	return v
}

func saveContentCat(c app.SaveContentCategory) SaveContentCat {
	v := SaveContentCat{Key: c.Key, Label: contentCategoryLabelN(c.Key, c.Count), Count: c.Count, Uses: c.Uses, Under: c.Under, Hidden: c.Hidden}
	for _, n := range c.Names {
		name := n.Name
		if n.Hidden {
			name = Tf("%s (hidden)", name)
		}
		if n.Inferred {
			name = Tf("%s (inferred)", name)
		}
		if n.Unclassified {
			name = Tf("%s (classification unknown)", name)
		}
		v.Names = append(v.Names, name)
	}
	return v
}

// contentCategoryOne is a category's label for a count of one ("1 faction", not "1 factions");
// categories whose name has no plural are left out.
var contentCategoryOne = map[string]string{
	"factions": "faction", "races": "humanlike race", "animals": "animal", "mechanoids": "mechanoid",
	"creatures": "other creature", "buildings": "building", "weapons": "weapon", "plants": "plant",
	"items": "item", "research": "research project", "events": "event", "quests": "quest",
	"xenotypes": "xenotype or gene", "abilities": "ability", "hediffs": "health condition", "traits": "trait",
	"recipes": "recipe", "biomes": "biome", "storytellers": "storyteller", "scenarios": "scenario",
}

// contentCategoryLabelN is the label shown after a count.
func contentCategoryLabelN(key string, n int) string {
	if one, ok := contentCategoryOne[key]; ok && n == 1 {
		return T(one)
	}
	return contentCategoryLabel(key)
}

func contentCategoryLabel(key string) string {
	switch key {
	case "factions":
		return T("factions")
	case "races":
		return T("humanlike races")
	case "animals":
		return T("animals")
	case "mechanoids":
		return T("mechanoids")
	case "creatures":
		return T("other creatures")
	case "buildings":
		return T("buildings")
	case "weapons":
		return T("weapons")
	case "apparel":
		return T("apparel")
	case "plants":
		return T("plants")
	case "items":
		return T("items")
	case "terrain":
		return T("terrain")
	case "research":
		return T("research projects")
	case "events":
		return T("events")
	case "quests":
		return T("quests")
	case "xenotypes":
		return T("xenotypes and genes")
	case "culture":
		return T("culture")
	case "abilities":
		return T("abilities")
	case "hediffs":
		return T("health conditions")
	case "traits":
		return T("traits")
	case "recipes":
		return T("recipes")
	case "appearance":
		return T("appearance")
	case "biomes":
		return T("biomes")
	case "storytellers":
		return T("storytellers")
	case "scenarios":
		return T("scenarios")
	case "unknown":
		return T("source not found")
	default:
		return T("other")
	}
}

// SaveNeeds is what the save needs of its Mods (12 §4.7, 16 §2.11 "Right"): the Mods
// grouped by what removing one costs, heaviest group first; empty groups are left out.
type SaveNeeds struct {
	Mods   int             `json:"mods"`           // the save's Mods, the game's own included
	Game   string          `json:"game,omitempty"` // Core and the DLC in the save, by name
	Groups []SaveNeedGroup `json:"groups"`
}

type SaveNeedGroup struct {
	Key   string        `json:"key"` // app.NeedBreaks …
	Label string        `json:"label"`
	Hint  string        `json:"hint"`
	Mods  []SaveNeedMod `json:"mods"`
}

type SaveNeedMod struct {
	Name      string         `json:"name"`
	PackageID string         `json:"packageId"`
	Library   string         `json:"library,omitempty"`
	Sum       string         `json:"sum,omitempty"` // what it holds, or why it is not checked
	Parts     []SaveNeedPart `json:"parts,omitempty"`
}

// SaveNeedPart is one kind of thing a Mod holds, with the names when there are any.
type SaveNeedPart struct {
	Label string   `json:"label"`
	Names []string `json:"names,omitempty"`
	More  int      `json:"more,omitempty"`
}

func saveNeedsOf(needs []app.SaveNeed) *SaveNeeds {
	if len(needs) == 0 {
		return nil
	}
	v := &SaveNeeds{Mods: len(needs), Groups: []SaveNeedGroup{}}
	by := map[string][]app.SaveNeed{}
	var game []string
	for _, n := range needs {
		if n.Game {
			game = append(game, n.Name)
			continue
		}
		by[n.Verdict] = append(by[n.Verdict], n)
	}
	v.Game = strings.Join(game, T(", "))
	for _, key := range app.NeedOrder {
		ns := by[key]
		if len(ns) == 0 {
			continue
		}
		// heaviest first; Mods with nothing to weigh keep the save's order
		weight := func(n app.SaveNeed) [3]int {
			return [3]int{n.Colonists + n.Dead, len(n.Factions) + len(n.Classes) + len(n.Weather), n.Defs}
		}
		sort.SliceStable(ns, func(i, j int) bool {
			a, b := weight(ns[i]), weight(ns[j])
			for k := range a {
				if a[k] != b[k] {
					return a[k] > b[k]
				}
			}
			return false
		})
		g := SaveNeedGroup{Key: key, Label: needLabel(key), Hint: needHint(key)}
		for _, n := range ns {
			g.Mods = append(g.Mods, saveNeedMod(n))
		}
		v.Groups = append(v.Groups, g)
	}
	return v
}

func needLabel(key string) string {
	switch key {
	case app.NeedBreaks:
		return T("breaks the save")
	case app.NeedLoses:
		return T("loses content")
	case app.NeedCare:
		return T("remove with care")
	case app.NeedSafe:
		return T("safe to remove")
	}
	return T("could not check")
}

func needHint(key string) string {
	switch key {
	case app.NeedBreaks:
		return T("removing one can stop the save loading: its pawns, factions or code are in it")
	case app.NeedLoses:
		return T("the save loads without one; its buildings, items and the like are gone")
	case app.NeedCare:
		return T("only research, recipes, appearance and the like: they reset and the colony goes on")
	case app.NeedSafe:
		return T("nothing of theirs is in the save, as far as the save can tell")
	}
	return T("not installed or not analysed: nothing can be said of them")
}

// needSumParts bounds a Mod's one-line summary; the rest is in its parts.
const needSumParts = 3

func saveNeedMod(n app.SaveNeed) SaveNeedMod {
	m := SaveNeedMod{Name: n.Name, PackageID: n.PackageID, Library: n.Library}
	if n.Verdict == app.NeedUnchecked {
		m.Sum = T(n.Why)
		return m
	}
	count := func(k int, one, many string) string {
		if k == 1 {
			return Tf("%d %s", k, T(one))
		}
		return Tf("%d %s", k, T(many))
	}
	if k := n.Colonists + n.Dead; k > 0 {
		l := count(k, "colonist", "colonists")
		if n.Dead > 0 {
			l = Tf("%s (%d dead)", l, n.Dead)
		}
		m.Parts = append(m.Parts, SaveNeedPart{Label: l})
	}
	if k := len(n.Factions); k > 0 {
		m.Parts = append(m.Parts, SaveNeedPart{Label: Tf("%d %s", k, contentCategoryLabelN("factions", k)), Names: n.Factions})
	}
	if k := len(n.Classes); k > 0 {
		names := n.Classes[:min(k, app.OwnedDefNames)]
		m.Parts = append(m.Parts, SaveNeedPart{Label: count(k, "class", "classes"), Names: names, More: k - len(names)})
	}
	if len(n.Weather) > 0 {
		m.Parts = append(m.Parts, SaveNeedPart{Label: T("a map's weather"), Names: n.Weather})
	}
	for _, c := range n.Cats {
		// said above by the faction's name and by the colonists
		if c.Key == "factions" && len(n.Factions) > 0 || c.Key == "races" && n.Colonists+n.Dead > 0 {
			continue
		}
		if c.Key == "other" && len(n.Weather) > 0 {
			// its weather is said above; the rest of "other" stays
			c.Count -= len(n.Weather)
			c.Names = slices.DeleteFunc(slices.Clone(c.Names), func(x app.SaveContentName) bool { return slices.Contains(n.Weather, x.Name) })
			if c.Count <= 0 {
				continue
			}
		}
		x := saveContentCat(c)
		m.Parts = append(m.Parts, SaveNeedPart{Label: Tf("%d %s", x.Count, x.Label), Names: x.Names, More: x.Count - len(x.Names)})
	}
	var sum []string
	for i, p := range m.Parts {
		if i == needSumParts {
			sum = append(sum, Tf("and %d more", len(m.Parts)-needSumParts))
			break
		}
		sum = append(sum, p.Label)
	}
	m.Sum = strings.Join(sum, T(", "))
	return m
}
