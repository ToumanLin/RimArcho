package cmds

// The error pattern library in the log outputs (archo log, MCP analyze_log) and the
// pattern:<id> entity (archo pattern). docs/research/01-范式库接入草案.md, 03 §6.3.

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/patterns"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "pattern", Args: "[<pattern:id>]", Summary: "error patterns from public logs: what a kind of log error usually is, its causes, route and fixes",
		MaxArgs: 1, Run: patternCmd,
		Flags: []Flag{{Name: "mod", Value: "<packageId>", Help: "the mod-specific patterns of one mod"}, {Name: "all", Help: "every pattern, not only the first ones"}}})
}

// clusterPatterns matches a cluster against the library; mod-specific patterns only for the
// mods the cluster's own attribution names as source.
func clusterPatterns(ix *app.Index, lc *app.LogCluster) patterns.Match {
	var src []string
	for _, at := range lc.Attr {
		if at.Role != app.RoleSource {
			continue
		}
		if m := ix.ByID[at.Instance]; m != nil && m.PackageID != "" {
			src = append(src, strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"))
		}
	}
	return patterns.Load().MatchCluster(lc.Class, lc.Template, src)
}

// patternTag is the short reference on a cluster's list line.
func patternTag(ix *app.Index, lc *app.LogCluster) string {
	if m := clusterPatterns(ix, lc); m.Generic != nil {
		return "  [" + m.Generic.Ref() + "]"
	}
	return ""
}

// addClusterPatterns adds what the library knows about a cluster to its detail: the generic
// kind with causes, route and fixes, and a known error of its source mod. Community
// background: the confidence and attribution above stay RimArcho's own.
func addClusterPatterns(root *render.Node, ix *app.Index, lc *app.LogCluster) {
	m := clusterPatterns(ix, lc)
	d := patterns.Load()
	if g := m.Generic; g != nil {
		n := root.Add("pattern: " + g.Ref() + "  " + g.Summary)
		n.KV("evidence", fmt.Sprintf("community: seen in %s (%.1f%% of error clusters); what this kind of error usually is, not evidence about this list", d.Seen(g), d.Share(g)))
		if len(g.Causes) > 0 {
			n.KV("causes", strings.Join(g.Causes, "; "))
		}
		if r := clusterRoute(g, lc.ID); r != "" {
			n.KV("route (tools)", r)
		}
		n.KV("fixes", strings.Join(g.Fixes, ", "))
		n.KV("repeat-uploads", g.Res.Line())
	}
	if p := m.Mod; p != nil {
		when := p.First
		if p.Last != "" && p.Last != p.First {
			when += " – " + p.Last
		}
		s := fmt.Sprintf("%s  this error of %s was seen in %s (%s); a newer version of the mod may have fixed it", p.Ref(), p.Mod, d.Seen(p), when)
		if len(p.Versions) > 0 {
			s += "; versions seen: " + strings.Join(p.Versions, ", ")
		}
		n := root.Add("known-error: " + s)
		n.KV("repeat-uploads", p.Res.Line())
	}
}

// clusterRoute is a pattern's route for one cluster: the step that is this very call is left
// out and E<n> becomes the cluster.
func clusterRoute(p *patterns.Pattern, id string) string {
	var out []string
	for _, r := range p.Route {
		if strings.HasPrefix(r, "analyze_log cluster=E<n>") {
			continue
		}
		out = append(out, strings.ReplaceAll(r, "E<n>", id))
	}
	return strings.Join(out, " → ")
}

func patternCmd(c *Ctx, a *Args) (*render.Doc, error) {
	d := patterns.Load()
	if len(a.Pos) == 1 && a.Pos[0] != "list" {
		p := d.Get(a.Pos[0])
		if p == nil {
			return nil, &app.RefError{Ref: a.Pos[0], Hint: "archo pattern lists the patterns"}
		}
		return patternDoc(d, p), nil
	}
	if pkg := a.Str("mod"); pkg != "" {
		pkg = strings.TrimSuffix(strings.ToLower(strings.TrimPrefix(pkg, "pkg:")), "_steam")
		ps := d.OfMod(pkg)
		doc, root := render.New(fmt.Sprintf("patterns of %s: %d known errors in public %s logs", pkg, len(ps), d.Game))
		lst := root.Section("list")
		for _, p := range ps {
			lst.Addf("%s  %s logs %s–%s  %s", p.Ref(), patterns.Thousands(p.Logs), p.First, p.Last, clip(templateMessage(p.Template), 110))
		}
		if len(ps) == 0 {
			lst.Add("(none: only errors seen in at least 10 logs from 5 players are kept)")
		}
		lst.Truncate(boolLimit(a, render.DefaultLimit), "archo pattern --mod "+pkg+" --all")
		if len(ps) > 0 {
			doc.AddNext("archo pattern " + ps[0].Ref())
		}
		return doc, nil
	}
	gs := append([]*patterns.Pattern(nil), d.Rules...)
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].NClusters > gs[j].NClusters })
	doc, root := render.New(fmt.Sprintf("patterns %s: %d kinds of log errors and %d mod-specific errors from %s public %s logs", d.Version, len(d.Rules), len(d.Mod), patterns.Thousands(d.Logs), d.Game))
	root.KV("evidence", "community: what a kind of error usually is, never which mod causes it in one list")
	lst := root.Section("list")
	for _, p := range gs {
		fb := ""
		if p.Fallback {
			fb = " (fallback: names the class only)"
		}
		lst.Addf("%s  %s  %4.1f%%  %s logs  %s%s", p.Ref(), p.Stage, d.Share(p), patterns.Thousands(p.Logs), p.Summary, fb)
	}
	lst.Truncate(boolLimit(a, render.DefaultLimit), "archo pattern --all")
	doc.AddNext("archo pattern pattern:g:xref.def", "archo pattern --mod <packageId>")
	return doc, nil
}

func patternDoc(d *patterns.Data, p *patterns.Pattern) *render.Doc {
	if !p.IsGeneric() {
		doc, root := render.New(p.Ref() + "  a known error of " + p.Mod)
		root.KV("evidence", "community: public logs attributed this error to the mod (RimArcho's attribution, not a confirmed bug); a newer version may have fixed it")
		root.KV("message", templateMessage(p.Template))
		root.KV("class", p.Class)
		root.KV("seen", fmt.Sprintf("%s, %d players' sessions, %s – %s", d.Seen(p), p.Sessions, p.First, p.Last))
		root.KV("versions", strings.Join(p.Versions, ", "))
		if l := p.Res.Line(); l != "" {
			root.KV("repeat-uploads", l)
			if f := patterns.FixList(p.Res.Fixes); f != "" {
				root.KV("changed-when-gone", f+" (disable-source: this mod removed)")
			}
		}
		if k := d.Get(p.Kind); k != nil {
			root.KV("kind", k.Ref()+"  "+k.Summary)
			doc.AddNext("archo pattern " + k.Ref())
		}
		return doc
	}
	doc, root := render.New(p.Ref() + "  " + p.Summary)
	root.KV("evidence", fmt.Sprintf("community: patterns %s from %s public HugsLib logs of RimWorld %s (%s); what this kind of error usually is, not evidence about one list", d.Version, patterns.Thousands(d.Logs), d.Game, d.Generated))
	cls := p.Class
	if cls == "" {
		cls = "any"
	}
	rec := fmt.Sprintf("cluster class %s, message matches /%s/", cls, p.Match)
	if p.Fallback {
		rec += " (fallback: names the class only, not shown on clusters)"
	}
	root.KV("recognises", rec)
	root.KV("stage", p.Stage)
	root.KV("seen", fmt.Sprintf("%s, %s players' sessions, %s clusters (%.1f%% of error clusters)", d.Seen(p), patterns.Thousands(p.Sessions), patterns.Thousands(p.NClusters), d.Share(p)))
	if len(p.Causes) > 0 {
		cs := root.Section("causes")
		for _, x := range p.Causes {
			cs.Add(x)
		}
	}
	if len(p.Route) > 0 {
		rs := root.Section("route")
		for _, x := range p.Route {
			rs.Add(x)
		}
	}
	root.KV("fixes", strings.Join(p.Fixes, ", "))
	if l := p.Res.Line(); l != "" {
		root.KV("repeat-uploads", l)
		if f := patterns.FixList(p.Res.Fixes); f != "" {
			root.KV("changed-when-gone", f+" (co-occurs with the change, not causal)")
		}
	}
	n := 0
	for _, m := range d.Mod {
		if m.Kind == p.ID {
			n++
		}
	}
	if n > 0 {
		root.KV("mod-specific", fmt.Sprintf("%d known errors of single mods are of this kind (archo pattern --mod <packageId>)", n))
	}
	return doc
}

// templateMessage drops the class prefix of a cluster template.
func templateMessage(t string) string {
	if i := strings.IndexByte(t, '|'); i >= 0 {
		return t[i+1:]
	}
	return t
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	n -= 3
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
