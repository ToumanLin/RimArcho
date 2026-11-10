package tui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmdline"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// The command line (16 §9.4): `:` opens an input on the last line that runs one archo
// command as the CLI would — the same command table and the same text (cmds.Execute) — in
// the background on its own database session. tab completes the word under the cursor,
// ↑↓ go through the history, a usage error keeps the line open with the error above it.

const (
	cmdHistKey  = "tui.cmdline.history" // meta key: the history, a JSON array, oldest first
	cmdCandRows = 8                     // candidate rows shown above the input
)

// sCursor is the command line's cursor: the cell under it in reverse video.
var sCursor = lipgloss.NewStyle().Reverse(true)

// cmdLine is the command line's state. It outlives closing: an unsent line comes back
// when `:` opens again (16 §3.1 rule 8).
type cmdLine struct {
	text []rune
	cur  int // cursor, a rune index into text

	hist   []string // oldest first; loaded on first open
	loaded bool
	hi     int    // the history entry shown; len(hist) is the line being typed
	draft  []rune // the line being typed while ↑↓ show the history

	cands    []cmdCand // the candidate list; nil when closed
	sel      int
	from, to int    // the runes the shown candidate replaced
	orig     []rune // what was there before completing (esc puts it back)

	err  string // the last usage error, red; cleared by the next edit
	info string // a note such as "nothing to complete"
}

// cmdCand is one completion.
type cmdCand struct {
	insert string // replaces the word, quoted when it has a space (quoteArg)
	label  string // shown in the list
	note   string // shown dim after the label
	open   bool   // the word may go on (no space after it)
}

// cmdDoneMsg: a command run from the command line ended.
type cmdDoneMsg struct {
	line        string // as typed, without a leading "archo"
	argv        []string
	out         string
	code        int
	uninstalled bool
}

func cmdArgv(line []rune) []string                    { return cmdline.Argv(string(line)) }
func cmdEnv(argv []string) (string, []string, string) { return cmdline.Env(argv) }

// openCmdLine is `:`.
func (m *model) openCmdLine() tea.Cmd {
	if m.cmdl == nil {
		m.cmdl = &cmdLine{}
	}
	l := m.cmdl
	if !l.loaded {
		l.loaded = true
		l.hist = cmdline.DecodeHistory(m.app.Store.Meta(cmdHistKey))
	}
	l.hi, l.draft, l.cands, l.err, l.info = len(l.hist), nil, nil, "", ""
	m.cmdOn = true
	return nil
}

// cmdKey: every key while the command line is open (16 §9.4).
func (m *model) cmdKey(k tea.KeyMsg) tea.Cmd {
	l := m.cmdl
	s := k.String()
	if l.cands != nil {
		switch s {
		case "tab", "down", "ctrl+n":
			l.pick(1)
			return nil
		case "shift+tab", "up", "ctrl+p":
			l.pick(-1)
			return nil
		case "esc":
			l.restore()
			return nil
		case "enter":
			l.cands = nil // takes the shown candidate; the line does not run
			return nil
		}
		l.cands = nil // any other key takes the shown candidate and then does its own thing
	}
	switch s {
	case "esc":
		m.cmdOn = false
		return nil
	case "enter":
		return m.cmdSubmit()
	case "tab":
		m.cmdComplete(1)
		return nil
	case "shift+tab":
		m.cmdComplete(-1)
		return nil
	case "up":
		l.histStep(-1)
		return nil
	case "down":
		l.histStep(1)
		return nil
	case "left":
		l.cur = max(l.cur-1, 0)
	case "right":
		l.cur = min(l.cur+1, len(l.text))
	case "home":
		l.cur = 0
	case "end":
		l.cur = len(l.text)
	case "ctrl+left":
		l.cur = l.wordStart(l.cur)
	case "ctrl+right":
		l.cur = l.wordEnd(l.cur)
	case "backspace":
		if l.cur > 0 {
			l.edit(l.cur-1, l.cur, nil)
		}
	case "delete":
		if l.cur < len(l.text) {
			l.edit(l.cur, l.cur+1, nil)
		}
	case "ctrl+w":
		l.edit(l.wordStart(l.cur), l.cur, nil)
	case "ctrl+u":
		l.edit(0, len(l.text), nil)
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			r := k.Runes
			if k.Type == tea.KeySpace && len(r) == 0 {
				r = []rune{' '}
			}
			// a pasted line may end in a newline; it is not a submit
			r = []rune(strings.Map(func(c rune) rune {
				if c == '\n' || c == '\r' || c == '\t' {
					return ' '
				}
				return c
			}, string(r)))
			l.edit(l.cur, l.cur, r)
		}
	}
	return nil
}

// edit replaces text[from:to] with r and puts the cursor after it; it is a change of the
// line, so the error, the note and the history position go.
func (l *cmdLine) edit(from, to int, r []rune) {
	l.text = slices.Concat(l.text[:from], r, l.text[to:])
	l.cur = from + len(r)
	l.err, l.info = "", ""
	l.hi = len(l.hist)
}

func (l *cmdLine) wordStart(i int) int {
	for i > 0 && unicode.IsSpace(l.text[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(l.text[i-1]) {
		i--
	}
	return i
}

func (l *cmdLine) wordEnd(i int) int {
	for i < len(l.text) && unicode.IsSpace(l.text[i]) {
		i++
	}
	for i < len(l.text) && !unicode.IsSpace(l.text[i]) {
		i++
	}
	return i
}

// histStep: ↑ (-1) shows the previous command run, ↓ (+1) the next; past the newest is
// the line being typed.
func (l *cmdLine) histStep(d int) {
	if n := l.hi + d; n < 0 || n > len(l.hist) {
		return
	}
	at, text, draft := cmdline.Step(l.hist, l.hi, d, string(l.text), string(l.draft))
	l.text, l.draft = []rune(text), []rune(draft)
	l.hi = at
	l.cur, l.err, l.info = len(l.text), "", ""
}

// remember puts a line that ran at the end of the history (an earlier copy goes) and
// saves the newest cmdline.HistoryLimit.
func (m *model) remember(line string) {
	l := m.cmdl
	l.hist = cmdline.Remember(l.hist, line)
	l.hi = len(l.hist)
	if b, err := json.Marshal(l.hist); err == nil {
		_ = m.app.Store.SetMeta(cmdHistKey, string(b))
	}
}

// pick shows the next (d 1) or previous (d -1) candidate in the line.
func (l *cmdLine) pick(d int) {
	n := len(l.cands)
	l.sel = (l.sel + d + n) % n
	l.show()
}

// show puts the selected candidate in place of the word.
func (l *cmdLine) show() {
	c := l.cands[l.sel]
	ins := []rune(c.insert)
	if !c.open && (l.to >= len(l.text) || !unicode.IsSpace(l.text[l.to])) {
		ins = append(ins, ' ')
	}
	l.text = slices.Concat(l.text[:l.from], ins, l.text[l.to:])
	l.to = l.from + len(ins)
	l.cur = l.to
}

// restore is esc in the candidate list: the word as it was before tab.
func (l *cmdLine) restore() {
	l.text = slices.Concat(l.text[:l.from], l.orig, l.text[l.to:])
	l.cur = l.from + len(l.orig)
	l.cands = nil
}

// cmdComplete is tab (d 1) or shift+tab (d -1) with the list closed: one candidate goes
// in, several open the list with the first (or the last) one shown.
func (m *model) cmdComplete(d int) {
	l := m.cmdl
	cands, from, to := m.cmdCands()
	if len(cands) == 0 {
		l.info = T("nothing to complete")
		return
	}
	l.from, l.to, l.orig = from, to, slices.Clone(l.text[from:to])
	l.sel, l.info = 0, ""
	if d < 0 {
		l.sel = len(cands) - 1
	}
	l.cands = cands
	l.show()
	if len(cands) == 1 {
		l.cands = nil
	}
}

// cmdCands uses the same registry/object completions as the GUI.
func (m *model) cmdCands() ([]cmdCand, int, int) {
	c := cmdline.Context{App: m.app}
	mod := func(it *item) cmdline.Mod { return cmdline.Mod{ID: it.id, Ref: it.ref, Name: it.name, Package: it.pkg} }
	for k := range m.cols {
		for i := range m.cols[k].items {
			c.Mods = append(c.Mods, mod(&m.cols[k].items[i]))
		}
	}
	if it := m.current(); it != nil {
		x := mod(it)
		c.Current = &x
	}
	for _, it := range m.selection() {
		c.Selected = append(c.Selected, mod(it))
	}
	cs, from, to := cmdline.Complete(c, string(m.cmdl.text), m.cmdl.cur)
	out := make([]cmdCand, 0, len(cs))
	for _, x := range cs {
		out = append(out, cmdCand{x.Insert, x.Label, x.Note, x.Open})
	}
	return out, from, to
}

// cmdSubmit is enter: check the line, then run it in the background (16 §9.4).
func (m *model) cmdSubmit() tea.Cmd {
	l := m.cmdl
	argv := cmdArgv(l.text)
	if len(argv) == 0 {
		m.cmdOn = false
		return nil
	}
	if m.cmdBusy != "" {
		l.err = T("the last command is still running")
		return nil
	}
	envName, argv, bad := cmdEnv(argv)
	if bad == "" {
		if err := cmds.Check(argv); err != nil {
			bad = err.Error()
		}
	}
	if bad != "" {
		l.err = strings.ReplaceAll(bad, "\n", "   ")
		return nil
	}
	line := strings.TrimSpace(string(l.text))
	if rest, ok := strings.CutPrefix(line, "archo "); ok {
		line = strings.TrimSpace(rest)
	}
	if argv[0] == "log" && slices.Contains(argv, "--follow") {
		l.err = T("the status line already follows the game; run it in a terminal for a scrolling log")
		return nil
	}
	if argv[0] == "uninstall" {
		options, run, err := cmds.ParseUninstallRequest(argv)
		if err != nil {
			l.err = err.Error()
			return nil
		}
		if run {
			m.cmdOn = false
			l.text, l.cur = nil, 0
			m.remember(line)
			return m.startUninstallFor(options, envName)
		}
	}
	m.cmdOn = false
	l.text, l.cur = nil, 0
	switch {
	case len(argv) == 3 && argv[0] == "source" && argv[1] == "decompile" && argv[2] == "--active":
		m.remember(line)
		return m.queueSourceDecompile(envName)
	case len(argv) == 1 && argv[0] == "scan" && (envName == "" || envName == m.envName):
		m.remember(line)
		return m.startScan() // never scan inside the TUI process (16 §8.2)
	case len(argv) == 1 && argv[0] == "analyze" && (envName == "" || envName == m.envName):
		m.remember(line)
		return m.startAnalyze() // the analysis child (05 §3)
	case len(argv) == 1 && argv[0] == "update":
		m.remember(line)
		return m.selfUpdatePanel() // installs, then restarts this TUI (17 §5.1)
	}
	m.cmdBusy = line // the status line says so while it runs (statusText)
	snap := m.snapshot()
	if envName != "" {
		snap.EnvName = envName
	}
	scan, home := m.opts.Scan, m.app.Home
	return func() tea.Msg {
		done := cmdDoneMsg{line: line, argv: argv}
		switch {
		case len(argv) == 1 && argv[0] == "scan":
			out, err := scan(snap.EnvName)
			done.out = out
			if err != nil {
				done.out, done.code = out+"\nerror: "+err.Error(), cmds.ExitError
			}
			return done
		case argv[0] == "analyze":
			var lines []string
			err := app.ChildAnalyze(home, snap.EnvName, "", gameVersionArg(argv), func(s string) { lines = append(lines, s) })
			done.out = strings.Join(lines, "\n")
			if err != nil {
				done.out, done.code = done.out+"\nerror: "+err.Error(), cmds.ExitError
			}
			return done
		}
		a, err := snap.ReadSession()
		if err != nil {
			done.out, done.code = "error: "+err.Error(), cmds.ExitError
			return done
		}
		defer a.Close()
		done.out, done.code = cmds.Execute(a, argv)
		done.uninstalled = a.Uninstalled
		return done
	}
}

// gameVersionArg is --game-version's value in argv, "" when there is none.
func gameVersionArg(argv []string) string {
	for i, a := range argv {
		if v, ok := strings.CutPrefix(a, "--game-version="); ok {
			return v
		}
		if a == "--game-version" && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

// cmdDone: one line of output goes to the status line, more open a panel; a usage error
// the command found itself brings the line back with the error (16 §9.4).
func (m *model) cmdDone(d cmdDoneMsg) tea.Cmd {
	m.cmdBusy = ""
	if d.uninstalled {
		m.app.Uninstalled = true
	}
	out := strings.TrimRight(d.out, "\n")
	if d.code == cmds.ExitUsage {
		msg := strings.ReplaceAll(strings.TrimPrefix(out, "error: "), "\n", "   ")
		if l := m.cmdl; !m.cmdOn && len(l.text) == 0 {
			l.text = []rune(d.line)
			l.cur, l.err, l.hi = len(l.text), msg, len(l.hist)
			m.cmdOn = true
		} else {
			m.say(sayFail, msg)
		}
		return nil
	}
	m.remember(d.line)
	m.reload()
	m.cmdAfter(d)
	if !strings.Contains(out, "\n") {
		kind := sayOK
		if d.code != cmds.ExitOK {
			kind = sayFail
		}
		if out == "" {
			out = T("done")
		}
		m.say(kind, strings.TrimPrefix(out, "error: "))
	} else {
		m.openPanel("archo "+d.line, out)
		if d.code != cmds.ExitOK {
			m.say(sayFail, T("the command failed"))
		}
	}
	m.detailRef = ""
	return m.requestDetail()
}

// cmdAfter: what a command changed that the screen keeps itself (16 §9.4).
func (m *model) cmdAfter(d cmdDoneMsg) {
	if d.code != cmds.ExitOK || len(d.argv) < 2 {
		return
	}
	switch d.argv[0] + " " + d.argv[1] {
	case "config set":
		if len(d.argv) >= 3 {
			switch d.argv[2] {
			case "ui.lang":
				SetLang(m.app.Store.Meta("cfg:ui.lang"))
				m.nav.key = "" // the navigation's names are in the language
			case "ui.tui.theme":
				SetTheme(m.app.Store.Meta("cfg:ui.tui.theme"))
			case "ui.icons":
				SetIcons(m.app.Store.Meta("cfg:ui.icons"))
			}
		}
	case "env use", "plan use":
		m.app.EnvName, m.app.PlanName = "", ""
		for k := range m.cols {
			m.cols[k].cursor, m.cols[k].off, m.cols[k].filter = 0, 0, ""
		}
		m.reload()
		for k := range m.cols {
			m.refilterCol(colKind(k), "")
		}
		m.prob.loaded = false
	}
}

// cmdLines draws the command line: the candidates (over the body's last rows), the
// explanation line and the input line; the caller puts them over the screen's last lines.
func (m *model) cmdLines() []string {
	l := m.cmdl
	w := m.w
	var out []string
	if n := len(l.cands); n > 0 {
		rows := min(n, cmdCandRows, max(m.h-5, 1))
		off := max(0, min(l.sel-rows/2, n-rows))
		lw := 0
		for _, c := range l.cands[off : off+rows] {
			lw = max(lw, width.StringWidth(c.label))
		}
		lw = min(lw, w/2)
		for i := off; i < off+rows; i++ {
			c := l.cands[i]
			text := fit(c.label, lw)
			note := ""
			if c.note != "" && w-6-lw > 8 {
				note = "  " + sDimS.Render(fit(c.note, w-6-lw))
			}
			if i == l.sel {
				out = append(out, "  "+sAccent.Render(markSel)+" "+sSel.Render(text)+note)
			} else {
				out = append(out, "    "+sText.Render(text)+note)
			}
		}
	}
	out = append(out, m.cmdHint(w), m.cmdInput(w))
	return out
}

// cmdHint is the line above the input: the error, a note, the recognised command's usage
// and summary, or the keys.
func (m *model) cmdHint(w int) string {
	l := m.cmdl
	switch {
	case l.err != "":
		return sErrS.Render(ic.fail) + " " + sErrS.Render(fit(l.err, w-2))
	case l.info != "":
		return sSecond.Render(fit(l.info, w))
	case l.cands != nil:
		return sSecond.Render(fit(fmt.Sprintf("%d / %d", l.sel+1, len(l.cands))+"   tab ↑↓ "+T("another")+"   enter "+T("take it")+"   esc "+T("put the word back"), w))
	}
	if _, argv, _ := cmdEnv(cmdArgv(l.text)); len(argv) > 0 {
		if c, _ := cmds.Find(argv); c != nil {
			u := c.Usage()
			return sText.Render(fit(u, min(width.StringWidth(u), w))) + sSecond.Render(fit("  "+c.Summary, max(w-width.StringWidth(u), 0)))
		}
	}
	keys := "tab " + T("complete") + "   ↑↓ " + T("history") + "   enter " + T("run") + "   esc " + T("close")
	if m.cmdBusy != "" {
		keys = Tf("running archo %s…", m.cmdBusy)
	}
	return sSecond.Render(fit(keys, w))
}

// cmdInput is the input line with its cursor; a long line scrolls to keep the cursor in view.
func (m *model) cmdInput(w int) string {
	l := m.cmdl
	head := markPrompt + " archo "
	room := max(w-width.StringWidth(head)-1, 4)
	start := 0
	for width.StringWidth(string(l.text[start:l.cur])) > room-1 {
		start++
	}
	before := string(l.text[start:l.cur])
	at, after := " ", ""
	if l.cur < len(l.text) {
		at, after = string(l.text[l.cur]), string(l.text[l.cur+1:])
	}
	rest := max(room-width.StringWidth(before)-width.StringWidth(at), 0)
	return sAccent.Render(markPrompt+" ") + sSecond.Render("archo ") + sText.Render(before) + sCursor.Render(at) + sText.Render(width.Truncate(after, rest, ""))
}
