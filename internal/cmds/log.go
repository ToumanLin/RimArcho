package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "log", Summary: "attribute Player.log errors to mods (clusters, roles, evidence, next steps)", MaxArgs: 0, Run: logCmd,
		Flags: []Flag{{Name: "cluster", Value: "<E#>"}, {Name: "file", Value: "<path>"}, {Name: "prev", Help: "Player-prev.log"},
			{Name: "text", Value: "<log or stack text>"}, {Name: "all", Help: "include informational clusters"},
			{Name: "select", Help: "include this log's clusters in archo problems"}, gameVersionFlag}})
}

// loadLog picks the log to analyse: --text, --file, --prev, else the current Player.log.
func loadLog(c *Ctx, a *Args) (*app.LogReport, *app.LoadoutState, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, nil, err
	}
	if t := a.Str("text"); t != "" {
		if err := c.App.EnsureLogAnalysis(st, app.TextLogVersion(t), progress()); err != nil && !errors.Is(err, app.ErrStillComputing) {
			return nil, nil, err
		}
		r, err := c.App.AnalyzeLogText(t)
		return r, st, err
	}
	path := a.Str("file")
	if path == "" {
		cur, prev := c.App.LogPaths(st)
		path = cur
		if a.Bool("prev") {
			path = prev
		}
		if path == "" {
			return nil, nil, fmt.Errorf("no Player.log found for %s under %s (archo log --file <path>)", st.GameMM(), st.Ix.Env.UserDataRoot)
		}
	}
	if err := c.App.EnsureLogAnalysis(st, app.LogVersionOf(path), progress()); err != nil && !errors.Is(err, app.ErrStillComputing) {
		return nil, nil, err
	}
	r, err := c.App.AnalyzeLog(path)
	return r, st, err
}

// analysedAs says when a log was attributed against its own game version (decisions-m4 §7.3).
func analysedAs(root *render.Node, r *app.LogReport) {
	if r.AnalysedAs != "" {
		root.KV("analysed", fmt.Sprintf("as %s (log version); environment is %s", r.AnalysedAs, r.EnvMM))
	}
}

func roleLine(ix *app.Index, at *app.Attribution) string {
	return fmt.Sprintf("%s  %s  evidence: %s", modLabel(ix, at.Instance), at.Role, strings.Join(at.Evidence, "; "))
}

func logCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, st, err := loadLog(c, a)
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	if id := strings.TrimPrefix(a.Str("cluster"), "log:"); id != "" {
		for _, lc := range r.Clusters {
			if lc.ID == id {
				return clusterDoc(ix, r, lc), nil
			}
		}
		return nil, &app.RefError{Ref: "log:" + id, Hint: "archo log lists the clusters"}
	}
	when := ""
	if !r.ModTime.IsZero() {
		when = ", " + r.ModTime.In(c.App.Loc).Format("2006-01-02 15:04")
	}
	ver := ""
	if r.Version != "" {
		ver = r.Version + ", "
	}
	shown, hidden := 0, 0
	byClass := map[string]int{}
	for _, lc := range r.Clusters {
		byClass[lc.Class]++
	}
	d, root := render.New(fmt.Sprintf("log %s (%s%d lines%s): %d clusters", r.Label, ver, r.Lines, when, len(r.Clusters)))
	if r.Path != "" {
		root.KV("file", r.Path)
		if a.Bool("select") {
			if err := c.App.SelectLog(st, r.Path); err != nil {
				return nil, err
			}
			root.KV("selected", "archo problems now includes these clusters as log.* problems")
		}
	}
	var classes []string
	for _, k := range []string{logs.ClassException, logs.ClassHarmony, logs.ClassXMLLoad, logs.ClassPatchFailed, logs.ClassXrefMissing, logs.ClassMaterial, logs.ClassOther} {
		if byClass[k] > 0 {
			classes = append(classes, fmt.Sprintf("%s %d", k, byClass[k]))
		}
	}
	analysedAs(root, r)
	root.KV("classes", strings.Join(classes, ", "))
	lst := root.Section("list")
	for _, lc := range r.Clusters {
		if lc.Class == logs.ClassOther && !a.Bool("all") {
			hidden++
			continue
		}
		shown++
		lst.Add(clusterLine(ix, lc))
	}
	if shown == 0 {
		lst.Add("(none)")
	}
	lst.Truncate(boolLimit(a, render.DefaultLimit), "archo log --all")
	if hidden > 0 {
		root.KV("hidden", fmt.Sprintf("%d informational clusters (archo log --all)", hidden))
	}
	for _, lc := range r.Clusters {
		if lc.Class != logs.ClassOther {
			d.AddNext("archo log --cluster " + lc.ID)
			break
		}
	}
	return d, nil
}

func clusterLine(ix *app.Index, lc *app.LogCluster) string {
	msg := lc.First.Message
	if len(msg) > 100 {
		msg = msg[:97] + "..."
	}
	who := ""
	if len(lc.Attr) > 0 {
		at := lc.Attr[0]
		who = "  → " + ix.Ref(at.Instance) + " (" + at.Role
		if len(lc.Attr) > 1 {
			who += fmt.Sprintf(", +%d", len(lc.Attr)-1)
		}
		who += ")"
	}
	return fmt.Sprintf("log:%s %s x%d  %s%s", lc.ID, lc.Class, lc.Count, msg, who)
}

func clusterDoc(ix *app.Index, r *app.LogReport, lc *app.LogCluster) *render.Doc {
	at := ""
	if len(lc.Lines) > 0 {
		at = fmt.Sprintf("  first at %s:%d", r.Label, lc.Lines[0])
	}
	d, root := render.New(fmt.Sprintf("log:%s %s x%d%s", lc.ID, lc.Class, lc.Count, at))
	msg := lc.First.Message
	if len(msg) > 300 {
		msg = msg[:297] + "..."
	}
	analysedAs(root, r)
	root.KV("message", msg)
	for _, x := range lc.First.Extra {
		if strings.HasPrefix(x, "file: ") {
			root.KV("file", strings.TrimPrefix(x, "file: "))
		}
	}
	if len(lc.First.Frames) > 0 {
		fr := root.Section("top-frames")
		for i, f := range lc.First.Frames {
			if i >= 8 {
				fr.Addf("… %d more frames", len(lc.First.Frames)-i)
				break
			}
			line := strings.ReplaceAll(f.Type, "+", ".") + "." + f.Method
			if f.Patched {
				line += "  (patched by Harmony)"
			}
			n := fr.Add(line)
			for _, p := range f.Harmony {
				n.Add(p.Text)
			}
		}
	}
	for _, x := range lc.Details {
		root.KV("detail", x)
	}
	att := root.Section("attribution")
	if len(lc.Attr) == 0 {
		att.Add("(no mod identified)")
	}
	for _, x := range lc.Attr {
		att.Add(roleLine(ix, x))
	}
	att.Truncate(12, "")
	conf := lc.Confidence
	if lc.Why != "" {
		conf += " (" + lc.Why + ")"
	}
	root.KV("confidence", conf)
	if len(lc.Checklist) > 0 {
		cl := root.Section("checklist")
		for _, x := range lc.Checklist {
			cl.Add(x)
		}
	}
	d.AddNext(lc.Next...)
	if len(d.Next) > 6 {
		d.Next = d.Next[:6]
	}
	return d
}
