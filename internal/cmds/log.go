package cmds

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/patterns"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "log", Summary: "attribute Player.log errors to mods (clusters, roles, evidence, next steps)", MaxArgs: 0, Run: logCmd,
		Flags: []Flag{{Name: "cluster", Value: "<E#>"}, {Name: "file", Value: "<path>"}, {Name: "prev", Help: "Player-prev.log"},
			{Name: "text", Value: "<log or stack text>"}, {Name: "all", Help: "include informational clusters"},
			{Name: "select", Help: "include this log's clusters in archo problems"},
			{Name: "since", Value: "<cursor>", Help: "only the clusters new or grown since an earlier archo log printed this cursor"},
			{Name: "follow", Help: "follow Player.log while the game runs: new clusters as they appear"},
			{Name: "once", Help: "with --follow: read what is there now and stop"}, {Name: "json", Help: "with --follow: one JSON object per cluster"}, gameVersionFlag}})
}

// loadLog picks the log to analyse: --text, --file, --prev, else the current Player.log.
func loadLog(c *Ctx, a *Args) (*app.LogReport, *app.LoadoutState, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, nil, err
	}
	if t := a.Str("text"); t != "" {
		if err := c.App.EnsureAttributionAnalysis(st, "", app.TextLogVersion(t), progress()); errors.Is(err, app.ErrStillComputing) {
			c.logPending = true
		} else if err != nil {
			return nil, nil, err
		}
		r, err := c.App.AnalyzeLogText(t)
		return r, st, err
	}
	path := a.Str("file")
	if path == "" {
		cur, prev := c.App.LogPaths(st)
		path = cur
		if path == "" {
			path = prev
		}
		if a.Bool("prev") {
			path = prev
		}
		if path == "" {
			return nil, nil, fmt.Errorf("no Player.log found for %s under %s (archo log --file <path>)", st.GameMM(), st.Ix.Env.UserDataRoot)
		}
	}
	if err := c.App.EnsureAttributionAnalysis(st, path, "", progress()); errors.Is(err, app.ErrStillComputing) {
		c.logPending = true
	} else if err != nil {
		return nil, nil, err
	}
	r, err := c.App.AnalyzeLog(path)
	return r, st, err
}

// analysedAs says when a log was attributed against its own game version (06 §4), when
// its game folder is no environment's (attributed against this one anyway), when the
// analysis was still running (MCP waits 10 s, 14 §1) and when mods were not analysed.
func analysedAs(c *Ctx, root *render.Node, r *app.LogReport) {
	if st := c.Loadout(); st != nil {
		if g := c.App.UnregisteredLogGame(st, r.Path); g != "" {
			root.KV("game", fmt.Sprintf("log game path %s is not this environment's game; attributed against %s", g, envRef(st.Ix.Env)))
		}
	}
	if c.logPending {
		root.KV("analysis", "still computing in the background: type, Harmony and patch evidence may be missing; run again in a few seconds")
	} else if r.Unanalysed > 0 {
		root.KV("analysis", "assemblies not analysed yet for "+app.EnabledMods(r.Unanalysed)+": their types and Harmony IDs name no mod (archo analyze)")
	}
	if r.AnalysedAs != "" {
		root.KV("analysed", fmt.Sprintf("as %s (log version); environment is %s", r.AnalysedAs, r.EnvMM))
	}
}

func roleLine(ix *app.Index, at *app.Attribution) string {
	return fmt.Sprintf("%s  %s  evidence: %s", modLabel(ix, at.Instance), at.Role, strings.Join(at.Evidence, "; "))
}

func logCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("follow") {
		return logFollow(c, a)
	}
	var cur app.LogCursor
	since := a.Str("since")
	if since != "" {
		if a.Str("cluster") != "" || a.Str("text") != "" {
			return nil, &UsageError{Msg: "--since lists the clusters of a log file: not with --cluster or --text"}
		}
		var err error
		if cur, err = app.ParseLogCursor(since); err != nil {
			return nil, &UsageError{Msg: err.Error()}
		}
	}
	r, st, err := loadLog(c, a)
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	if since != "" {
		return logSince(c, a, st, r, cur, since)
	}
	if id := strings.TrimPrefix(a.Str("cluster"), "log:"); id != "" {
		for _, lc := range r.Clusters {
			if lc.ID == id {
				return clusterDoc(c, ix, r, lc), nil
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
		root.KV("cursor", r.Cursor)
		if a.Bool("select") {
			if err := c.App.SelectLog(st, r.Path); err != nil {
				return nil, err
			}
			root.KV("selected", "archo problems now includes these clusters as log.* problems")
		}
	}
	var classes []string
	for _, k := range logs.Classes {
		if byClass[k] > 0 {
			classes = append(classes, fmt.Sprintf("%s %d", k, byClass[k]))
		}
	}
	analysedAs(c, root, r)
	root.KV("classes", strings.Join(classes, ", "))
	if len(r.Phases) > 0 {
		phases := root.Section("phases")
		for _, p := range r.Phases {
			line := p.Name
			if p.Save != "" {
				line += " " + p.Save
			}
			if !p.At.IsZero() {
				line += "  " + p.At.In(c.App.Loc).Format("15:04:05")
			}
			phases.Add(line)
		}
	}
	savesNote(c, root, st, r)
	lst := root.Section("list")
	var noise []*app.LogCluster
	folded := 0
	// Errors before warnings, and a missing texture (drawing only) last of the warnings;
	// the IDs keep their order of first appearance (06 §3).
	ordered := app.ListOrder(r.Clusters)
	for _, lc := range ordered {
		if logs.Informational(lc.Class) && !a.Bool("all") {
			hidden++
			if lc.Class == logs.ClassNoise {
				noise = append(noise, lc)
			}
			continue
		}
		if lc.ConsequenceOf != "" && !a.Bool("all") {
			folded++ // listed under the failed def file's cluster (06 §3)
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
		root.KV("hidden", fmt.Sprintf("%d informational clusters (archo log --all)%s", hidden, noiseNote(noise)))
	}
	if folded > 0 {
		root.KV("folded", fmt.Sprintf("%d clusters that follow from a def file that failed to load, counted on its row (archo log --all)", folded))
	}
	for _, lc := range ordered {
		if !logs.Informational(lc.Class) && lc.ConsequenceOf == "" {
			d.AddNext("archo log --cluster " + lc.ID)
			break
		}
	}
	return d, nil
}

// savesNote says when the run loaded a save that is not in Saves/: a test run cannot reproduce
// errors that need it (06 §3, MCP eval h8: agents searched the disk for it).
func savesNote(c *Ctx, root *render.Node, st *app.LoadoutState, r *app.LogReport) {
	missing := c.App.MissingSaves(st, r)
	if len(missing) == 0 {
		return
	}
	names := make([]string, 0, 3)
	for i, m := range missing {
		if i == 3 {
			names = append(names, fmt.Sprintf("and %d more", len(missing)-i))
			break
		}
		names = append(names, strconv.Quote(m))
	}
	what := "save " + names[0]
	if len(missing) > 1 {
		what = "saves " + strings.Join(names, ", ")
	}
	which := "which is"
	if len(missing) > 1 {
		which = "which are"
	}
	root.KV("saves", fmt.Sprintf("this run loaded %s, %s not in Saves/; a test run cannot reproduce errors that need that save", what, which))
}

// noiseNote names the hidden clusters that are known noise (logs.KnownNoise), so that a reader
// of the raw log knows those lines were recognised: "; known noise, not mod problems: log:E5
// material made in code, log:E6 translation error count".
func noiseNote(noise []*app.LogCluster) string {
	var parts []string
	for _, lc := range noise {
		if n := logs.KnownNoise(lc.First.Message); n != nil {
			parts = append(parts, "log:"+lc.ID+" "+n.Label)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "; known noise, not mod problems: " + strings.Join(parts, ", ")
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
	} else if commonUnattributed(lc) != "" {
		who = "  (" + patterns.CommonLabel + ")"
	}
	if lc.Consequences > 0 {
		who += fmt.Sprintf("  + %d consequent lines in %d clusters", lc.ConsequentLines, lc.Consequences)
	} else if lc.ConsequenceOf != "" {
		who = "  (consequence of log:" + lc.ConsequenceOf + ")"
	}
	return fmt.Sprintf("log:%s %s x%d%s  %s%s%s", lc.ID, lc.Class, lc.Count, logs.PhaseSummary(lc.Phases), msg, who, patternTag(ix, lc))
}

func clusterDoc(c *Ctx, ix *app.Index, r *app.LogReport, lc *app.LogCluster) *render.Doc {
	at := ""
	if len(lc.Lines) > 0 && lc.Lines[0] > 0 { // 0: only the companion log has it
		at = fmt.Sprintf("  first at %s:%d", r.Label, lc.Lines[0])
	}
	d, root := render.New(fmt.Sprintf("log:%s %s x%d%s", lc.ID, lc.Class, lc.Count, at))
	if phase := logs.PhaseSummary(lc.Phases); phase != "" {
		root.KV("phases", strings.TrimSpace(phase))
	}
	// The whole message up to 1200 bytes: the inner exception at its end is often the cause
	// (G7 agents re-read the raw log for what a 300-byte cut hid).
	msg := lc.First.Message
	if len(msg) > 1200 {
		msg = msg[:1197] + "..."
	}
	analysedAs(c, root, r)
	root.KV("message", msg)
	extra := 0
	for _, x := range lc.First.Extra {
		if strings.HasPrefix(x, "file: ") {
			root.KV("file", strings.TrimPrefix(x, "file: "))
			continue
		}
		if extra++; extra <= 6 {
			if len(x) > 300 {
				x = x[:297] + "..."
			}
			root.KV("also", x)
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
	if len(lc.Near) > 0 {
		nb := root.Section("nearby (other log messages around the first occurrence)")
		for _, x := range lc.Near {
			nb.Add(x)
		}
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
	if note := commonUnattributed(lc); note != "" {
		root.KV("note", note)
	}
	if fx := twoSourcesFix(ix, lc); fx != "" {
		root.KV("fixes", fx)
	}
	addClusterPatterns(root, ix, lc)
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

// twoSourcesFix is the `fixes:` line of an exception or Harmony-stack cluster with two
// source candidates: one may have thrown and the other made it throw, so the report names both
// and both fix places (06 §3, MCP eval h7: the thrower's guard hides the error, undoing the
// other's action removes it). "" for any other cluster.
func twoSourcesFix(ix *app.Index, lc *app.LogCluster) string {
	if lc.Class != logs.ClassException && lc.Class != logs.ClassHarmony {
		return ""
	}
	var who []string
	for _, at := range lc.Attr {
		if at.Role == app.RoleSource {
			who = append(who, modLabel(ix, at.Instance))
		}
	}
	if len(who) < 2 {
		return ""
	}
	return fmt.Sprintf("%d sources, %s: one may have thrown and the other made it throw (a null it passed, a list it changed, state it removed). Report both and name both fix places: a guard or a different loop in the one that threw hides the error; undoing the action of the one that caused it removes the error", len(who), strings.Join(who, " and "))
}

// logSince lists the clusters new or grown after a cursor (06 §6, MCP analyze_log since).
func logSince(c *Ctx, a *Args, st *app.LoadoutState, r *app.LogReport, cur app.LogCursor, since string) (*render.Doc, error) {
	s, err := c.App.Since(r, cur)
	if err != nil {
		return nil, &UsageError{Msg: err.Error()}
	}
	shown, hidden := 0, 0
	var lines []string
	var noise []*app.LogCluster
	for _, lc := range app.ListOrder(s.New) {
		if logs.Informational(lc.Class) && !a.Bool("all") {
			hidden++
			if lc.Class == logs.ClassNoise {
				noise = append(noise, lc)
			}
			continue
		}
		shown++
		mark := "new"
		if n := s.Then[lc.Key]; n > 0 && s.Stale == "" {
			mark = fmt.Sprintf("+%d", lc.Count-n)
		}
		lines = append(lines, mark+"  "+clusterLine(st.Ix, lc))
	}
	head := fmt.Sprintf("log %s (%d lines): %d new or grown since the cursor, %d clusters in all", r.Label, r.Lines, shown, len(r.Clusters))
	if s.Stale != "" {
		head = fmt.Sprintf("log %s (%d lines): the cursor is stale, %d clusters", r.Label, r.Lines, len(r.Clusters))
	}
	d, root := render.New(head)
	root.KV("file", r.Path)
	if s.Stale != "" {
		root.KV("stale", s.Stale+"; every cluster of the current log is listed")
	} else {
		root.KV("since", since)
	}
	root.KV("cursor", r.Cursor)
	analysedAs(c, root, r)
	savesNote(c, root, st, r)
	lst := root.Section("list")
	for _, l := range lines {
		lst.Add(l)
	}
	if shown == 0 {
		lst.Add("(nothing new)")
	}
	lst.Truncate(boolLimit(a, render.DefaultLimit), "archo log --all --since "+since)
	if hidden > 0 {
		root.KV("hidden", fmt.Sprintf("%d informational clusters (--all)%s", hidden, noiseNote(noise)))
	}
	if s.Moved != "" {
		d.AddNext("archo log --prev --since " + since)
	}
	for _, lc := range app.ListOrder(s.New) {
		if !logs.Informational(lc.Class) {
			d.AddNext("archo log --cluster " + lc.ID)
			break
		}
	}
	return d, nil
}
