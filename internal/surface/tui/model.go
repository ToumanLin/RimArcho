// Package tui is the terminal interface (06). It holds no business logic: every action is
// a command from internal/cmds, and scans run as a child `archo scan` process so the TUI
// process stays small (M1 decision §6.3).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Options configure the TUI.
type Options struct {
	App *app.App
	// Scan runs `archo scan` and returns its output. Nil means: start this executable as a
	// child process with the same data directory and environment.
	Scan func(envName string) (string, error)
	// ScanOnStart scans before showing the list if the environment was never scanned.
	ScanOnStart bool
	// Analyze brings the Def/Patch analysis up to date; nil means the `archo analyze`
	// child process (03 §3), so the merged document never lives in the TUI process.
	Analyze AnalyzeRunner
}

// ChildScan runs `<exe> scan --env <name>` as a separate process.
func ChildScan(envName string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	args := []string{"scan"}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	out, err := exec.Command(exe, args...).CombinedOutput()
	return string(out), err
}

const (
	tabList = iota
	tabProblems
	tabChanges
)

// Filters cycled by f.
var filters = []string{"all", "enabled", "disabled", "unloadable"}

// row is one line of the list view.
type row struct {
	pos      int    // 1-based load position; 0 = not enabled
	id       string // instance ID ("" for a missing entry)
	ref      string
	name     string
	source   string
	version  string
	flags    string
	unsaved  bool
	disabled bool // cannot be enabled (residue, unloadable, …)
	lower    string
}

// panel is a popup: command output, optionally waiting for y/n, or a chooser.
type panel struct {
	title   string
	lines   []string
	off     int
	confirm []string // argv to run on "y"
	choices []string // chooser values (env names)
	choice  int
	onPick  func(m *model, value string) tea.Cmd
}

// prompt is an input line: ":" command line, "m" move target, "r" rule.
type prompt struct {
	label  string
	text   string
	submit func(m *model, text string) tea.Cmd
}

type model struct {
	opts Options
	app  *app.App
	w, h int
	tab  int

	envName  string
	gameVer  string
	st       *app.LoadoutState
	loadErr  error
	rows     []row
	view     []int
	cursor   int
	off      int
	marked   map[string]bool
	filter   int
	search   string
	editing  bool // typing a search
	unsaved  int
	changes  int
	question int
	missing  int

	idleSeq   int
	detailRef string
	detail    []string
	detailSeq int

	insp     *inspectView
	inspHist []*inspectView

	prob        probState
	newLog      bool
	analyzing   bool
	analyzeWait func() tea.Msg

	tabText []string
	chg     chState
	// afterPanel runs once after a confirmed panel command (the Changes view reloads).
	afterPanel func(m *model)
	tabOff     int

	panel    *panel
	prompt   *prompt
	status   string
	scanning bool
	quitting bool
}

// New builds the TUI model.
func New(o Options) tea.Model {
	if o.Scan == nil {
		o.Scan = ChildScan
	}
	return &model{opts: o, app: o.App, w: 120, h: 30, marked: map[string]bool{}}
}

// MemoryLimit is the Go heap soft cap of the TUI process (06 §8 T5: idle ≤ 80 MB with
// ≥ 15 MB headroom). The GC works harder near the cap instead of growing the heap.
const MemoryLimit = 40 << 20

// TuneProcess applies the TUI process's memory settings (M3 measurements in
// docs/decisions-m3.md): a soft heap cap, GOGC 50, and a 256 KiB SQLite page cache on the
// single connection (the default 2 MB stays resident in modernc's allocator).
func TuneProcess(a *app.App) {
	debug.SetMemoryLimit(MemoryLimit)
	debug.SetGCPercent(50)
	a.Store.DB.Exec("PRAGMA cache_size=-256")
}

// Run starts the TUI on the terminal.
func Run(o Options) error {
	TuneProcess(o.App)
	SetLang(o.App.Store.Meta("cfg:ui.lang"))
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if o.App.Store.Meta("cfg:ui.mouse") != "off" {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(New(o), opts...).Run()
	return err
}

// ---- messages ----

type loadedMsg struct{}
type scanDoneMsg struct {
	out string
	err error
}
type detailTick struct{ seq int }
type idleTick struct{ seq int }
type detailMsg struct {
	ref   string
	lines []string
}

func (m *model) Init() tea.Cmd {
	m.reload()
	if m.opts.ScanOnStart && m.loadErr == nil && m.neverScanned() {
		return m.startScan()
	}
	return m.requestDetail()
}

func (m *model) neverScanned() bool {
	e, err := m.app.Env()
	if err != nil {
		return false
	}
	sc, _, _ := m.app.LastScan(e.ID)
	return sc == nil
}

// run executes a command line exactly as the CLI would and reloads the state.
func (m *model) run(argv ...string) (string, int) {
	out, code := cmds.Execute(m.app, argv)
	m.reload()
	return out, code
}

// runToStatus runs a command; success shows its first line in the status bar, failure
// opens a panel with the full text.
func (m *model) runToStatus(argv ...string) {
	out, code := m.run(argv...)
	if code == cmds.ExitOK {
		m.status = summary(out)
		return
	}
	m.openPanel("archo "+strings.Join(argv, " "), out, nil)
}

// summary is the first line of a command's output plus its first detail line, e.g.
// "workspace: 4 active, 1 change unsaved (env:main) — added #z3bw Beta at 4".
func summary(s string) string {
	lines := strings.Split(strings.TrimSpace(s), string(rune(10)))
	out := lines[0]
	if len(lines) > 1 && !strings.HasPrefix(lines[1], "next:") {
		out += " — " + strings.Join(strings.Fields(lines[1]), " ")
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (m *model) openPanel(title, text string, confirm []string) {
	m.panel = &panel{title: title, lines: strings.Split(strings.TrimRight(text, "\n"), "\n"), confirm: confirm}
}

// reload rebuilds the rows from the workspace (and syncs it with ModsConfig.xml).
func (m *model) reload() {
	prevID := ""
	if r := m.current(); r != nil {
		prevID = r.id
	}
	st, err := m.app.Loadout()
	m.st, m.loadErr = st, err
	m.rows = m.rows[:0]
	if err != nil {
		m.view = nil
		return
	}
	m.envName, m.gameVer = st.Ix.Env.Name, st.Ix.Env.GameVersion
	chs := st.Changes()
	m.unsaved = len(chs)
	unsaved := map[string]bool{}
	for _, c := range chs {
		unsaved[c.Entry.Key()] = true
	}
	mm := env.MajorMinor(st.Ix.Env.GameVersion)
	inList := map[string]bool{}
	m.missing = 0
	for i, e := range st.List {
		if e.Instance == "" {
			m.missing++
			m.rows = append(m.rows, row{pos: i + 1, ref: "pkg:" + e.Missing, name: e.Missing, flags: "missing", unsaved: unsaved[e.Key()], lower: e.Missing})
			continue
		}
		inList[e.Instance] = true
		if mod := st.Ix.ByID[e.Instance]; mod != nil {
			r := m.modRow(st, mod, mm)
			r.pos, r.unsaved = i+1, unsaved[e.Key()]
			m.rows = append(m.rows, r)
		}
	}
	var rest []row
	for _, mod := range st.Ix.Mods {
		if inList[mod.ID] || mod.Status != identity.Live {
			continue
		}
		rest = append(rest, m.modRow(st, mod, mm))
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].lower < rest[j].lower })
	m.rows = append(m.rows, rest...)
	for id := range m.marked {
		if st.Ix.ByID[id] == nil {
			delete(m.marked, id)
		}
	}
	_, m.newLog = m.app.NewLog(st)
	if qs, err := m.app.Store.Questions(st.Ix.Env.ID, false); err == nil {
		m.question = len(qs)
	}
	if sc, ss, _ := m.app.LastScan(st.Ix.Env.ID); sc != nil && ss != nil {
		m.changes = 0
		for _, n := range ss.Events {
			m.changes += n
		}
	}
	m.refilter(prevID)
}

func (m *model) modRow(st *app.LoadoutState, mod *store.Mod, mm string) row {
	r := row{id: mod.ID, ref: st.Ix.Ref(mod.ID), name: mod.DisplayName(), source: string(mod.Kind)}
	r.lower = strings.ToLower(r.name + " " + mod.PackageID + " " + mod.Rel + " " + mod.Authors)
	sup := strings.Fields(mod.SupportedVersions)
	switch {
	case mm == "" || len(sup) == 0:
		r.version = "-"
	case contains(sup, mm):
		r.version = mm
	default:
		r.version = "!" + sup[len(sup)-1]
	}
	var flags []string
	switch {
	case mod.Unloadable != "":
		flags, r.disabled = append(flags, mod.Unloadable), true
	case mod.Residue:
		flags, r.disabled = append(flags, "residue"), true
	case !st.Reg.Known(mod.ID):
		flags, r.disabled = append(flags, "not loaded by game"), true
	case !st.Reg.Loadable(mod.ID):
		flags, r.disabled = append(flags, "duplicate"), true
	}
	if len(st.Ix.ByLineage[mod.LineageID]) > 1 {
		flags = append(flags, "copies")
	}
	r.flags = strings.Join(flags, " ")
	return r
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// refilter applies search and filter, keeping the cursor on prevID when possible.
func (m *model) refilter(prevID string) {
	m.view = m.view[:0]
	q := strings.ToLower(m.search)
	for i, r := range m.rows {
		switch filters[m.filter] {
		case "enabled":
			if r.pos == 0 {
				continue
			}
		case "disabled":
			if r.pos != 0 {
				continue
			}
		case "unloadable":
			if !r.disabled && r.flags != "missing" {
				continue
			}
		}
		if q != "" && !strings.Contains(r.lower, q) && !strings.Contains(strings.ToLower(r.ref), q) {
			continue
		}
		m.view = append(m.view, i)
	}
	m.cursor = min(m.cursor, max(len(m.view)-1, 0))
	if prevID != "" {
		for i, ri := range m.view {
			if m.rows[ri].id == prevID {
				m.cursor = i
			}
		}
	}
	m.scroll()
}

func (m *model) current() *row {
	if m.cursor < 0 || m.cursor >= len(m.view) {
		return nil
	}
	return &m.rows[m.view[m.cursor]]
}

func (m *model) listHeight() int { return max(m.h-4, 1) }

func (m *model) scroll() {
	p := m.listHeight() - 1 // minus column header
	if m.cursor < m.off {
		m.off = m.cursor
	}
	if m.cursor >= m.off+p {
		m.off = m.cursor - p + 1
	}
	m.off = max(m.off, 0)
}

func (m *model) moveCursor(d int) tea.Cmd {
	m.cursor = max(0, min(m.cursor+d, len(m.view)-1))
	m.scroll()
	return m.requestDetail()
}

// requestDetail debounces the detail pane: it is filled 120 ms after the cursor rests.
func (m *model) requestDetail() tea.Cmd {
	m.detailSeq++
	seq := m.detailSeq
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return detailTick{seq} })
}

func (m *model) loadDetail() tea.Cmd {
	r := m.current()
	if r == nil || r.id == "" || !m.showDetail() {
		m.detailRef, m.detail = "", nil
		return nil
	}
	if r.ref == m.detailRef {
		return nil
	}
	ref := r.ref
	return func() tea.Msg {
		out, _ := cmds.Execute(m.app, []string{"mod", ref})
		return detailMsg{ref: ref, lines: strings.Split(strings.TrimRight(out, "\n"), "\n")}
	}
}

// showDetail: the detail pane is hidden below 100 columns (06 §1, T2).
func (m *model) showDetail() bool { return m.w >= 100 }

func (m *model) startScan() tea.Cmd {
	if m.scanning {
		return nil
	}
	m.scanning = true
	m.status = T("scanning…")
	scan, name := m.opts.Scan, m.envName
	return func() tea.Msg {
		out, err := scan(name)
		return scanDoneMsg{out: out, err: err}
	}
}

func (m *model) selection() []string {
	var refs []string
	for _, r := range m.rows {
		if r.id != "" && m.marked[r.id] {
			refs = append(refs, r.ref)
		}
	}
	if len(refs) == 0 {
		if r := m.current(); r != nil {
			refs = append(refs, r.ref)
		}
	}
	return refs
}

func plural(n int, w string) string {
	if lang == "zh" {
		return fmt.Sprintf("%d 个%s", n, T(w))
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, w)
	}
	return fmt.Sprintf("%d %ss", n, w)
}
