// Package tui is the terminal interface (06). It holds no business logic: every action is
// a command from internal/cmds, and scans run as a child `archo scan` process so the TUI
// process stays small (M1 decision §6.3).
package tui

import (
	"context"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/view"
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
	// child process (05 §3), so the merged document never lives in the TUI process.
	Analyze AnalyzeRunner
	// OpenURL opens the Workshop page (o); nil means the default browser.
	OpenURL func(string) error
	// OpenDir shows a folder (ctrl+o); nil means the file manager.
	OpenDir func(string) error
}

// ChildScan runs `<exe> scan --env <name>` as a separate process (app.ChildScan with the
// data directory of the environment variable or the default).
func ChildScan(envName string) (string, error) { return app.ChildScan(env.AppHome(), envName) }

// Screens. The main screen is the four columns; the others are opened from it and esc
// returns (16 §2.2).
const (
	tabList = iota
	tabProblems
	tabChanges
	tabLibrary   // Mod library (16 §2.9), from the palette
	tabSubscribe // Mod library › add a subscription (16 §2.9): + on updates
	tabSearch    // Mod library › Search files (16 §2.9)
	tabRules     // Load-order rules (16 §2.5): r in the editor
	tabHome      // Start (16 §2.3): shift+1, and the editor's L
)

// panel is a secondary screen: command output to read, or a chooser.
type panel struct {
	title   string
	lines   []string
	off     int
	choices []string // chooser values (env names)
	choice  int
	onPick  func(m *model, value string) tea.Cmd
	// onPickAt: enter on choice i, for choices whose labels may repeat (H's steps)
	onPickAt func(m *model, i int) tea.Cmd
	// onDelete: d / delete on a choice (the environment chooser's remove game, 16 §2.14)
	onDelete func(m *model, value string) tea.Cmd
	// onEnter: enter runs it (a confirmation: the dry run is the text, 16 §6)
	onEnter func(m *model) tea.Cmd
	// keys: a text panel's own keys (Feedback's m / p / l, 16 §2.14); the panel stays open
	keys map[string]func(m *model) tea.Cmd
}

// prompt is an input line: a name, a note, a rule (r), an import source. The command
// line (:) is cmdLine.
type prompt struct {
	label  string
	text   string
	submit func(m *model, text string) tea.Cmd
}

// Status line kinds (16 §6).
const (
	sayNone = iota
	sayOK
	sayFail
	sayBusy
	sayInfo
	sayWarn // needs the player (a lapsed login), not a failure of what they did
)

type model struct {
	opts Options
	app  *app.App
	w, h int
	tab  int

	envName    string
	gameVer    string
	st         *app.LoadoutState
	loadErr    error
	cols       [3]column
	focus      colKind
	marked     map[string]bool
	editing    bool // typing a filter
	unsaved    int
	changes    int
	lastEvents map[string]int // the last scan's events by kind
	question   int
	missing    int

	uninstallRun *uninstallRun
	idleSeq      int
	detailRef    string
	detail       []string // technical information (the mod command), when expanded
	detailSeq    int
	detailTech   bool

	insp     *inspectView
	inspHist []*inspectView

	prob      probState
	newLog    bool
	analyzing bool
	// analyzeQuiet: the check Can it run starts by itself; its head says it runs, and the
	// status line keeps what the player just did (the undo hint, 16 §2.4)
	analyzeQuiet bool
	analyzeWait  func() tea.Msg

	chg          chState
	libr         libState
	add          addState   // add a subscription
	fs           fsState    // Search files
	rules        rulesState // Load-order rules
	home         homeState  // Start
	texturePanel *panel
	textureKey   string
	// launches: the current list when the editor has another one open ("": the editor's
	// list is the current one); the top bar says it (07 §1.3)
	launches   string
	launchesID int64
	tasks      taskRunner // the subscription actions' background tasks (16 §2.16)

	bar      barState  // the editor's filter bar over Not enabled and Enabled (16 §2.5)
	board    *boardRef // the board the columns were built from (the bar filters it)
	boardSeq int
	barCache *view.BarFilter
	barKey   string
	libCache *app.Library // the library as of board build libAt (the rows' ops)
	menu     *rowMenu     // the row menu is open (16 §8.5)
	libAt    int

	liveTr view.LiveTracker
	live   *view.Live // the running game (16 §2.1); nil until the first poll

	hist    []view.HistItem // the history area, shown in empty staging (16 §2.5)
	histCur int
	histOff int

	panel   *panel
	prompt  *prompt
	cmdl    *cmdLine // the command line, `:` (16 §9.4); kept while closed
	cmdOn   bool     // the command line is open
	cmdBusy string   // the command line's command that runs, "" none
	tags    *tagBar  // the tag bar is open (16 §3.8)
	status  string
	sayKind int

	content view.ContentCache // content kinds for type: (loaded on first use)

	dd          *detailData // the current mod's details beyond the row
	fsecs       *fullSecs   // the full-screen detail's sub-sections (16 §4.2)
	pal         *palette    // the command palette (16 §3.5), nil when closed
	sel         selRange    // the shift range of the selection (16 §3.4)
	pos         *posInput   // the position input, m (16 §3.6)
	placed      []string    // the mods space just enabled: an m right after it moves them (16 §3.5)
	prefs       colPrefs    // column widths and wrapping (16 §2.5)
	prefsEnv    string
	resizing    *resizeDrag // a column gap being dragged
	rowMap      [3][]int    // per list column: body line → view index (-1: title or empty), as last drawn
	armed       *armed      // a destructive key pressed once (16 §6)
	full        bool        // the full-screen detail
	fullOff     int
	fullSec     int    // the full-screen sub-section tab / shift+tab went to (0: the summary)
	detailFocus bool   // focus area 4, the compact detail: ↑↓ scroll it (16 §3.2)
	detailOff   int    // its scroll (PgUp PgDn and the wheel scroll it from any area)
	detailOffID string // the mod it was scrolled for
	lastLines   []dline

	scanning bool
	quitting bool

	// selfRun is RimArcho's own update while it installs; restartExe the new program Run
	// starts once the screen has closed (17 §5.1).
	selfRun    *selfUpdate
	restartExe string
}

// New builds the TUI model.
func New(o Options) tea.Model {
	if o.Scan == nil {
		home := o.App.Home
		o.Scan = func(envName string) (string, error) { return app.ChildScan(home, envName) }
	}
	o.App.LazyLog = true // the newest log is parsed when the Problems view opens
	// Until then the status-bar counts use a stored log report only, never a parse.
	o.App.LogCacheOnly = true
	return &model{opts: o, app: o.App, w: 120, h: 30, focus: cActive, marked: map[string]bool{}}
}

// MemoryLimit is the Go heap soft cap of the TUI process (16 §3.5 T5: idle ≤ 80 MB with
// ≥ 15 MB headroom). The GC works harder near the cap instead of growing the heap.
const MemoryLimit = 40 << 20

// TuneProcess applies the TUI process's memory settings (M3 measurements in
// 17 §7.3): a soft heap cap, GOGC 50, and a 256 KiB SQLite page cache on the
// single connection (the default 2 MB stays resident in modernc's allocator).
func TuneProcess(a *app.App) {
	debug.SetMemoryLimit(MemoryLimit)
	debug.SetGCPercent(50)
	a.Store.DB.Exec("PRAGMA cache_size=-256")
}

// Run starts the TUI on the terminal.
func Run(o Options) error {
	// Snapshot options before the model starts editing its App. Each maintenance
	// pass owns a separate connection and releases its inventory afterwards.
	maintenance := &app.App{Store: o.App.Store}
	maintenance.ReadOptions(o.App)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	TuneProcess(o.App)
	SetLang(o.App.Store.Meta("cfg:ui.lang"))
	SetIcons(o.App.Store.Meta("cfg:ui.icons"))
	SetColors()
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if o.App.Store.Meta("cfg:ui.mouse") != "off" {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	mdl := New(o).(*model)
	program := tea.NewProgram(mdl, opts...)
	go app.AutomaticLoop(ctx, func(parent context.Context) {
		r, err := maintenance.ReadSession()
		if err != nil {
			return
		}
		defer r.Close()
		checkCtx, stop := context.WithTimeout(parent, 15*time.Minute)
		defer stop()
		r.RunAutomaticCycle(checkCtx, func(kind string, run func() error) {
			before := r.AutomaticStatus(kind)
			_ = run()
			if parent.Err() == nil {
				program.Send(automaticDoneMsg{kind: kind, before: before, after: r.AutomaticStatus(kind)})
			}
		})
	})
	_, err := program.Run()
	// Terminal loss must not interrupt file work.
	if run := mdl.uninstallRun; run != nil {
		<-run.done
	}
	mdl.closeTasks()
	if run := mdl.selfRun; run != nil {
		// Quit during an install: stop it, and let a swap that has begun finish or roll back.
		run.cancel()
		app.WaitSelfSwap(30 * time.Second)
	}
	if err == nil && mdl.restartExe != "" {
		return &Restart{Exe: mdl.restartExe}
	}
	return err
}

// Restart is Run's answer after RimArcho updated itself: the caller closes the database
// and starts Exe in this terminal with the same arguments (17 §5.1).
type Restart struct{ Exe string }

func (r *Restart) Error() string { return "restart " + r.Exe }

// ---- messages ----

type scanDoneMsg struct {
	out string
	err error
}
type detailTick struct{ seq int }
type automaticDoneMsg struct {
	kind          string
	before, after app.AutomaticStatus
}
type idleTick struct{ seq int }
type detailMsg struct {
	ref   string
	lines []string
}

func (m *model) Init() tea.Cmd {
	m.reload()
	m.problemCounts()
	if m.promptDirectories() {
		return liveNext()
	}
	if m.opts.ScanOnStart && m.loadErr == nil && m.neverScanned() {
		return tea.Batch(m.startScan(), liveNext())
	}
	return liveNext()
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

// boardRef is the shared board of the columns and its build number.
type boardRef struct {
	b   *view.Board
	seq int
}

// resizeDrag: a mouse drag on the gap right of a list column (16 §2.5).
type resizeDrag struct {
	kind   colKind
	x0, w0 int
}

// armed is a destructive action waiting for its second press (16 §6).
type armed struct {
	key string
	run func(m *model) tea.Cmd
}

// arm: the first press of a destructive key only says what the second press will do; the
// same key again does it, any other key cancels.
func (m *model) arm(key, warning string, run func(m *model) tea.Cmd) tea.Cmd {
	m.armed = &armed{key: key, run: run}
	m.say(sayFail, warning)
	return nil
}

// say sets the status line (16 §6); it is cleared by the next key.
func (m *model) say(kind int, text string) { m.sayKind, m.status = kind, text }

// act runs a command; success shows okText, failure the command's error in plain words.
func (m *model) act(okText string, argv ...string) bool {
	out, code := m.run(argv...)
	if code == cmds.ExitOK {
		m.say(sayOK, okText)
		return true
	}
	m.say(sayFail, m.humanize(firstLine(out)))
	return false
}

// humanize replaces instance references by mod names and drops the "error: " prefix.
func (m *model) humanize(s string) string { return view.Humanize(m.st, s) }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (m *model) openPanel(title, text string) {
	m.panel = &panel{title: title, lines: strings.Split(strings.TrimRight(text, "\n"), "\n")}
}

func (m *model) col() *column { return &m.cols[m.focus] }

// current is the item under the focused column's cursor.
func (m *model) current() *item { return m.col().cur() }

func (m *model) moveCursor(d int) tea.Cmd {
	c := m.col()
	c.cursor = max(0, min(c.cursor+d, len(c.view)-1))
	c.scroll(m.rowsShown() - 1)
	return m.requestDetail()
}

// requestDetail debounces the technical information: loaded 120 ms after the cursor rests,
// and only while it is expanded.
func (m *model) requestDetail() tea.Cmd {
	m.detailSeq++
	seq := m.detailSeq
	tick := tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return detailTick{seq} })
	return tick
}

func (m *model) loadDetail() tea.Cmd {
	it := m.current()
	if it == nil || it.id == "" || !m.detailTech || (m.layout().detailW == 0 && !m.full) {
		m.detailRef, m.detail = "", nil
		return nil
	}
	if it.ref == m.detailRef {
		return nil
	}
	ref := it.ref
	return func() tea.Msg {
		out, _ := cmds.Execute(m.app, []string{"mod", ref})
		return detailMsg{ref: ref, lines: strings.Split(strings.TrimRight(out, "\n"), "\n")}
	}
}

func (m *model) startScan() tea.Cmd {
	if m.scanning {
		return nil
	}
	m.scanning = true
	m.tasks.scanning.Store(true)
	m.say(sayBusy, T("scanning…"))
	scan, name := m.opts.Scan, m.envName
	return func() tea.Msg {
		out, err := scan(name)
		return scanDoneMsg{out: out, err: err}
	}
}

// selection: the marked mods of the focused column, else the one under the cursor.
func (m *model) selection() []*item {
	c := m.col()
	var out []*item
	for i := range c.items {
		if it := &c.items[i]; it.id != "" && m.marked[it.id] {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		if it := c.cur(); it != nil {
			out = append(out, it)
		}
	}
	return out
}

func refsOf(its []*item) []string {
	var out []string
	for _, it := range its {
		out = append(out, it.ref)
	}
	return out
}

// named: "Harmony" for one mod, "3 mods" for several.
func named(its []*item) string {
	if len(its) == 1 {
		return its[0].name
	}
	return plural(len(its), "mod")
}

func plural(n int, w string) string {
	return view.Plural(n, w)
}
