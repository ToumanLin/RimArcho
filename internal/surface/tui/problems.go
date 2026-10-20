package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Loading 需要处理 (06 §7; the items and the screen are in needs.go), the analysis in a child
// process (A) and the choice of log (l).

type probState struct {
	items       []need
	cursor, off int
	stale       bool
	never       bool
	loaded      bool
	countsKey   string
	// status is each mod's row icon and need the top bar's number: the counted items of
	// 需要处理 (06 §3, §4.2, §7).
	status map[string]int
	need   int
	open   map[string]bool // folded items shown open (enter), by text
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

// loadProblems reads the problems (declaration layer now, Def/Patch layers as stored) and
// builds the items of 需要处理 from them (06 §7).
func (m *model) loadProblems() {
	ps, err := m.app.Problems(false, nil)
	p := &m.prob
	p.loaded = true
	p.countsKey = m.app.Store.ChangeKey()
	if err != nil {
		p.items, p.need, p.status = nil, 0, map[string]int{}
		m.say(sayFail, err.Error())
		return
	}
	// The newest log was parsed into this list (decisions-m4 §7.2): it has been seen.
	if m.newLog && m.st != nil {
		if cur, _ := m.app.LogPaths(m.st); cur != "" && !m.app.LazyLog && m.app.ProblemLog(m.st) == cur {
			m.app.SelectLog(m.st, cur)
			m.newLog = false
		}
	}
	p.never = ps.Analysis == nil || ps.Analysis.Run == nil
	p.stale = !p.never && !ps.Analysis.Fresh
	p.items = m.buildNeeds(ps)
	// The number in the top bar is the number of items the player can act on, and a row
	// carries an icon exactly when its mod is in one of them (06 §3, §4.2).
	p.status, p.need = map[string]int{}, 0
	for _, it := range p.items {
		if !it.counted {
			continue
		}
		p.need++
		icons := it.mods
		if !it.fold && len(icons) > 1 {
			icons = icons[:1] // the subject; the others are what it needs or conflicts with
		}
		for _, id := range icons {
			if it.status > p.status[id] {
				p.status[id] = it.status
			}
		}
	}
	p.cursor = min(p.cursor, max(len(p.items)-1, 0))
}

// needCount is the top bar's "needs attention" number (06 §4.2): problems, identity
// questions, an external change of the list, a list saved for another game version.
func (m *model) needCount() int {
	m.problemCounts()
	return m.prob.need
}

// problemCounts reloads the items when the database changed (an edit, a rule, an answer).
func (m *model) problemCounts() {
	if m.loadErr != nil {
		return
	}
	if !m.prob.loaded || m.prob.countsKey != m.app.Store.ChangeKey() {
		m.loadProblems()
	}
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
		m.say(sayOK, T("the log is read"))
		return m.ensureProblems()
	}
	m.panel = p
	return nil
}
