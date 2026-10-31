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
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The command line (06 §9.4): `:` opens an input on the last line that runs one archo
// command as the CLI would — the same command table and the same text (cmds.Execute) — in
// the background on its own database session. tab completes the word under the cursor,
// ↑↓ go through the history, a usage error keeps the line open with the error above it.

const (
	cmdHistKey  = "tui.cmdline.history" // meta key: the history, a JSON array, oldest first
	cmdHistMax  = 50
	cmdCandRows = 8 // candidate rows shown above the input
)

// sCursor is the command line's cursor: the cell under it in reverse video.
var sCursor = lipgloss.NewStyle().Reverse(true)

// cmdLine is the command line's state. It outlives closing: an unsent line comes back
// when `:` opens again (06 §3.1 rule 8).
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

// cmdWord is one argument of the line: its text without quotes and its rune span (quotes
// included).
type cmdWord struct {
	text     string
	from, to int
}

// cmdWords splits a line like a shell, without one's escapes: spaces separate arguments,
// "…" or '…' keep spaces inside one, a backslash is an ordinary character (Windows paths)
// and # is not a comment (05 §3.3). An unclosed quote runs to the end.
func cmdWords(line []rune) []cmdWord {
	var out []cmdWord
	for i := 0; i < len(line); {
		if unicode.IsSpace(line[i]) {
			i++
			continue
		}
		start := i
		var b strings.Builder
		for i < len(line) && !unicode.IsSpace(line[i]) {
			if q := line[i]; q == '"' || q == '\'' {
				for i++; i < len(line) && line[i] != q; i++ {
					b.WriteRune(line[i])
				}
				if i < len(line) {
					i++
				}
				continue
			}
			b.WriteRune(line[i])
			i++
		}
		out = append(out, cmdWord{b.String(), start, i})
	}
	return out
}

// cmdArgv is the line's arguments without a leading "archo".
func cmdArgv(line []rune) []string {
	var argv []string
	for _, w := range cmdWords(line) {
		argv = append(argv, w.text)
	}
	if len(argv) > 0 && argv[0] == "archo" {
		argv = argv[1:]
	}
	return argv
}

// cmdEnv takes --env <name> / --env=<name> out of argv: like the CLI's global flag it
// applies to this command only.
func cmdEnv(argv []string) (env string, rest []string, err string) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return env, append(rest, argv[i:]...), ""
		case a == "--env":
			if i+1 >= len(argv) {
				return "", nil, "--env needs a name"
			}
			env = argv[i+1]
			i++
		case strings.HasPrefix(a, "--env="):
			env = strings.TrimPrefix(a, "--env=")
		default:
			rest = append(rest, a)
		}
	}
	return env, rest, ""
}

// openCmdLine is `:`.
func (m *model) openCmdLine() tea.Cmd {
	if m.cmdl == nil {
		m.cmdl = &cmdLine{}
	}
	l := m.cmdl
	if !l.loaded {
		l.loaded = true
		_ = json.Unmarshal([]byte(m.app.Store.Meta(cmdHistKey)), &l.hist)
	}
	l.hi, l.draft, l.cands, l.err, l.info = len(l.hist), nil, nil, "", ""
	m.cmdOn = true
	return nil
}

// cmdKey: every key while the command line is open (06 §9.4).
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
	n := l.hi + d
	if n < 0 || n > len(l.hist) {
		return
	}
	if l.hi == len(l.hist) {
		l.draft = slices.Clone(l.text)
	}
	l.hi = n
	if n == len(l.hist) {
		l.text = l.draft
	} else {
		l.text = []rune(l.hist[n])
	}
	l.cur, l.err, l.info = len(l.text), "", ""
}

// remember puts a line that ran at the end of the history (an earlier copy goes) and
// saves the newest cmdHistMax.
func (m *model) remember(line string) {
	l := m.cmdl
	l.hist = slices.DeleteFunc(l.hist, func(h string) bool { return h == line })
	l.hist = append(l.hist, line)
	if len(l.hist) > cmdHistMax {
		l.hist = l.hist[len(l.hist)-cmdHistMax:]
	}
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

// quoteArg quotes an argument with a space in it (a list name).
func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, "\"") {
		return "\"" + s + "\""
	}
	return s
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

// cmdCands are the completions of the word under the cursor and its rune span.
func (m *model) cmdCands() ([]cmdCand, int, int) {
	l := m.cmdl
	words := cmdWords(l.text)
	from, to, word := l.cur, l.cur, ""
	var prior []string
	for _, w := range words {
		if w.from > l.cur {
			break // the words after the cursor do not matter
		}
		if w.to < l.cur {
			prior = append(prior, w.text)
		} else {
			from, to, word = w.from, w.to, w.text
		}
	}
	if len(prior) > 0 && prior[0] == "archo" {
		prior = prior[1:]
	}
	var out []cmdCand
	c, rest := cmds.Find(prior)
	if c != nil {
		out = append(out, m.argCands(c, rest, word)...)
	}
	out = append(out, nameCands(prior, word)...)
	return out, from, to
}

// nameCands: the commands that go on from the words before (and, at the first word, those
// with the word anywhere in their name).
func nameCands(prior []string, word string) []cmdCand {
	var pre, sub []cmdCand
	lw := strings.ToLower(word)
	for _, c := range cmds.Commands() {
		parts := strings.Fields(c.Name)
		if len(parts) <= len(prior) || !slices.Equal(parts[:len(prior)], prior) {
			continue
		}
		next := parts[len(prior)]
		cand := cmdCand{insert: strings.Join(parts[len(prior):], " "), label: strings.TrimSpace(c.Name + " " + c.Args), note: c.Summary}
		switch {
		case strings.HasPrefix(next, lw):
			pre = append(pre, cand)
		case lw != "" && len(prior) == 0 && strings.Contains(c.Name, lw):
			sub = append(sub, cand)
		}
	}
	return append(pre, sub...)
}

// argCands: the flags of c (a word starting with -) or the values of the positional
// argument the word is, from the placeholder in c's usage (06 §9.4).
func (m *model) argCands(c *cmds.Command, rest []string, word string) []cmdCand {
	if strings.HasPrefix(word, "-") {
		var out []cmdCand
		for _, f := range append(slices.Clone(c.Flags), cmds.Flag{Name: "env", Value: "<name>", Help: "use another game for this command"}) {
			if !strings.HasPrefix("--"+f.Name, word) {
				continue
			}
			label := "--" + f.Name
			if f.Value != "" {
				label += " " + f.Value
			}
			out = append(out, cmdCand{insert: "--" + f.Name, label: label, note: f.Help})
		}
		return out
	}
	// the positional arguments before the word; a value flag takes the next word
	n := 0
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "--") || a == "--" {
			n++
			continue
		}
		if strings.Contains(a, "=") {
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if name == "env" {
			if i+1 == len(rest) {
				return m.envCands(word) // the word is --env's value
			}
			i++
			continue
		}
		for _, f := range c.Flags {
			if f.Name == name && f.Value != "" {
				if i+1 == len(rest) {
					return nil // the word is this flag's value: not completed
				}
				i++
			}
		}
	}
	tok := argToken(c.Args, n)
	switch {
	case tok == "":
		return nil
	case !strings.Contains(tok, "<") && strings.Contains(tok, "|"):
		return literalCands(strings.Split(tok, "|"), word)
	case isModToken(tok):
		return m.modCands(word, strings.Contains(tok, "…") || strings.Contains(tok, "..."))
	}
	switch tok {
	case "<key>":
		if strings.HasPrefix(c.Name, "config") {
			return m.settingCands(word)
		}
	case "<value>":
		if c.Name == "config set" && n == 1 {
			var out []cmdCand
			for i, v := range cmds.SettingChoices(positional(rest, c)[0]) {
				if strings.HasPrefix(v, word) {
					cand := cmdCand{insert: v, label: v}
					if i == 0 {
						cand.note = T("default")
					}
					out = append(out, cand)
				}
			}
			return out
		}
	case "<kind>":
		if c.Name == "cache clear" {
			return literalCands([]string{"previews", "workshop", "images", "source", "logs"}, word)
		}
	case "<name>", "<old>", "<plan>", "<name|file>":
		switch {
		case c.Name == "env add", c.Name == "plan new":
			return nil // a new name
		case strings.HasPrefix(c.Name, "env "):
			return m.envCands(word)
		case strings.HasPrefix(c.Name, "plan "), c.Name == "test run":
			return m.planCands(word)
		case strings.HasPrefix(c.Name, "pack "):
			return m.packCands(word)
		case strings.HasPrefix(c.Name, "deps "):
			return literalCands([]string{"todds", "steamcmd"}, word)
		}
	}
	return nil
}

// literalCands: the values that start with the word.
func literalCands(values []string, word string) []cmdCand {
	var out []cmdCand
	for _, v := range values {
		if strings.HasPrefix(v, word) {
			out = append(out, cmdCand{insert: v, label: v})
		}
	}
	return out
}

// positional is rest without flags and their values.
func positional(rest []string, c *cmds.Command) []string {
	var out []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
			continue
		}
		if strings.Contains(a, "=") {
			continue
		}
		for _, f := range c.Flags {
			if f.Name == strings.TrimPrefix(a, "--") && f.Value != "" {
				i++
			}
		}
	}
	return out
}

// argToken is the usage placeholder of the n-th positional argument: the first form of
// a usage with alternatives ("a | b"), brackets removed; a repeating last one (<ref…>)
// covers every later position.
func argToken(usage string, n int) string {
	form, _, _ := strings.Cut(usage, " | ")
	toks := strings.Fields(strings.NewReplacer("[", "", "]", "").Replace(form))
	if len(toks) == 0 {
		return ""
	}
	if n < len(toks) {
		return toks[n]
	}
	if last := toks[len(toks)-1]; strings.Contains(last, "…") || strings.Contains(last, "...") {
		return last
	}
	return ""
}

// isModToken: a placeholder for a mod reference (<ref>, <ref…>, <mod>, <A>, <B>), not
// <p:ref>, <src:label> and the like.
func isModToken(tok string) bool {
	if strings.Contains(tok, ":") {
		return false
	}
	return strings.Contains(tok, "ref") || strings.Contains(tok, "<mod>") || tok == "<A>" || tok == "<B>"
}

// modCands: the mods of the current game whose name, package id or reference matches the
// word, inserted as a reference without # (05 §3.1 rule 11). An empty word starts with the
// mod under the cursor and, for a repeating argument, the selection.
func (m *model) modCands(word string, many bool) []cmdCand {
	lw := strings.ToLower(strings.TrimPrefix(word, "#"))
	ref := func(it *item) string { return strings.TrimPrefix(it.ref, "#") }
	cand := func(it *item) cmdCand {
		note := ref(it)
		if it.pkg != "" {
			note += "  " + it.pkg
		}
		return cmdCand{insert: ref(it), label: it.name, note: note}
	}
	var out []cmdCand
	seen := map[string]bool{}
	if lw == "" {
		if sel := m.selection(); many && len(sel) > 1 {
			var refs []string
			for _, it := range sel {
				refs = append(refs, ref(it))
			}
			out = append(out, cmdCand{insert: strings.Join(refs, " "), label: Tf("the %d selected mods", len(sel))})
		}
		if it := m.current(); it != nil && it.id != "" {
			out = append(out, cand(it))
			seen[it.id] = true
		}
	}
	var pre, sub, other []cmdCand
	for k := range m.cols {
		for i := range m.cols[k].items {
			it := &m.cols[k].items[i]
			if it.id == "" || seen[it.id] {
				continue
			}
			seen[it.id] = true
			name := strings.ToLower(it.name)
			switch {
			case strings.HasPrefix(name, lw):
				pre = append(pre, cand(it))
			case strings.Contains(name, lw):
				sub = append(sub, cand(it))
			case strings.Contains(strings.ToLower(it.pkg), lw) || strings.HasPrefix(ref(it), lw):
				other = append(other, cand(it))
			}
		}
	}
	return slices.Concat(out, pre, sub, other)
}

func (m *model) envCands(word string) []cmdCand {
	envs, err := m.app.Store.Envs()
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, e := range envs {
		if strings.HasPrefix(strings.ToLower(e.Name), strings.ToLower(word)) {
			out = append(out, cmdCand{insert: quoteArg(e.Name), label: e.Name})
		}
	}
	return out
}

func (m *model) planCands(word string) []cmdCand {
	rows, err := view.PlanRows(m.app)
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, r := range rows {
		if strings.HasPrefix(strings.ToLower(r.Name), strings.ToLower(strings.Trim(word, "\"'"))) {
			out = append(out, cmdCand{insert: quoteArg(r.Name), label: r.Name})
		}
	}
	return out
}

func (m *model) packCands(word string) []cmdCand {
	rows, _, err := view.PackRows(m.app)
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, r := range rows {
		if strings.HasPrefix(strings.ToLower(r.Name), strings.ToLower(strings.Trim(word, "\"'"))) {
			out = append(out, cmdCand{insert: quoteArg(r.Name), label: r.Name})
		}
	}
	return out
}

func (m *model) settingCands(word string) []cmdCand {
	var out []cmdCand
	for _, k := range cmds.SettingKeys() {
		if strings.HasPrefix(k, word) {
			out = append(out, cmdCand{insert: k, label: k, note: cmds.Setting(m.app, k)})
		}
	}
	return out
}

// cmdSubmit is enter: check the line, then run it in the background (06 §9.4).
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
	case len(argv) == 1 && argv[0] == "scan" && (envName == "" || envName == m.envName):
		m.remember(line)
		return m.startScan() // never scan inside the TUI process (06 §8.2)
	case len(argv) == 1 && argv[0] == "analyze" && (envName == "" || envName == m.envName):
		m.remember(line)
		return m.startAnalyze() // the analysis child (03 §3)
	case len(argv) == 1 && argv[0] == "update":
		m.remember(line)
		return m.selfUpdatePanel() // installs, then restarts this TUI (07 §5.1)
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
// the command found itself brings the line back with the error (06 §9.4).
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

// cmdAfter: what a command changed that the screen keeps itself (06 §9.4).
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
