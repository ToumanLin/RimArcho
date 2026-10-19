package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Progress receives progress lines of long commands (analysis); the CLI points it at stderr.
var Progress func(string)

func init() {
	register(&Command{Name: "analyze", Summary: "rebuild the Def and Patch facts (normally run on demand as a child process)", MaxArgs: 0, Run: analyzeCmd,
		Flags: []Flag{gameVersionFlag}})
	register(&Command{Name: "problems", Summary: "problems: declarations, Defs, Patches (03 §4)", MaxArgs: 0, Run: problemsCmd,
		Flags: []Flag{{Name: "subject", Value: "<ref>", Help: "only problems involving this mod"}, {Name: "code", Value: "<code>", Help: "code or prefix, e.g. dep or patch.no-match"},
			{Name: "severity", Value: "<error|warning|info>"}, {Name: "all"}, {Name: "detail"}, {Name: "ignored"}, {Name: "no-analyze", Help: "do not rebuild a stale analysis"}, {Name: "min-severity", Value: "<error|warning|info>"}, gameVersionFlag}})
	register(&Command{Name: "problems show", Args: "<p:ref>", Summary: "one problem with its evidence", MinArgs: 1, MaxArgs: 1, Run: problemShowCmd})
	register(&Command{Name: "problems ignore", Args: "<p:ref>", Summary: "ignore a problem until its mod changes", MinArgs: 1, MaxArgs: 1, Run: problemIgnoreCmd})
	register(&Command{Name: "problems unignore", Args: "<p:ref>", Summary: "stop ignoring a problem", MinArgs: 1, MaxArgs: 1, Run: problemUnignoreCmd})
}

func progress() func(string) { return Progress }

func analyzeCmd(c *Ctx, a *Args) (*render.Doc, error) {
	app.SetAnalyzeLimits()
	r, err := c.App.Analyze(func(s string) {
		if Progress != nil {
			Progress("progress: " + s)
		}
	})
	if err != nil {
		return nil, err
	}
	s := r.Stats
	d, root := render.New(fmt.Sprintf("analysis a:%d for %s: %d active mods, %s", r.ID, r.MM, s.ActiveMods, app.Elapsed(s.TotalMs)))
	root.KV("defs", fmt.Sprintf("%d from %d files (%d installed mods indexed, %d from cache)", s.Defs, s.DefFiles, s.InstalledMods, s.CachedMods))
	root.KV("patches", fmt.Sprintf("%d operations in %d files: %d failed, %d of mod-defined classes", s.TopOps, s.PatchFiles, s.Failed, s.UnknownOps))
	root.KV("harmony", fmt.Sprintf("%d patches in %d assemblies (%d types indexed)", s.HarmonyPatches, s.Assemblies, s.Types))
	if s.ParseErrors > 0 {
		root.KV("xml-errors", fmt.Sprint(s.ParseErrors))
	}
	d.AddNext("archo problems")
	return d, nil
}

// expand renders "@{id}" tokens as refs; the first mention of the subject gets its name.
func expand(ix *app.Index, s, subject string) string {
	named := false
	return finding.Expand(s, func(id string) string {
		m := ix.ByID[id]
		if m == nil {
			return "#?"
		}
		if id == subject && !named {
			named = true
			return ix.Ref(id) + " " + m.DisplayName()
		}
		return ix.Ref(id)
	})
}

func problemLine(ix *app.Index, p *app.Problem) string {
	l := fmt.Sprintf("%s %s  %s  (%s)", p.Severity, p.Code, expand(ix, p.Claim, p.SubjectInstance()), p.Ref)
	if p.IgnoreExpired {
		l += "  [ignore expired: the mod changed]"
	}
	return l
}

func addProblemDetail(ix *app.Index, n *render.Node, p *app.Problem) {
	for _, e := range p.Evidence {
		ref := ""
		if e.Ref != "" {
			ref = expand(ix, e.Ref, "") + " "
		}
		n.Add("evidence: " + e.Source + " " + ref + expand(ix, e.Text, ""))
	}
	n.Truncate(12, "archo problems show "+p.Ref)
	for _, s := range p.Suggestions {
		n.Add("fix: " + expand(ix, s, ""))
	}
}

func severityCounts(ps []*app.Problem) string {
	n := map[string]int{}
	for _, p := range ps {
		n[p.Severity]++
	}
	return fmt.Sprintf("%d error, %d warning, %d info", n[finding.Error], n[finding.Warning], n[finding.Info])
}

// analysisText describes the stored analysis.
func analysisText(c *Ctx, s *app.AnalysisState, err error) string {
	switch {
	case errors.Is(err, app.ErrStillComputing) && s != nil && s.Run != nil:
		return fmt.Sprintf("a:%d %s is stale; the new analysis is still computing in the background (call again in a few seconds)", s.Run.ID, c.App.FormatTime(s.Run.Finished))
	case errors.Is(err, app.ErrStillComputing):
		return "still computing in the background (declaration problems only; call again in a few seconds)"
	case err != nil:
		return "failed: " + err.Error() + " (declaration problems only)"
	case s == nil || s.Run == nil:
		return "not run (declaration problems only; archo analyze)"
	case !s.Fresh:
		return fmt.Sprintf("a:%d %s, stale: the Loadout or a mod changed (archo analyze)", s.Run.ID, c.App.FormatTime(s.Run.Finished))
	}
	return fmt.Sprintf("a:%d %s", s.Run.ID, c.App.FormatTime(s.Run.Finished))
}

func problemsCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ps, err := c.App.Problems(!a.Bool("no-analyze"), progress())
	if err != nil {
		return nil, err
	}
	ix := ps.State.Ix
	list := ps.List
	if a.Bool("ignored") {
		list = ps.Ignored
	}
	var subj string
	if r := a.Str("subject"); r != "" {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		subj = finding.Inst(m.ID)
	}
	code, sev, minSev := a.Str("code"), a.Str("severity"), a.Str("min-severity")
	full := a.Bool("all") || code != "" || subj != "" || sev == finding.Info
	var sel []*app.Problem
	hidden := map[string]int{}
	var versions []*app.Problem
	for _, p := range list {
		if subj != "" && !contains(p.Subjects, subj) {
			continue
		}
		if code != "" && p.Code != code && !strings.HasPrefix(p.Code, code+".") {
			continue
		}
		if sev != "" && p.Severity != sev {
			continue
		}
		if minSev != "" && sevRank(p.Severity) > sevRank(minSev) {
			continue
		}
		if !full && app.NoisyCodes[p.Code] {
			hidden[p.Code]++
			continue
		}
		if !full && p.Code == "version.unsupported" {
			versions = append(versions, p) // folded into one row below
			continue
		}
		sel = append(sel, p)
	}
	counted := append(append([]*app.Problem{}, sel...), versions...)
	head := "problems: " + severityCounts(counted)
	if a.Bool("ignored") {
		head = "ignored problems: " + severityCounts(counted)
	}
	d, root := render.New(head)
	root.KV("analysis", analysisText(c, ps.Analysis, ps.AnalysisErr))
	detail := a.Bool("detail") || len(sel) <= 3
	limit := render.DefaultLimit
	if detail {
		limit = 8
	}
	if a.Bool("all") {
		limit = 0
	}
	lst := root.Section("list")
	if len(sel) == 0 {
		lst.Add("(none)")
	}
	for _, p := range sel {
		n := lst.Add(problemLine(ix, p))
		if detail {
			addProblemDetail(ix, n, p)
		}
	}
	if len(versions) > 0 {
		lst.Add(versionSummary(versions, ps.State.GameMM()))
	}
	expandCmd := "archo problems --all"
	if a.Str("subject") != "" {
		expandCmd = "archo problems --subject " + a.Str("subject") + " --all"
	}
	lst.Truncate(limit, expandCmd)
	if len(hidden) > 0 {
		var hs []string
		for _, k := range []string{"def.override", "patch.contested"} {
			if hidden[k] > 0 {
				hs = append(hs, fmt.Sprintf("%d %s", hidden[k], k))
			}
		}
		root.KV("hidden", strings.Join(hs, ", ")+" (archo problems --all)")
	}
	if !a.Bool("ignored") && len(ps.Ignored) > 0 {
		root.KV("ignored", fmt.Sprintf("%d (archo problems --ignored)", len(ps.Ignored)))
	}
	if len(sel) > 0 {
		if !detail {
			d.AddNext("archo problems show " + sel[0].Ref)
		} else if len(sel[0].Suggestions) > 0 {
			d.AddNext(expand(ix, sel[0].Suggestions[0], ""))
		}
	}
	return d, nil
}

func findProblem(c *Ctx, ref string) (*app.ProblemSet, *app.Problem, error) {
	ps, err := c.App.Problems(false, nil)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range append(append([]*app.Problem{}, ps.List...), ps.Ignored...) {
		if p.Ref == ref {
			return ps, p, nil
		}
	}
	return ps, nil, &app.RefError{Ref: ref, Hint: "archo problems lists the current problems with their p: refs"}
}

func problemShowCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ps, p, err := findProblem(c, a.Pos[0])
	if err != nil {
		return nil, err
	}
	ix := ps.State.Ix
	d, root := render.New(problemLine(ix, p))
	root.KV("confidence", p.Confidence)
	root.KV("layer", p.Layer)
	var subs []string
	for _, s := range p.Subjects {
		subs = append(subs, expand(ix, s, ""))
	}
	root.KV("subjects", strings.Join(subs, " "))
	if p.Ignored {
		root.KV("ignored", "yes (archo problems unignore "+p.Ref+")")
	}
	addProblemDetail(ix, root, p)
	if len(p.Suggestions) > 0 {
		d.AddNext(expand(ix, p.Suggestions[0], ""))
	}
	return d, nil
}

func problemIgnoreCmd(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := c.App.IgnoreProblem(a.Pos[0], false)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New("ignored " + p.Ref + " " + p.Code)
	root.Add(expand(ix, p.Claim, p.SubjectInstance()))
	root.KV("until", "the mod changes (ignores are kept per lineage and expire when its files change)")
	d.AddNext("archo problems unignore " + p.Ref)
	return d, nil
}

func problemUnignoreCmd(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := c.App.IgnoreProblem(a.Pos[0], true)
	if err != nil {
		return nil, err
	}
	d, _ := render.New("no longer ignored: " + p.Ref + " " + p.Code)
	return d, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// addModProblems adds the "content:" and "problems:" lines of `archo mod` (05 §2.2). It
// uses the stored analysis as is (no rebuild).
func addModProblems(c *Ctx, ix *app.Index, m *store.Mod, root *render.Node) error {
	st, err := c.App.Loadout()
	if err != nil {
		return nil // no workspace: nothing to report
	}
	s, err := c.App.Analysis(st)
	if err != nil {
		return err
	}
	if s.Run != nil {
		defs := c.App.Store.DefCount(m.ID)
		ops, failed := c.App.Store.PatchCount(s.Run.ID, m.ID)
		line := fmt.Sprintf("defs %d, patches %d", defs, ops)
		if failed > 0 {
			line += fmt.Sprintf(" (%d failed)", failed)
		}
		if asms, _ := c.App.Store.Assemblies(m.ID); len(asms) > 0 {
			hp, manual := 0, false
			for _, a := range asms {
				hp += a.Patches
				manual = manual || a.Manual
			}
			line += fmt.Sprintf(", assemblies %d, harmony patches %d", len(asms), hp)
			if manual {
				line += " (+ manual Harmony.Patch)"
			}
		}
		if !s.Fresh {
			line += "  [analysis stale]"
		}
		root.KV("content", line)
	}
	ps, err := c.App.Problems(false, nil)
	if err != nil {
		return err
	}
	tok := finding.Inst(m.ID)
	var mine []*app.Problem
	for _, p := range ps.List {
		if contains(p.Subjects, tok) {
			mine = append(mine, p)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	n := root.KV("problems", severityCounts(mine))
	for _, p := range mine {
		n.Add(problemLine(ix, p))
	}
	n.Truncate(3, "archo problems --subject "+ix.Ref(m.ID))
	return nil
}

func sevRank(s string) int {
	switch s {
	case finding.Error:
		return 0
	case finding.Warning:
		return 1
	}
	return 2
}

// versionSummary folds version.unsupported into one row per game version (decisions-m3 §12.1).
func versionSummary(ps []*app.Problem, mm string) string {
	sev := finding.Info
	warn := 0
	for _, p := range ps {
		if p.Severity == finding.Warning {
			sev = finding.Warning
			warn++
		}
	}
	line := fmt.Sprintf("%s version.unsupported  %d enabled mods do not list %s", sev, len(ps), mm)
	if prev := app.PrevMinor(mm); prev != "" {
		line += fmt.Sprintf(" (%d list %s, %d older only)", len(ps)-warn, prev, warn)
	}
	return line + "  (archo problems --code version.unsupported)"
}
