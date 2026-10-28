package cmds

import (
	"fmt"
	"net/url"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func parentProblemDoc(c *Ctx, ps *app.ProblemSet, p *app.Problem, all bool) (*render.Doc, error) {
	h, err := c.App.ParentHelp(ps, p)
	if err != nil {
		return nil, err
	}
	m := ps.State.Ix.ByID[p.SubjectInstance()]
	name := ps.State.Ix.Ref(p.SubjectInstance())
	if m != nil {
		name += " " + m.DisplayName()
	}
	d, root := render.New(h.Meaning(name) + " (" + p.Ref + ")")
	fixes := root.Add("fixes:")
	for _, f := range h.Fixes {
		switch f.Kind {
		case "enable":
			fixes.Add("fix: archo enable " + f.Ref + "  (" + f.Name + " defines " + f.Parent + ")")
		case "subscribe":
			fixes.Add("candidate (" + f.Basis + ", provider unconfirmed): " + f.Name + "; archo subscribe workshop " + f.Ref + "; " + f.URL)
		case "search":
			fixes.Add("candidate (" + f.Basis + ", provider unconfirmed): " + f.Name + "; https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(f.Name))
		case "requirements":
			u := f.URL
			if u == "" {
				u = "https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(f.Name)
			}
			root.Add("check this mod's Workshop requirements: " + u)
		}
	}
	if !all {
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
	if h.CacheIncomplete {
		root.Add("providers: file check is out of date; archo analyze, then inspect this problem again")
	}
	affected := root.Add("affected:")
	for _, part := range h.Parts {
		affected.Add(fmt.Sprintf("%s %s cannot load: missing base content %s", part.Type, part.Name, part.Parent))
	}
	if !all {
		affected.Truncate(8, "archo problems --subject "+ps.State.Ix.Ref(p.SubjectInstance())+" --code def.parent-missing --detail --all")
	}
	ev := root.Add("evidence:")
	seen := map[string]bool{}
	for _, q := range h.Problems {
		for _, e := range q.Evidence {
			line := expand(ps.State.Ix, e.Text, "")
			if seen[line] {
				continue
			}
			seen[line] = true
			ev.Add(line)
		}
	}
	if !all {
		ev.Truncate(5, "archo problems --subject "+ps.State.Ix.Ref(p.SubjectInstance())+" --code def.parent-missing --detail --all")
	}
	if p.Ignored {
		root.Add("ignored: yes; archo problems unignore " + p.Ref)
	}
	return d, nil
}
