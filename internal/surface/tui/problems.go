package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/finding"
)

// Problems view (06 §4): grouped by severity, then by subject mod; enter expands the
// evidence and the suggested commands; a runs the selected suggestion (after a preview);
// i ignores; A re-analyses in a child process.

var severities = []string{"", finding.Error, finding.Warning, finding.Info}

type probState struct {
	list        []*app.Problem
	ignored     int
	analysis    string // one-line analysis state
	stale       bool
	never       bool
	cursor, off int
	expanded    map[string]bool
	sug         int // selected suggestion of the expanded problem under the cursor
	sev         int // index into severities
	showIgnored bool
	loaded      bool
	counts      [3]int
	countsKey   string
}

// analysis progress plumbing
type analyzeLineMsg struct {
	line string
	ch   <-chan string
}
type analyzeDoneMsg struct{ err error }

// AnalyzeRunner runs the Def/Patch analysis for an environment; nil in Options means the
// `archo analyze` child process.
type AnalyzeRunner func(envName string, progress func(string)) error

func (m *model) ensureProblems() tea.Cmd {
	// The player looks at problems now: include the newest log, parsing it if needed.
	m.app.LazyLog, m.app.LogCacheOnly = false, false
	m.loadProblems()
	if (m.prob.stale || m.prob.never) && !m.analyzing {
		return m.startAnalyze()
	}
	return nil
}

// loadProblems reads the problems (declaration layer now, Def/Patch layers as stored).
func (m *model) loadProblems() {
	ps, err := m.app.Problems(false, nil)
	p := &m.prob
	if p.expanded == nil {
		p.expanded = map[string]bool{}
	}
	p.loaded = true
	if err != nil {
		p.list, p.analysis = nil, T("error: ")+err.Error()
		return
	}
	// The newest log was parsed into this list (decisions-m4 §7.2): it has been seen.
	if m.newLog && m.st != nil {
		if cur, _ := m.app.LogPaths(m.st); cur != "" && !m.app.LazyLog && m.app.ProblemLog(m.st) == cur {
			m.app.SelectLog(m.st, cur)
			m.newLog = false
		}
	}
	src := append(append([]*app.Problem{}, ps.List...), ps.Stale...)
	if p.showIgnored {
		src = ps.Ignored
	}
	p.list = p.list[:0]
	all := severities[p.sev] == ""
	var versions, skips []*app.Problem
	byLog := map[*app.LogRun][]*app.Problem{} // log problems go under their log (03 §6.3)
	for _, x := range src {
		switch {
		case x.Log != nil:
			if all || x.Severity == severities[p.sev] {
				byLog[x.Log] = append(byLog[x.Log], x)
			}
			continue
		case all && app.NoisyCodes[x.Code]:
			continue // shown under the info filter (decisions-m3 §12.2)
		case all && x.Code == "version.unsupported":
			versions = append(versions, x)
			continue
		case all && x.Code == "harmony.skip-prefix" && x.Severity == finding.Info:
			skips = append(skips, x)
			continue
		}
		if all || x.Severity == severities[p.sev] {
			p.list = append(p.list, x)
		}
	}
	if len(versions) > 0 {
		p.list = append(p.list, versionRow(versions, m.st, ps.State.GameMM()))
	}
	if len(skips) > 0 {
		p.list = append(p.list, skipRow(skips))
	}
	for _, r := range ps.Logs {
		switch lp := byLog[r]; {
		case len(lp) == 0:
		case r.Stale() && all:
			p.list = append(p.list, staleLogRow(r, lp))
		default:
			p.list = append(p.list, lp...)
		}
	}
	p.ignored = len(ps.Ignored)
	p.never = ps.Analysis == nil || ps.Analysis.Run == nil
	p.stale = !p.never && !ps.Analysis.Fresh
	switch {
	case p.never:
		p.analysis = T("not analyzed yet — A to analyze (child process)")
	case p.stale:
		p.analysis = T("analysis is stale — A to re-analyze")
	default:
		p.analysis = fmt.Sprintf("a:%d %s", ps.Analysis.Run.ID, m.app.FormatTime(ps.Analysis.Run.Finished))
	}
	p.counts = [3]int{}
	for _, x := range ps.List {
		switch x.Severity {
		case finding.Error:
			p.counts[0]++
		case finding.Warning:
			p.counts[1]++
		default:
			p.counts[2]++
		}
	}
	p.countsKey = m.app.Store.ChangeKey()
	p.cursor = min(p.cursor, max(len(p.list)-1, 0))
}

// problemCounts for the status bar; recomputed when the database changed.
func (m *model) problemCounts() (string, bool) {
	if m.loadErr != nil {
		return "", false
	}
	if !m.prob.loaded || m.prob.countsKey != m.app.Store.ChangeKey() {
		keepSev, keepIg := m.prob.sev, m.prob.showIgnored
		m.loadProblems()
		m.prob.sev, m.prob.showIgnored = keepSev, keepIg
	}
	c := m.prob.counts
	return Tf("problems %d/%d/%d", c[0], c[1], c[2]), c[0] > 0
}

func (m *model) startAnalyze() tea.Cmd {
	if m.analyzing || m.loadErr != nil {
		return nil
	}
	m.analyzing = true
	m.status = T("analyzing…")
	run, name := m.opts.Analyze, m.envName
	if run == nil {
		home := m.app.Home
		run = func(env string, progress func(string)) error { return app.ChildAnalyze(home, env, "", progress) }
	}
	ch := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		done <- run(name, func(s string) {
			select {
			case ch <- s:
			default: // drop progress lines rather than block the analysis
			}
		})
		close(ch)
	}()
	var wait func() tea.Msg
	wait = func() tea.Msg {
		if l, ok := <-ch; ok {
			return analyzeLineMsg{line: l, ch: ch}
		}
		return analyzeDoneMsg{err: <-done}
	}
	m.analyzeWait = wait
	return wait
}

func (m *model) analyzeMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case analyzeLineMsg:
		m.status = T("analyzing…") + " " + strings.TrimPrefix(msg.line, "progress: ")
		return m.analyzeWait
	case analyzeDoneMsg:
		m.analyzing = false
		if msg.err != nil {
			m.openPanel(T("analysis failed"), msg.err.Error(), nil)
		} else {
			m.status = T("analysis done")
		}
		m.reload()
		m.loadProblems()
	}
	return nil
}

// problem rows: what the view shows, with the problem index of each line.
type prow struct {
	text  string
	pi    int // problem index, -1 for headings
	sug   int // suggestion index, -1 otherwise
	style int // 0 plain, 1 heading, 2 dim, 3 error, 4 warning
	main  bool
}

func (m *model) probRows() []prow {
	p := &m.prob
	ix := m.st.Ix
	var rows []prow
	group := func(x *app.Problem) string {
		if x.Log != nil {
			return "log:" + x.Log.Path
		}
		return x.Severity
	}
	grp, subj := "?", "?"
	for i, x := range p.list {
		if g := group(x); g != grp {
			grp, subj = g, "?"
			n := 0
			for _, y := range p.list {
				if group(y) == grp {
					n++
					if y.Ref == staleLogRef {
						n += len(y.Evidence) - 1
					}
				}
			}
			head := T(x.Severity)
			if x.Log != nil {
				head = m.logHeading(x.Log)
			}
			rows = append(rows, prow{text: fmt.Sprintf("── %s (%d) ──", head, n), pi: -1, sug: -1, style: 1})
		}
		if s := x.SubjectInstance(); s != subj {
			subj = s
			label := "—"
			if mod := ix.ByID[s]; mod != nil {
				label = ix.Ref(s) + " " + mod.DisplayName()
				if pos := m.st.Position(s); pos > 0 {
					label += fmt.Sprintf("  [%d]", pos)
				}
			}
			rows = append(rows, prow{text: "  " + label, pi: -1, sug: -1, style: 2})
		}
		mark := "▸"
		if p.expanded[x.Ref] {
			mark = "▾"
		}
		st := 0
		switch x.Severity {
		case finding.Error:
			st = 3
		case finding.Warning:
			st = 4
		}
		if x.Log != nil && x.Log.Stale() {
			st = 2 // not counted: no alarm colour
		}
		claim := finding.Expand(x.Claim, func(id string) string { return ix.Ref(id) })
		line := fmt.Sprintf("    %s %s  %s", mark, x.Code, claim)
		if x.IgnoreExpired {
			line += "  " + T("[ignore expired]")
		}
		rows = append(rows, prow{text: line, pi: i, sug: -1, style: st, main: true})
		if p.expanded[x.Ref] {
			for _, e := range x.Evidence {
				ref := ""
				if e.Ref != "" {
					ref = finding.Expand(e.Ref, ix.Ref) + " "
				}
				rows = append(rows, prow{text: "        " + T("evidence: ") + e.Source + " " + ref + finding.Expand(e.Text, ix.Ref), pi: i, sug: -1, style: 2})
			}
			for k, s := range x.Suggestions {
				rows = append(rows, prow{text: fmt.Sprintf("        [%c] %s", 'a'+rune(min(k, 25)), finding.Expand(s, ix.Ref)), pi: i, sug: k})
			}
			rows = append(rows, prow{text: "        " + x.Ref + "  " + x.Confidence, pi: i, sug: -1, style: 2})
		}
	}
	return rows
}

func (m *model) problemLines(n int) []string {
	p := &m.prob
	head := fmt.Sprintf("%s %d · %s %d · %s %d", T("error"), p.counts[0], T("warning"), p.counts[1], T("info"), p.counts[2])
	if severities[p.sev] != "" {
		head += "   " + T("severity: ") + T(severities[p.sev])
	}
	if p.showIgnored {
		head += "   " + T("showing ignored")
	} else if p.ignored > 0 {
		head += "   " + Tf("ignored %d (I shows)", p.ignored)
	}
	out := []string{stHeader.Render(fit(head, m.w)), stDim.Render(fit(p.analysis, m.w))}
	if m.st == nil {
		return out
	}
	rows := m.probRows()
	if len(p.list) == 0 {
		return append(out, "", "  "+T("no problems"))
	}
	// Scroll so the cursor's problem line is visible.
	cur := 0
	for i, r := range rows {
		if r.main && r.pi == p.cursor {
			cur = i
			break
		}
	}
	page := max(n-len(out), 1)
	if cur < p.off {
		p.off = max(cur-1, 0)
	}
	if cur >= p.off+page {
		p.off = cur - page + 1
	}
	if p.expanded[m.curProblemRef()] {
		// keep the expansion in view
		last := cur
		for i := cur; i < len(rows) && rows[i].pi == p.cursor; i++ {
			last = i
		}
		if last >= p.off+page {
			p.off = min(cur, last-page+1)
		}
	}
	for i := p.off; i < len(rows) && len(out) < n; i++ {
		r := rows[i]
		l := fit(r.text, m.w)
		switch {
		case i == cur:
			l = stSel.Render(l)
		case r.pi == p.cursor && r.sug == p.sug && r.sug >= 0:
			l = stSel.Render(l)
		case r.style == 1:
			l = stHeader.Render(l)
		case r.style == 2:
			l = stDim.Render(l)
		case r.style == 3:
			l = stErr.Render(l)
		case r.style == 4:
			l = stWarn.Render(l)
		}
		out = append(out, l)
	}
	return out
}

func (m *model) curProblem() *app.Problem {
	if m.prob.cursor < 0 || m.prob.cursor >= len(m.prob.list) {
		return nil
	}
	return m.prob.list[m.prob.cursor]
}

func (m *model) curProblemRef() string {
	if p := m.curProblem(); p != nil {
		return p.Ref
	}
	return ""
}

func (m *model) problemsKey(s string) tea.Cmd {
	p := &m.prob
	x := m.curProblem()
	switch s {
	case "up", "k":
		p.cursor, p.sug = max(p.cursor-1, 0), 0
	case "down", "j":
		p.cursor, p.sug = min(p.cursor+1, max(len(p.list)-1, 0)), 0
	case "pgup":
		p.cursor, p.sug = max(p.cursor-10, 0), 0
	case "pgdown":
		p.cursor, p.sug = min(p.cursor+10, max(len(p.list)-1, 0)), 0
	case "home", "g":
		p.cursor = 0
	case "end", "G":
		p.cursor = max(len(p.list)-1, 0)
	case "enter", " ":
		if x != nil {
			if p.expanded[x.Ref] && s == "enter" {
				return m.openInspect([]string{"problems", "show", x.Ref})
			}
			p.expanded[x.Ref] = !p.expanded[x.Ref]
			p.sug = 0
		}
	case "tab":
		if x != nil && p.expanded[x.Ref] && len(x.Suggestions) > 0 {
			p.sug = (p.sug + 1) % len(x.Suggestions)
		}
	case "a":
		if x == nil || len(x.Suggestions) == 0 {
			m.status = T("no suggested command for this problem")
			return nil
		}
		k := 0
		if p.expanded[x.Ref] {
			k = p.sug
		}
		return m.runSuggestion(finding.Expand(x.Suggestions[k], m.st.Ix.Ref))
	case "i":
		if x == nil {
			return nil
		}
		if x.Ref == "p:version" || x.Ref == "p:skip-prefix" || x.Ref == staleLogRef {
			m.status = T("select single mods with the s filter to ignore them")
			return nil
		}
		verb := "ignore"
		if p.showIgnored {
			verb = "unignore"
		}
		m.runToStatus("problems", verb, x.Ref)
		m.loadProblems()
	case "I":
		p.showIgnored = !p.showIgnored
		p.cursor, p.off = 0, 0
		m.loadProblems()
	case "s":
		p.sev = (p.sev + 1) % len(severities)
		p.cursor, p.off = 0, 0
		m.loadProblems()
		sev := severities[p.sev]
		if sev == "" {
			sev = "all"
		}
		m.status = T("severity: ") + T(sev)
	case "A":
		return m.startAnalyze()
	case "o":
		if x != nil {
			if id := x.SubjectInstance(); id != "" {
				return m.openInspect([]string{"mod", m.st.Ix.Ref(id)})
			}
		}
	}
	return nil
}

// runSuggestion previews a suggested command: read-only ones run at once, others ask.
func (m *model) runSuggestion(s string) tea.Cmd {
	if !strings.HasPrefix(s, "archo ") {
		m.openPanel(T("suggestion"), s+"\n\n"+T("(not a command; open it yourself)"), nil)
		return nil
	}
	argv := strings.Fields(strings.TrimPrefix(s, "archo "))
	if len(argv) == 0 {
		return nil
	}
	switch argv[0] {
	case "why", "def", "patches", "problems", "mod", "find", "files", "changes":
		return m.openInspect(argv)
	case "sort":
		out, _ := m.run("sort", "--dry-run")
		m.openPanel(T("sort — preview"), out, []string{"sort"})
		return nil
	}
	m.openPanel(s, T("run this command?"), argv)
	return nil
}

// statusProblems is the status bar segment.
func (m *model) statusProblems() string {
	s, bad := m.problemCounts()
	if s == "" {
		return T("problems —")
	}
	if m.prob.stale || m.prob.never {
		s += "*"
	}
	if bad {
		return stErr.Render(s)
	}
	return s
}

var _ = cmds.ExitOK

// versionRow folds version.unsupported into one expandable row (decisions-m3 §12.1).
func versionRow(vs []*app.Problem, st *app.LoadoutState, mm string) *app.Problem {
	f := &finding.Finding{Code: "version.unsupported", Severity: finding.Info, Key: "*", Layer: "decl", Confidence: finding.Certain}
	warn := 0
	for _, v := range vs {
		if v.Severity == finding.Warning {
			f.Severity = finding.Warning
			warn++
		}
		f.Evidence = append(f.Evidence, finding.Evidence{Source: v.Severity, Text: v.Claim})
	}
	f.Claim = Tf("%d enabled mods do not list %s", len(vs), mm)
	if prev := app.PrevMinor(mm); prev != "" {
		f.Claim += Tf(" (%d list %s, %d older only)", len(vs)-warn, prev, warn)
	}
	return &app.Problem{Finding: f, Ref: "p:version"}
}

// skipRow folds harmony.skip-prefix into one expandable row (decisions-m4 §3).
func skipRow(vs []*app.Problem) *app.Problem {
	f := &finding.Finding{Code: "harmony.skip-prefix", Severity: vs[0].Severity, Key: "*", Layer: "harmony", Confidence: finding.Possible}
	mods := map[string]bool{}
	for _, v := range vs {
		if len(v.Subjects) > 0 {
			mods[v.Subjects[0]] = true
		}
		f.Evidence = append(f.Evidence, finding.Evidence{Source: v.Severity, Text: v.Claim})
	}
	f.Claim = Tf("%d bool prefixes from %d mods can skip methods other mods also patch", len(vs), len(mods))
	return &app.Problem{Finding: f, Ref: "p:skip-prefix"}
}

// chooseLog picks Player.log or Player-prev.log; its clusters join the Problems view as
// log.* problems (06 §4, §7.3).
func (m *model) chooseLog() tea.Cmd {
	if m.st == nil {
		return nil
	}
	cur, prev := m.app.LogPaths(m.st)
	if cur == "" {
		m.status = T("no Player.log found")
		return nil
	}
	p := &panel{title: T("analyze which log? (enter)"), choices: []string{cur}}
	if prev != "" {
		p.choices = append(p.choices, prev)
	}
	p.onPick = func(m *model, path string) tea.Cmd {
		if err := m.app.SelectLog(m.st, path); err != nil {
			m.openPanel(T("error: "), err.Error(), nil)
			return nil
		}
		m.newLog = false
		m.tab = tabProblems
		m.prob.cursor, m.prob.off = 0, 0
		m.loadProblems()
		n, stale := 0, false
		for _, x := range m.prob.list {
			switch {
			case x.Ref == staleLogRef:
				n, stale = n+len(x.Evidence), true
			case x.Log != nil:
				n, stale = n+1, stale || x.Log.Stale()
			}
		}
		m.status = Tf("log analyzed: %d problems", n)
		if stale {
			m.status = Tf("log analyzed: %d problems from a stale log, not counted", n)
		}
		return m.ensureProblems()
	}
	m.panel = p
	return nil
}

const staleLogRef = "p:log-stale"

// staleLogRow folds a stale log's problems into one expandable row (03 §6.3, 06 §4).
func staleLogRow(r *app.LogRun, ps []*app.Problem) *app.Problem {
	f := &finding.Finding{Code: "log.stale", Severity: finding.Info, Key: r.Path, Layer: "log", Confidence: finding.Certain,
		Suggestions: []string{"archo problems --code log --all"}}
	n := map[string]int{}
	for _, p := range ps {
		n[p.Severity]++
		if sevOrder(p.Severity) < sevOrder(f.Severity) {
			f.Severity = p.Severity
		}
		f.Evidence = append(f.Evidence, finding.Evidence{Source: p.Severity, Text: p.Claim})
	}
	f.Claim = Tf("%d error, %d warning from this log; not counted", n[finding.Error], n[finding.Warning])
	return &app.Problem{Finding: f, Ref: staleLogRef, Log: r}
}

func sevOrder(s string) int {
	switch s {
	case finding.Error:
		return 0
	case finding.Warning:
		return 1
	}
	return 2
}

// logHeading names a log group: file, game version, time, and why it is stale.
func (m *model) logHeading(r *app.LogRun) string {
	s := T("log") + " " + r.Label
	if v := strings.Fields(r.Version); len(v) > 0 {
		s += " " + v[0]
	}
	s += " · " + m.app.FormatTime(r.ModTime)
	if r.OtherGame {
		s += " · " + Tf("stale: written by %s, the game is %s", r.MM, r.EnvMM)
	} else if r.Before {
		s += " · " + T("stale: older than the Loadout's last change")
	} else {
		s += " · " + T("current")
	}
	return s
}
