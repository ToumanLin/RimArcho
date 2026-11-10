package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Loading Can it run (16 §5; the items and the screen are in needs.go), the analysis in a child
// process (A) and the choice of log (l).

type probState struct {
	seq           int
	reading       bool
	readKey       string
	logKey        string
	memoryReading bool
	memoryAt      time.Time
	items         []need
	cursor, off   int
	stale         bool
	never         bool
	loaded        bool
	countsKey     string
	// status is each mod's row icon and need the top bar's number: the counted items of
	// Can it run (16 §7, §4.2, §7).
	status map[string]int
	need   int
	open   map[string]bool // folded items shown open (enter), by text
	base   []view.Need     // the analysis' items; the running game's lead them (16 §2.1)
	// lastRun says which run the last-run section is from ("the list · Sep 29 19:41")
	lastRun string
	// lastSave is Before you start's last-save line (view.Needs.LastSave); nil none.
	lastSave *view.LastSaveView
	// elsewhere is what this page leaves to another page (view.Needs.Elsewhere, 16 §2.4)
	elsewhere []view.Elsewhere
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
	m.prob.seq++
	m.prob.reading = true
	seq, scope, key := m.prob.seq, m.pageScope(), m.pageKey()
	readKey, logKey := m.prob.readKey, m.prob.logKey
	// Parsing the log can itself write a report. Local edits change the scope;
	// a worker's own report must not invalidate its answer.
	valid := func(m *model) bool {
		if m.tab != tabProblems || m.prob.seq != seq || m.pageScope() != scope {
			return false
		}
		m.prob.reading = false
		return true
	}
	return m.bgWhen(valid, func(a *app.App) func(*model) tea.Cmd {
		st, err := a.Loadout()
		logPath, stamp := "", ""
		if err == nil && st != nil {
			logPath = a.ProblemLog(st)
			stamp = logPath
			if fi, e := os.Stat(logPath); e == nil {
				stamp = fmt.Sprintf("%s|%d|%d", logPath, fi.Size(), fi.ModTime().UnixNano())
			}
		}
		if err == nil && readKey == key && logKey == stamp {
			return func(m *model) tea.Cmd { return m.checkProblems() }
		}
		var vn *view.Needs
		if err == nil {
			ps, e := a.Problems(false, nil)
			err = e
			if err == nil {
				vn = view.BuildNeeds(a, st, ps)
			}
		}
		return func(m *model) tea.Cmd {
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
				return nil
			}
			m.setNeeds(vn)
			m.prob.readKey, m.prob.logKey = key, stamp
			if m.newLog && m.st != nil {
				if cur, _ := m.app.LogPaths(m.st); cur != "" && logPath == cur {
					m.app.SelectLog(m.st, cur)
					m.newLog = false
				}
			}
			return m.checkProblems()
		}
	})
}

func (m *model) checkProblems() tea.Cmd {
	if (m.prob.stale || m.prob.never) && !m.analyzing {
		before := m.status
		cmd := m.startAnalyze()
		m.analyzeQuiet, m.status = true, before
		return tea.Batch(cmd, m.startMemoryEstimate(false))
	}
	return m.startMemoryEstimate(false)
}

// loadProblems reads the problems (declaration layer now, Def/Patch layers as stored) and
// builds the items of Can it run from them (16 §5).
func (m *model) loadProblems() {
	ps, err := m.app.Problems(false, nil)
	p := &m.prob
	p.loaded = true
	p.countsKey = m.app.Store.ChangeKey()
	if err != nil {
		p.base, p.items, p.need, p.status, p.lastRun, p.elsewhere = nil, nil, 0, map[string]int{}, "", nil
		m.say(sayFail, err.Error())
		return
	}
	// The newest log was parsed into this list (06 §4): it has been seen.
	if m.newLog && m.st != nil {
		if cur, _ := m.app.LogPaths(m.st); cur != "" && !m.app.LazyLog && m.app.ProblemLog(m.st) == cur {
			m.app.SelectLog(m.st, cur)
			m.newLog = false
		}
	}
	vn := view.BuildNeeds(m.app, m.st, ps)
	m.setNeeds(vn)
	// The initial cache-only read is complete when there is no log. Opening the
	// page still checks its log path/mtime on the worker before reusing these items.
	p.readKey, p.logKey = m.pageKey(), ""
}

func (m *model) setNeeds(vn *view.Needs) {
	p := &m.prob
	p.loaded, p.countsKey = true, m.app.Store.ChangeKey()
	p.never, p.stale = vn.Never, vn.Stale
	p.base, p.status, p.need, p.lastRun, p.elsewhere = vn.Items, vn.Status, vn.Count, vn.LastRun, vn.Elsewhere
	p.lastSave = vn.LastSave
	m.composeNeeds()
}

// composeNeeds puts the running game's errors (group "this run") before the analysis' items.
func (m *model) composeNeeds() {
	p := &m.prob
	items := p.base
	if m.live != nil && len(m.live.Items) > 0 {
		items = append(append([]view.Need{}, m.live.Items...), p.base...)
	}
	p.items = m.toNeeds(items)
	p.cursor = min(p.cursor, max(len(p.items)-1, 0))
}

// needCount is the top bar's "Can it run N" number (16 §2.2): problems, identity
// questions, an external change of the list, a list saved for another game version, and
// the running game's errors, so it equals the counted items of the screen (16 §2.4).
func (m *model) needCount() int {
	m.problemCounts()
	n := m.prob.need
	if m.live != nil {
		for _, it := range m.live.Items {
			if it.Counted {
				n++
			}
		}
	}
	return n
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
	m.analyzing, m.analyzeQuiet = true, false
	m.status = T("analyzing…")
	run, name := m.opts.Analyze, m.envName
	if run == nil {
		home := m.app.Home
		run = func(env string, progress func(string)) error { return app.ChildAnalyze(home, env, "", "", progress) }
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
	wait := func() tea.Msg {
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
		if !m.analyzeQuiet {
			m.status = T("analyzing…") + " " + strings.TrimPrefix(msg.line, "progress: ")
		}
		return m.analyzeWait
	case analyzeDoneMsg:
		m.analyzing = false
		if msg.err != nil {
			m.openPanel(T("analysis failed"), msg.err.Error())
		} else if !m.analyzeQuiet {
			m.status = T("analysis done")
		}
		m.reload()
		m.loadProblems()
	}
	return nil
}

// chooseLog picks Player.log or Player-prev.log; its clusters join the Problems view as
// log.* problems (16 §2, §7.3).
func (m *model) chooseLog() tea.Cmd {
	if m.st == nil {
		return nil
	}
	cur, prev := m.app.LogPaths(m.st)
	if cur == "" {
		cur, prev = prev, ""
	}
	if cur == "" {
		m.status = T("no Player.log found")
		return nil
	}
	p := &panel{title: T("analyze which log? (enter)"), choices: []string{cur}, nav: 3}
	if prev != "" {
		p.choices = append(p.choices, prev)
	}
	p.onPick = func(m *model, path string) tea.Cmd {
		if err := m.app.SelectLog(m.st, path); err != nil {
			m.openPanel(T("error: "), err.Error())
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
