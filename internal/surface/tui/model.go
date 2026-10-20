// Package tui is the terminal interface (06). It holds no business logic: every action is
// a command from internal/cmds, and scans run as a child `archo scan` process so the TUI
// process stays small (M1 decision §6.3).
package tui

import (
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
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
	// OpenURL opens the Workshop page (o); nil means the default browser.
	OpenURL func(string) error
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

// Screens. The main screen is the four columns; the others are opened from it and esc
// returns (06 §4.2).
const (
	tabList = iota
	tabProblems
	tabChanges
)

// panel is a secondary screen: command output, optionally waiting for y/n, or a chooser.
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

// Status line kinds (06 §9).
const (
	sayNone = iota
	sayOK
	sayFail
	sayBusy
	sayInfo
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

	idleSeq    int
	detailRef  string
	detail     []string // technical information (the mod command), when expanded
	detailSeq  int
	detailTech bool

	insp     *inspectView
	inspHist []*inspectView

	prob        probState
	newLog      bool
	analyzing   bool
	analyzeWait func() tea.Msg

	chg chState
	// afterPanel runs once after a confirmed panel command (the Changes view reloads).
	afterPanel func(m *model)

	panel   *panel
	prompt  *prompt
	tags    *tagBar // the tag bar is open (06 §12.3)
	status  string
	sayKind int

	content    map[string]int // content kinds for type: (loaded on first use)
	contentKey string

	dd         *detailData          // the current mod's details beyond the row
	imgs       map[string]*imgEntry // description images by url and width
	imgLoading string               // the image being loaded ("" none)
	thumbShown string               // the mod whose thumbnail may be drawn: the cursor rested on it (06 §6.2)
	thumbSeq   int
	dwell       time.Duration // ui.preview_dwell_ms (06 §12.4: 0, the thumbnail at once)
	previewsOff bool          // ui.previews off: no images at all
	prefetching string        // the thumbnail being made ahead of the cursor
	imgUse      map[string]int
	imgClock    int
	fsecs      *fullSecs  // the full-screen detail's sub-sections (06 §6.3)
	pal        *palette   // the command palette (06 §8), nil when closed
	pick       *pickState // a pick session (06 §12.2)
	prefs      colPrefs   // column widths and wrapping (06 §12.1)
	prefsEnv   string
	resizing   *resizeDrag // a column gap being dragged
	rowMap     [3][]int    // per list column: body line → view index (-1: title or empty), as last drawn
	armed      *armed      // a destructive key pressed once (06 §9)
	images     *imgWriter  // the WezTerm host's output; nil: no inline images
	full       bool        // the full-screen detail
	fullOff    int
	lastLines  []dline

	scanning bool
	quitting bool
}

// New builds the TUI model.
func New(o Options) tea.Model {
	if o.Scan == nil {
		o.Scan = ChildScan
	}
	o.App.LazyLog = true // the newest log is parsed when the Problems view opens
	// Until then the status-bar counts use a stored log report only, never a parse.
	o.App.LogCacheOnly = true
	return &model{opts: o, app: o.App, w: 120, h: 30, focus: cActive, marked: map[string]bool{}, imgs: map[string]*imgEntry{}, imgUse: map[string]int{},
		previewsOff: cmds.Setting(o.App, "ui.previews") == "off", dwell: dwellSetting(o.App)}
}

// dwellSetting reads ui.preview_dwell_ms.
func dwellSetting(a *app.App) time.Duration {
	n, _ := strconv.Atoi(cmds.Setting(a, "ui.preview_dwell_ms"))
	return time.Duration(n) * time.Millisecond
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
	SetIcons(o.App.Store.Meta("cfg:ui.icons"))
	SetColors()
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if o.App.Store.Meta("cfg:ui.mouse") != "off" {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	mdl := New(o).(*model)
	if os.Getenv("TERM_PROGRAM") == "WezTerm" { // inline images in the host only (06 §1.3)
		mdl.images = &imgWriter{Writer: os.Stdout}
		opts = append(opts, tea.WithOutput(mdl.images))
	}
	_, err := tea.NewProgram(mdl, opts...).Run()
	return err
}

// ---- messages ----

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
	m.problemCounts()
	if m.opts.ScanOnStart && m.loadErr == nil && m.neverScanned() {
		return m.startScan()
	}
	return nil
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

// resizeDrag: a mouse drag on the gap right of a list column (06 §12.1).
type resizeDrag struct {
	kind   colKind
	x0, w0 int
}

// armed is a destructive action waiting for its second press (06 §9).
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

// say sets the status line (06 §9); it is cleared by the next key.
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

// runToStatus runs a command (Problems / Changes screens and the command line); success
// shows its first line, failure opens a panel with the full text.
func (m *model) runToStatus(argv ...string) {
	out, code := m.run(argv...)
	if code == cmds.ExitOK {
		m.say(sayOK, m.humanize(firstLine(out)))
		return
	}
	m.openPanel("archo "+strings.Join(argv, " "), out, nil)
}

var idRe = regexp.MustCompile(`#[0-9a-z]{3,8}\b`)

// humanize replaces instance references by mod names and drops the "error: " prefix
// (06 §2.1 rule 6: no #id on screen).
func (m *model) humanize(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "error: ")
	if m.st == nil {
		return s
	}
	return idRe.ReplaceAllStringFunc(s, func(r string) string {
		if mod, err := m.st.Ix.ResolveOne(r); err == nil {
			return mod.DisplayName()
		}
		return r
	})
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

func (m *model) col() *column { return &m.cols[m.focus] }

// current is the item under the focused column's cursor.
func (m *model) current() *item { return m.col().cur() }

func (m *model) curID() string { return m.col().curID() }

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
	return tea.Batch(tick, m.thumbDwell())
}

// thumbDwell: the thumbnail follows the selection at once (06 §12.4); with
// ui.preview_dwell_ms set, it goes when the selection changes and comes back once the
// cursor has rested that long (the old 06 §6.2 behaviour).
func (m *model) thumbDwell() tea.Cmd {
	it := m.current()
	if it == nil || it.id == "" {
		m.thumbShown = ""
		return nil
	}
	if it.id == m.thumbShown {
		return nil
	}
	if m.dwell <= 0 {
		m.thumbShown = it.id
		return nil
	}
	m.thumbShown = ""
	m.thumbSeq++
	seq, id := m.thumbSeq, it.id
	return tea.Tick(m.dwell, func(time.Time) tea.Msg { return thumbTick{seq, id} })
}

type thumbTick struct {
	seq int
	id  string
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
	if lang == "zh" {
		return Tf("%d 个%s", n, T(w))
	}
	if n == 1 {
		return Tf("%d %s", n, w)
	}
	return Tf("%d %ss", n, w)
}
