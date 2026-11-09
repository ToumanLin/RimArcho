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
