package view

import "github.com/ToumanLin/RimArcho/internal/app"

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
	return v
}

func saveContentCat(c app.SaveContentCategory) SaveContentCat {
	v := SaveContentCat{Key: c.Key, Label: contentCategoryLabel(c.Key), Count: c.Count, Uses: c.Uses, Under: c.Under, Hidden: c.Hidden}
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
