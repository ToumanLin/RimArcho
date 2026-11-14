package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// addSaveContent is saves show --content; with pick (--mod) it says what removing those
// Mods costs in place of the list of every Mod.
func addSaveContent(root *render.Node, c *app.SaveContentResult, all bool, ref string, budget int, pick []app.SaveNeed) {
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
		if b := needsBreaking(c.Needs, all); b != "" && pick == nil {
			root.KV("breaks the save", b)
			budget--
		}
	}
	// a picked Mod is answered even when the save has only game content
	for _, m := range pick {
		addNeed(root, m)
	}
	tail := 1 + len(app.ContentLimits)
	if c.Unknown != nil {
		tail++
	}
	if len(c.Unchecked) > 0 {
		tail++
	}
	limit := len(c.Mods)
	if pick != nil {
		limit = 0
	} else if !all {
		limit = min(limit, max(0, budget-tail-1))
	}
	for _, m := range c.Mods[:limit] {
		root.KV(m.Name, parts(m.Cats))
	}
	if limit < len(c.Mods) && pick == nil {
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
		parts := needHolds(m)
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

// needHolds is what of a Mod makes the save break without it: its pawns, factions, classes
// and weather.
func needHolds(m app.SaveNeed) []string {
	var parts []string
	if k := m.Colonists + m.Dead; k > 0 {
		p := render.Plural(k, "colonist")
		if m.Dead > 0 {
			p += fmt.Sprintf(" (%d dead)", m.Dead)
		}
		parts = append(parts, p)
	}
	if m.World > 0 {
		parts = append(parts, render.Plural(m.World, "world pawn"))
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
	return parts
}

// needWhy says what a verdict means for the one Mod (saves show --mod).
var needWhy = map[string]string{
	app.NeedBreaks: "the save may not load without it",
	app.NeedLoses:  "the save loads without it; its things in the world are gone",
	app.NeedCare:   "research, recipes, appearance and the like reset and the colony goes on",
	app.NeedSafe:   "nothing of it is in the save and none of the save's Mods needs it, as far as the save can tell",
}

// addNeed is one Mod's verdict with why and what it holds (saves show --mod).
func addNeed(root *render.Node, m app.SaveNeed) {
	verdict := needText[m.Verdict][0]
	why := needWhy[m.Verdict]
	switch {
	case m.Game:
		verdict, why = "part of the game", "always needed"
	case m.Verdict == app.NeedUnchecked:
		why = m.Why
	case len(m.Cats) == 0 && len(needHolds(m)) == 0 && !m.Grid && len(m.NeededBy) > 0:
		// nothing of its own in the save: its verdict is that of the Mods needing it
		if m.Verdict == app.NeedCare {
			why = "only Mods that are safe to remove need it, but they do not run without it"
		} else {
			why += ", as the Mods that need it do not run without it"
		}
	}
	n := root.Add(fmt.Sprintf("%s (%s): %s", m.Name, m.PackageID, verdict))
	n.KV("why", why)
	holds := needHolds(m)
	for _, cat := range m.Cats {
		if cat.Key == "factions" && len(m.Factions) > 0 || cat.Key == "races" && m.Colonists+m.Dead+m.World > 0 {
			continue // said above, by the faction and the pawns
		}
		holds = append(holds, fmt.Sprintf("%d %s", cat.Count, contentTextLabel(cat.Key, cat.Count)))
	}
	if m.Grid {
		holds = append(holds, "terrain, biomes or rock on a map or the world")
	}
	n.KV("holds", strings.Join(holds, ", "))
	n.KV("needed by", strings.Join(m.NeededBy, ", "))
}

// saveNeedsOf picks the save's Mods ref names (saves show --mod): by packageId or name,
// else by what the Mod reference resolves to; an error when none of the save's Mods is it.
func saveNeedsOf(c *Ctx, needs []app.SaveNeed, ref string) ([]app.SaveNeed, error) {
	var out []app.SaveNeed
	want := strings.TrimSpace(ref)
	if p, ok := app.PackageRef(want); ok && p != "" {
		want = p
	}
	for _, m := range needs {
		if normPkgID(m.PackageID) == normPkgID(want) || strings.EqualFold(m.Name, strings.TrimSpace(ref)) {
			out = append(out, m)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	pkgs, _, err := modFilter(c, ref)
	if err != nil {
		return nil, err
	}
	for _, m := range needs {
		if pkgs[normPkgID(m.PackageID)] {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s is not one of the save's Mods", ref)
	}
	return out, nil
}
