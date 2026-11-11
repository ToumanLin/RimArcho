package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func addSaveContent(root *render.Node, c *app.SaveContentResult, all bool, ref string, budget int) {
	if !c.Read {
		root.KV("content", c.Note)
		return
	}
	if c.Note != "" {
		root.KV("note", c.Note)
		budget--
	}
	root.KV("content", render.Plural(c.Defs, "distinct name")+"; sources use installed Mod versions")
	budget--
	parts := func(cats []app.SaveContentCategory) string {
		var out []string
		for _, cat := range cats {
			out = append(out, fmt.Sprintf("%d %s", cat.Count, contentTextLabel(cat.Key, cat.Count)))
		}
		if len(out) == 0 {
			return "none"
		}
		return strings.Join(out, ", ")
	}
	root.KV("totals", parts(c.Totals))
	budget--
	if split := needsSplit(c.Needs); split != "" {
		root.KV("removing a Mod", split)
		budget--
		if b := needsBreaking(c.Needs, all); b != "" {
			root.KV("breaks the save", b)
			budget--
		}
	}
	tail := 1 + len(app.ContentLimits)
	if c.Unknown != nil {
		tail++
	}
	if len(c.Unchecked) > 0 {
		tail++
	}
	limit := len(c.Mods)
	if !all {
		limit = min(limit, max(0, budget-tail-1))
	}
	for _, m := range c.Mods[:limit] {
		root.KV(m.Name, parts(m.Cats))
	}
	if limit < len(c.Mods) {
		root.Addf("… %d more Mods (archo saves show %s --content --all)", len(c.Mods)-limit, ref)
	}
	if c.Unknown != nil {
		root.KV("source not found", render.Plural(c.Unknown.Count, "name"))
	}
	if k := c.GridUnnamed["TerrainDef"] + c.GridUnnamed["BiomeDef"] + c.GridUnnamed["ThingDef"]; k > 0 {
		root.KV("terrain, biomes and rock not named", render.Plural(k, "kind"))
	}
	if len(c.Unchecked) > 0 {
		names := c.Unchecked
		if !all && len(names) > 6 {
			names = append(names[:6:6], fmt.Sprintf("and %d more (--all)", len(c.Unchecked)-6))
		}
		root.KV("not analysed", strings.Join(names, ", "))
	}
	n := root.Add("limits:")
	for _, text := range app.ContentLimits {
		n.Add(text)
	}
}

func contentTextLabel(k string, n int) string {
	if k == "xenotypes" {
		if n == 1 {
			return "xenotype or gene"
		}
		return "xenotypes and genes"
	}
	word := map[string]string{"races": "humanlike race", "creatures": "other creature", "research": "research project", "hediffs": "health condition", "abilities": "ability", "apparel": "apparel item", "terrain": "terrain type", "culture": "cultural element", "appearance": "appearance option", "other": "other entry"}[k]
	if word == "" {
		word = strings.TrimSuffix(k, "s")
	}
	return strings.TrimPrefix(render.Plural(n, word), fmt.Sprintf("%d ", n))
}

// needsSplit counts the save's Mods by what removing one costs (12 §4.7), the game's own
// left out: "2 break the save, 1 loses content, 3 safe to remove".
func needsSplit(needs []app.SaveNeed) string {
	n := map[string]int{}
	for _, m := range needs {
		if !m.Game {
			n[m.Verdict]++
		}
	}
	var out []string
	for _, k := range app.NeedOrder {
		if n[k] == 0 {
			continue
		}
		text := needText[k][1]
		if n[k] == 1 {
			text = needText[k][0]
		}
		out = append(out, fmt.Sprintf("%d %s", n[k], text))
	}
	return strings.Join(out, ", ")
}

// needText is a verdict after a count of one, and of several.
var needText = map[string][2]string{
	app.NeedBreaks:    {"breaks the save", "break the save"},
	app.NeedLoses:     {"loses content", "lose content"},
	app.NeedCare:      {"leaves only marks", "leave only marks"},
	app.NeedSafe:      {"safe to remove", "safe to remove"},
	app.NeedUnchecked: {"not checked", "not checked"},
}

// needsBreaking names the Mods whose removal breaks the save, with what each holds.
func needsBreaking(needs []app.SaveNeed, all bool) string {
	var out []string
	for _, m := range needs {
		if m.Game || m.Verdict != app.NeedBreaks {
			continue
		}
		var parts []string
		if k := m.Colonists + m.Dead; k > 0 {
			p := render.Plural(k, "colonist")
			if m.Dead > 0 {
				p += fmt.Sprintf(" (%d dead)", m.Dead)
			}
			parts = append(parts, p)
		}
		if k := len(m.Factions); k > 0 {
			parts = append(parts, render.Plural(k, "faction"))
		}
		if k := len(m.Classes); k > 0 {
			if k == 1 {
				parts = append(parts, "1 class")
			} else {
				parts = append(parts, fmt.Sprintf("%d classes", k))
			}
		}
		if len(m.Weather) > 0 {
			parts = append(parts, "a map's weather")
		}
		if k := len(m.NeededBy); k > 0 {
			parts = append(parts, "needed by "+render.Plural(k, "Mod"))
		}
		if len(parts) == 0 { // a FactionDef the save names with no faction of it left
			parts = append(parts, "a faction Def")
		}
		out = append(out, m.Name+" ("+strings.Join(parts, ", ")+")")
	}
	if !all && len(out) > 6 {
		out = append(out[:6:6], fmt.Sprintf("and %d more (--all)", len(out)-6))
	}
	return strings.Join(out, ", ")
}
