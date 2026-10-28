package cmds

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// The two other needs inspections that formerly opened an all-patches/card dump.
// Differences and log inspections already describe only their requested subject.
func problemDecisionDoc(c *Ctx, ps *app.ProblemSet, p *app.Problem) *render.Doc {
	ix := ps.State.Ix
	m := ix.ByID[p.SubjectInstance()]
	if m == nil {
		return nil
	}
	var title string
	switch p.Code {
	case "patch.no-match":
		if len(p.Subjects) < 2 {
			return nil
		}
		target := p.Subjects[1]
		name := expand(ix, target, finding.InstID(target))
		title = fmt.Sprintf("%s %s: some changes cannot take effect because %s is disabled (%s)", ix.Ref(m.ID), m.DisplayName(), name, p.Ref)
	case "mod.known-error":
		title = fmt.Sprintf("%s %s has a known error in shared logs regardless of its companions; an update may fix it (%s)", ix.Ref(m.ID), m.DisplayName(), p.Ref)
	default:
		return nil
	}
	d, root := render.New(title)
	for _, fix := range p.Suggestions {
		if strings.HasPrefix(fix, "archo enable ") {
			root.Add("fix: " + expand(ix, fix, ""))
		}
	}
	ws := m.WorkshopID()
	if ws == "" {
		if w, ok := c.App.WorkshopFor(m.PackageID); ok {
			ws = w.WorkshopID
		}
	}
	u := "https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(m.DisplayName())
	if ws != "" {
		u = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
	}
	root.Add("check the mod's Workshop page: " + u)
	root.Add("evidence: " + expand(ix, p.Claim, ""))
	ev := root.Add("evidence:")
	for _, e := range p.Evidence {
		ev.Add(expand(ix, e.Text, ""))
	}
	ev.Truncate(5, "archo problems --subject "+ix.Ref(m.ID)+" --code "+p.Code+" --detail --all")
	return d
}
