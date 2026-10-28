package cmds

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func parentProblemDoc(c *Ctx, ps *app.ProblemSet, p *app.Problem, all bool) (*render.Doc, error) {
	h, err := c.App.FindParentHelp(c.Context, ps, p)
	if err != nil {
		return nil, err
	}
	m := ps.State.Ix.ByID[p.SubjectInstance()]
	name := ps.State.Ix.Ref(p.SubjectInstance())
	if m != nil {
		name += " " + m.DisplayName()
	}
	d, root := render.New(h.Meaning(name) + " (" + p.Ref + ")")
	var fixes *render.Node
	fix := func(line string) {
		if fixes == nil {
			fixes = root.Add("fixes:")
		}
		fixes.Add(line)
	}
	for _, f := range h.Fixes {
		switch f.Kind {
		case "enable":
			fix("fix: archo enable " + f.Ref + "  (" + f.Name + " defines " + f.Parent + ")")
		case "subscribe":
			fix("candidate (" + f.Basis + ", provider unconfirmed): " + f.Name + "; archo subscribe workshop " + f.Ref + "; " + f.URL)
		case "search":
			fix("candidate (" + f.Basis + ", provider unconfirmed): " + f.Name + "; https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(f.Name))
		case "requirements":
			u := f.URL
			if u == "" {
				u = "https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(f.Name)
			}
			root.Add("check this mod's Workshop requirements: " + u)
		}
	}
	if fixes != nil && !all {
		fixes.Truncate(6, "archo problems --ref "+p.Ref+" --all")
	}
	root.Add("risk: mods with missing base content were error sources in 16% of shared logs (4.1% by chance); this is not a prediction for this list")
	if h.Quiet {
		root.Add("last run: no errors attributed to this mod in the matching log; probably harmless content for an optional mod")
		ignores := root.Add("optional, do not warn again:")
		for _, q := range h.Problems {
			if !q.Ignored {
				ignores.Add("fix: archo problems ignore " + q.Ref + "  (until the mod changes)")
			}
		}
		if !all {
			ignores.Truncate(3, "archo problems --ref "+p.Ref+" --all")
		}
	}
	if len(h.UnresolvedNames()) > 0 {
		if h.SearchIncomplete {
			root.Add("providers: not every installed mod could be checked")
		} else if !h.CacheIncomplete {
			root.Add("providers: no installed mod supplies it, enabled or not")
		}
	}
	affected := root.Add("affected:")
	for _, q := range h.Problems {
		part, ok := app.MissingParent(q)
		if !ok {
			continue
		}
		line := fmt.Sprintf("%s %s cannot load: missing base content %s", part.Type, part.Name, part.Parent)
		locations, _ := app.ParentLocations(q)
		if len(locations) > 0 {
			line += "  (" + strings.Join(locations, ", ") + ")"
		}
		affected.Add(line)
	}
	if !all {
		affected.Truncate(8, "archo problems --ref "+p.Ref+" --all")
	}
	var evidence []string
	seen := map[string]bool{}
	for _, q := range h.Problems {
		_, other := app.ParentLocations(q)
		for _, text := range other {
			line := expand(ps.State.Ix, text, "")
			if !seen[line] {
				seen[line] = true
				evidence = append(evidence, line)
			}
		}
	}
	if len(evidence) > 0 {
		ev := root.Add("evidence:")
		for _, line := range evidence {
			ev.Add(line)
		}
		if !all {
			ev.Truncate(5, "archo problems --ref "+p.Ref+" --all")
		}
	}
	if p.Ignored {
		root.Add("ignored: yes; archo problems unignore " + p.Ref)
	}
	return d, nil
}
