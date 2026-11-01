package tui

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod library › Search files (06 §2.9, §9.1): the window's page in the terminal, on the same
// view.FileSearch. Area 1 is the query: the box, then the kind row (files / C# code and the
// file kinds) and the scope row (the open list's mods, or the mods named with + Mod); ↑↓
// go between the rows, ←→ between a row's chips, space or enter switches one. Area 2 is
// the results: ↑↓ move over the files and lines, enter or ctrl+o opens the file's folder,
// [ ] turn the page. A new search, esc while one runs, and leaving the page cancel it
// (06 §3.1 rule 9). The terminal selects text itself, so there is no ctrl+c copy (06 §3.1
// rule 12).

// fsDefaults are the kinds searched when the player picked none (FileSearchReq.Kinds empty).
var fsDefaults = []string{"defs", "patches", "about"}

// fsTick refreshes a running search's progress (06 §2.16.1 G17).
const fsTick = 300 * time.Millisecond

type fsTickMsg struct{ seq int }

// fsRow is one line of the results: a mod's head (no cursor), a file, or a matching line.
type fsRow struct {
	mod     int // index in the page's Mods
	file    int // -1 for the mod's head
	line    int // -1 for the file's row
	cursor  bool
	dirPath string // the file the row opens the folder of
	modID   string
}

type fsState struct {
	text                     string
	code, regex, cs, wd, def bool
	kinds                    map[string]bool
	mods                     []view.RuleMod // named mods; empty: the open list's enabled mods
	info                     string         // the default scope in words

	results bool // area 2 has the focus
	row     int  // area 1: 0 the box, 1 the kinds, 2 the scope
	chip    int  // the chip under the cursor on rows 1 and 2

	picking bool // + Mod: the name box and its candidates
	pick    string
	cands   []view.RuleMod
	candAt  int

	page   *view.FileSearchPage
	at     view.FileSearchReq // the request page answers
	rows   []fsRow
	cursor int // index in rows (a row with cursor)
	off    int

	running  bool
	seq      int
	cancel   context.CancelFunc
	progress *fsProgress
}

// fsProgress is the decompiling's progress, written by the search's goroutine.
type fsProgress struct {
	mu sync.Mutex
	s  string
}

func (p *fsProgress) set(s string) { p.mu.Lock(); p.s = s; p.mu.Unlock() }
func (p *fsProgress) get() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.s
}

// openSearch shows Search files, its box focused; what was typed before is kept.
func (m *model) openSearch() tea.Cmd {
	m.panel, m.insp, m.full = nil, nil, false
	s := &m.fs
	if s.kinds == nil {
		s.kinds = map[string]bool{}
		for _, k := range fsDefaults {
			s.kinds[k] = true
		}
	}
	m.tab = tabSearch
	s.results, s.row, s.chip, s.picking = false, 0, 0, false
	if info, err := view.FileSearchScope(m.app); err == nil {
		s.info = info
	}
	return nil
}

// fsLeft: the page was left; its search is no longer wanted.
func (m *model) fsLeft() {
	if m.tab != tabSearch && m.fs.running {
		m.fsStop(false)
	}
}

// fsRequest is the page's question for page n (the window's request()).
func (s *fsState) request(n int) view.FileSearchReq {
	q := view.FileSearchReq{Pattern: strings.TrimSpace(s.text), Code: s.code, Regex: s.regex, Page: n}
	if !s.code {
		q.Case, q.Word, q.Def = s.cs || s.def, s.wd || s.def, s.def
		defaults := len(s.kinds) == len(fsDefaults)
		for _, k := range fsDefaults {
			defaults = defaults && s.kinds[k]
		}
		if !defaults {
			for _, k := range view.FileSearchKinds {
				if s.kinds[k.Kind] {
					q.Kinds = append(q.Kinds, k.Kind)
				}
			}
		}
	}
	for _, mod := range s.mods {
		q.Mods = append(q.Mods, mod.ID)
	}
	return q
}

// fsRun searches page n in the background; the search before is cancelled and its answer
// dropped.
func (m *model) fsRun(n int) tea.Cmd {
	s := &m.fs
	q := s.request(n)
	refuse := ""
	switch {
	case q.Pattern == "":
		refuse = T("type what to look for")
	case q.Code && len(q.Mods) == 0:
		refuse = T("C# code searches only named mods: add one with + Mod first")
	case !q.Code && len(s.kinds) == 0:
		refuse = T("choose a file kind")
	}
	if refuse != "" {
		if s.running {
			m.fsStop(false)
		}
		m.say(sayFail, refuse)
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.seq++
	seq := s.seq
	s.running, s.cancel, s.progress = true, cancel, &fsProgress{}
	prog := s.progress
	search := m.bg(func(a *app.App) func(m *model) tea.Cmd {
		page, err := view.FileSearch(ctx, a, q, prog.set)
		return func(m *model) tea.Cmd {
			s := &m.fs
			if s.seq != seq {
				return nil
			}
			stopped := ctx.Err() != nil
			cancel()
			s.running, s.cancel = false, nil
			if stopped {
				return nil
			}
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
				return nil
			}
			s.page, s.at = page, q
			s.buildRows()
			s.cursor, s.off = s.firstCursor(), 0
			return nil
		}
	})
	return tea.Batch(search, fsNext(seq))
}

func fsNext(seq int) tea.Cmd {
	return tea.Tick(fsTick, func(time.Time) tea.Msg { return fsTickMsg{seq} })
}

// fsStop is esc while a search runs (and leaving the page).
func (m *model) fsStop(announce bool) {
	s := &m.fs
	s.seq++
	if s.cancel != nil {
		s.cancel()
	}
	s.running, s.cancel = false, nil
	if announce {
		m.say(sayOK, T("stopped"))
	}
}

// buildRows flattens the page: each mod's head, then its files and their lines.
func (s *fsState) buildRows() {
	s.rows = s.rows[:0]
	if s.page == nil {
		return
	}
	for mi, mod := range s.page.Mods {
		s.rows = append(s.rows, fsRow{mod: mi, file: -1, line: -1, modID: mod.ID})
		for fi, f := range mod.Files {
			s.rows = append(s.rows, fsRow{mod: mi, file: fi, line: -1, cursor: true, dirPath: f.Path, modID: mod.ID})
			for li := range f.Lines {
				s.rows = append(s.rows, fsRow{mod: mi, file: fi, line: li, cursor: true, dirPath: f.Path, modID: mod.ID})
			}
		}
	}
}

func (s *fsState) firstCursor() int {
	for i, r := range s.rows {
		if r.cursor {
			return i
		}
	}
	return 0
}

// step moves the results' cursor d rows that take one (06 §3.1 rule 1: it only moves).
func (s *fsState) step(d int) {
	if len(s.rows) == 0 {
		return
	}
	i := s.cursor
	dir := 1
	if d < 0 {
		dir, d = -1, -d
	}
	for ; d > 0; d-- {
		j := i + dir
		for j >= 0 && j < len(s.rows) && !s.rows[j].cursor {
			j += dir
		}
		if j < 0 || j >= len(s.rows) {
			break
		}
		i = j
	}
	s.cursor = i
}

// ---- keys ----

// fsTyping: the box has the keys (letters go into it, 06 §2.9).
func (m *model) fsTyping() bool {
	s := &m.fs
	return m.tab == tabSearch && (s.picking || !s.results && s.row == 0)
}

// fsOption: alt+c alt+w alt+r alt+d switch case, whole word, regex and Def name; alt+1
// alt+2 files and C# code; in either area, as in the window.
func (m *model) fsOption(k string) (tea.Cmd, bool) {
	s := &m.fs
	switch k {
	case "alt+1", "alt+2":
		return m.fsChange(func() { s.code = k == "alt+2" }), true
	case "alt+r":
		return m.fsChange(func() { s.regex = !s.regex }), true
	case "alt+d":
		if s.code {
			return nil, true
		}
		return m.fsChange(func() { s.def = !s.def }), true
	case "alt+c", "alt+w":
		if s.code || s.def {
			return nil, true // Def name decides them; C# never tells case (06 §2.9)
		}
		if k == "alt+c" {
			return m.fsChange(func() { s.cs = !s.cs }), true
		}
		return m.fsChange(func() { s.wd = !s.wd }), true
	}
	return nil, false
}

// fsChange applies an option; with something typed, it searches again.
func (m *model) fsChange(apply func()) tea.Cmd {
	apply()
	s := &m.fs
	s.chip = min(s.chip, len(m.fsChips(s.row))-1)
	if strings.TrimSpace(s.text) != "" {
		return m.fsRun(1)
	}
	if s.running {
		m.fsStop(false)
	}
	return nil
}

// fsTypeKey: the keys while the box (or + Mod's name box) has them.
func (m *model) fsTypeKey(k tea.KeyMsg) tea.Cmd {
	s := &m.fs
	if cmd, ok := m.fsOption(k.String()); ok {
		return cmd
	}
	if s.picking {
		return m.fsPickKey(k)
	}
	switch k.Type {
	case tea.KeyRunes, tea.KeySpace:
		text := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(string(k.Runes))
		if k.Type == tea.KeySpace {
			text = " "
		}
		s.text += text
		return nil
	case tea.KeyBackspace:
		if r := []rune(s.text); len(r) > 0 {
			s.text = string(r[:len(r)-1])
		}
		return nil
	case tea.KeyCtrlU:
		s.text = ""
		return nil
	case tea.KeyEnter:
		return m.fsRun(1)
	case tea.KeyDown:
		s.row, s.chip = 1, 0
		return nil
	case tea.KeyEsc:
		if s.running {
			m.fsStop(true)
			return nil
		}
		s.results = true // leave the box; the text stays
		return nil
	case tea.KeyTab:
		return m.subPage(1)
	case tea.KeyShiftTab:
		return m.subPage(-1)
	}
	return nil
}

// fsPickKey: + Mod's name box: type a name, ↑↓ choose, enter adds, esc closes.
func (m *model) fsPickKey(k tea.KeyMsg) tea.Cmd {
	s := &m.fs
	switch k.Type {
	case tea.KeyRunes, tea.KeySpace:
		if k.Type == tea.KeySpace {
			s.pick += " "
		} else {
			s.pick += string(k.Runes)
		}
	case tea.KeyBackspace:
		if r := []rune(s.pick); len(r) > 0 {
			s.pick = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		s.pick = ""
	case tea.KeyUp:
		s.candAt = max(s.candAt-1, 0)
		return nil
	case tea.KeyDown:
		s.candAt = min(s.candAt+1, max(len(s.cands)-1, 0))
		return nil
	case tea.KeyEnter, tea.KeyTab:
		if s.candAt < len(s.cands) {
			s.mods = append(s.mods, s.cands[s.candAt])
			s.picking, s.pick, s.cands = false, "", nil
			s.chip = len(s.mods) // stays on + Mod
			if strings.TrimSpace(s.text) != "" {
				return m.fsRun(1)
			}
		}
		return nil
	case tea.KeyEsc:
		s.picking, s.pick, s.cands = false, "", nil
		return nil
	default:
		return nil
	}
	s.cands = m.fsCandidates(s.pick)
	s.candAt = 0
	return nil
}

// fsCandidates: mods to name for text, the ones already named left out (6 at most).
func (m *model) fsCandidates(text string) []view.RuleMod {
	list, err := view.RuleCandidates(m.app, text, "")
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	var out []view.RuleMod
	for _, c := range list {
		named := c.ID == ""
		for _, x := range m.fs.mods {
			named = named || x.ID == c.ID
		}
		if !named {
			out = append(out, c)
		}
	}
	return out
}

// fsChip is one chip of the kind or scope row.
type fsChip struct {
	label string
	on    bool
	off   bool // greyed: cannot be switched now
	do    func(m *model) tea.Cmd
}

// fsChips are row r's chips (1 the kinds, 2 the scope).
func (m *model) fsChips(r int) []fsChip {
	s := &m.fs
	var out []fsChip
	switch r {
	case 1:
		out = append(out,
			fsChip{label: T("files"), on: !s.code, do: func(m *model) tea.Cmd { return m.fsChange(func() { m.fs.code = false }) }},
			fsChip{label: T("C# code"), on: s.code, do: func(m *model) tea.Cmd { return m.fsChange(func() { m.fs.code = true }) }})
		if !s.code {
			for _, k := range view.FileSearchKinds {
				kind := k.Kind
				label := k.Label
				if label != "Defs" && label != "About" { // names the window does not translate
					label = T(label)
				}
				out = append(out, fsChip{label: label, on: s.kinds[kind], do: func(m *model) tea.Cmd {
					return m.fsChange(func() {
						if m.fs.kinds[kind] {
							delete(m.fs.kinds, kind)
						} else {
							m.fs.kinds[kind] = true
						}
					})
				}})
			}
		}
	case 2:
		for i, mod := range s.mods {
			i := i
			out = append(out, fsChip{label: mod.Name + " ×", on: true, do: func(m *model) tea.Cmd {
				return m.fsChange(func() { m.fs.mods = append(m.fs.mods[:i:i], m.fs.mods[i+1:]...) })
			}})
		}
		out = append(out, fsChip{label: T("+ Mod"), do: func(m *model) tea.Cmd {
			m.fs.picking, m.fs.pick, m.fs.cands, m.fs.candAt = true, "", nil, 0
			return nil
		}})
	}
	return out
}

// fsKey: the page's keys when the box does not have them.
func (m *model) fsKey(s string) tea.Cmd {
	f := &m.fs
	if cmd, ok := m.fsOption(s); ok {
		return cmd
	}
	switch s {
	case "esc":
		switch {
		case f.running:
			m.fsStop(true)
		case !f.results:
			f.row = 0 // back to the box
		default:
			m.tab = tabList // the TUI has no sidebar: the editor (06 §9.1)
			return m.requestDetail()
		}
		return nil
	case "/", "ctrl+f", "1":
		f.results, f.row = false, 0
		return nil
	case "2":
		f.results = true
		return nil
	case "[":
		if f.page != nil && f.page.Page > 1 {
			return m.fsRun(f.page.Page - 1)
		}
		return nil
	case "]":
		if f.page != nil && f.page.More {
			return m.fsRun(f.page.Page + 1)
		}
		return nil
	}
	if !f.results {
		return m.fsRowKey(s)
	}
	page := max(m.h-12, 1)
	switch s {
	case "up", "k":
		f.step(-1)
	case "down", "j":
		f.step(1)
	case "pgup":
		f.step(-page)
	case "pgdown":
		f.step(page)
	case "home":
		f.cursor = f.firstCursor()
	case "end":
		f.step(len(f.rows))
	case "enter", "ctrl+o":
		if f.cursor < len(f.rows) && f.rows[f.cursor].cursor {
			m.fsOpenFolder(f.rows[f.cursor])
		}
	}
	return nil
}

// fsRowKey: the kind and scope rows of area 1.
func (m *model) fsRowKey(s string) tea.Cmd {
	f := &m.fs
	chips := m.fsChips(f.row)
	switch s {
	case "up", "k":
		f.row = max(f.row-1, 0)
		f.chip = 0
	case "down", "j":
		if f.row == 2 {
			f.results = true
		} else {
			f.row++
			f.chip = 0
		}
	case "left", "h":
		f.chip = max(f.chip-1, 0)
	case "right", "l":
		f.chip = min(f.chip+1, len(chips)-1)
	case " ", "enter", "x", "delete":
		if f.chip < len(chips) && !chips[f.chip].off {
			if (s == "x" || s == "delete") && f.row != 2 {
				return nil
			}
			return chips[f.chip].do(m)
		}
	}
	return nil
}

// fsOpenFolder opens the folder of a result's file (view.SearchFolder says which may open).
func (m *model) fsOpenFolder(r fsRow) {
	ix, err := m.app.Index()
	if err == nil {
		var dir string
		if dir, err = view.SearchFolder(ix, m.app.Home, r.modID, r.dirPath); err == nil {
			open := m.opts.OpenDir
			if open == nil {
				open = openDir
			}
			err = open(dir)
		}
	}
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return
	}
	m.say(sayOK, T("opened the folder"))
}

// ---- drawing ----

// fsLines draws the page in n lines.
func (m *model) fsLines(n int) []string {
	f := &m.fs
	inner := max(m.w-4, 20)
	num := func(a int, on bool) string {
		if on {
			return sAccent.Render(itoa(a) + " ")
		}
		return sDimS.Render(itoa(a) + " ")
	}
	// area 1: the box and its toggles
	box := f.text
	if !f.results && f.row == 0 && !f.picking {
		box += "_"
	}
	var toggles []string
	tog := func(label, key string, on, off bool) {
		st := sDimS
		switch {
		case off:
			st = sDimS.Strikethrough(true)
		case on:
			st = sSel
		}
		toggles = append(toggles, st.Render(label)+sDimS.Render(" "+key))
	}
	if !f.code {
		tog("Aa", "alt+c", f.cs || f.def, f.def)
		tog("ab", "alt+w", f.wd || f.def, f.def)
	}
	tog(".*", "alt+r", f.regex, false)
	if !f.code {
		tog("Def", "alt+d", f.def, false)
	}
	right := strings.Join(toggles, "  ")
	rw := width.StringWidth(stripANSIWidth(right))
	prompt := sDimS.Render(markPrompt + " ")
	if !f.results && f.row == 0 {
		prompt = sAccent.Render(markPrompt + " ")
	}
	boxW := max(inner-rw-4, 10)
	out := []string{num(1, !f.results) + prompt + sText.Render(fit(box, boxW)) + "  " + right}
	if box == "" || box == "_" {
		out[0] = num(1, !f.results) + prompt + sDimS.Render(fit(T("text to find")+"_", boxW)) + "  " + right
	}
	for r := 1; r <= 2; r++ {
		label := T("kind")
		if r == 2 {
			label = T("scope")
		}
		line := "  " + sDimS.Render(fit(label, 6)) + "  "
		if r == 2 && len(f.mods) == 0 {
			line += sSecond.Render(f.info) + "   "
		}
		for i, c := range m.fsChips(r) {
			cur := !f.results && f.row == r && i == f.chip && !f.picking
			st := sSecond
			switch {
			case c.off:
				st = sDimS
			case c.on:
				st = sText
			}
			label := c.label
			if c.on && r == 1 {
				label = "✓" + label
			}
			if cur {
				line += sAccent.Render(markSel) + sSel.Render(label) + "  "
			} else {
				line += " " + st.Render(label) + "  "
			}
		}
		out = append(out, width.Truncate(line, m.w-2, "…"))
	}
	if f.picking {
		out = append(out, "    "+sAccent.Render(markPrompt+" ")+sText.Render(fit(f.pick+"_", inner-6)))
		for i, c := range f.cands {
			name := c.Name
			if c.Pos > 0 {
				name += "  " + sDimS.Render(itoa(c.Pos))
			}
			if i == f.candAt {
				out = append(out, "      "+sAccent.Render(markSel+" ")+sSel.Render(c.Name)+strings.TrimPrefix(name, c.Name))
			} else {
				out = append(out, "        "+sText.Render(c.Name)+strings.TrimPrefix(name, c.Name))
			}
		}
		if strings.TrimSpace(f.pick) != "" && len(f.cands) == 0 {
			out = append(out, "        "+sDimS.Render(T("nothing matches")))
		}
	}
	out = append(out, "")
	// area 2: the results
	head := num(2, f.results) + sDimS.Render(T("results"))
	if f.results {
		head = num(2, true) + sAccent.Render(T("results"))
	}
	switch {
	case f.running:
		lead := f.progress.get()
		if lead == "" {
			lead = T("searching…")
		}
		head += "  " + sSecond.Render(markBusy+" "+lead) + sDimS.Render("  esc "+T("stop"))
	case f.page != nil && f.page.Summary != "":
		head += "  " + sSecond.Render(f.page.Summary)
	}
	out = append(out, width.Truncate(head, m.w-2, "…"))
	rows := max(n-len(out)-1, 1)
	body := m.fsResultLines(inner)
	if f.cursor < f.off {
		f.off = f.cursor
	}
	if f.cursor >= f.off+rows {
		f.off = f.cursor - rows + 1
	}
	f.off = max(0, min(f.off, len(body)-rows))
	out = append(out, window(body, f.off, rows)...)
	return out
}

// fsResultLines are the results' lines, one per row, then the notes.
func (m *model) fsResultLines(w int) []string {
	f := &m.fs
	p := f.page
	if p == nil {
		return nil
	}
	var out []string
	for i, r := range f.rows {
		mod := p.Mods[r.mod]
		cur := f.results && i == f.cursor
		mark := "  "
		if cur {
			mark = sAccent.Render(markSel) + " "
		}
		switch {
		case r.file < 0:
			pos := ""
			if mod.Pos > 0 {
				pos = itoa(mod.Pos)
			}
			out = append(out, "  "+sDimS.Render(width.FillLeft(pos, 4))+"  "+sBold.Render(mod.Name)+"  "+sDimS.Render(Tf("%d matches", mod.Count)))
		case r.line < 0:
			file := mod.Files[r.file]
			text := file.Rel
			st := sText
			if cur {
				st = sSel
			}
			line := "      " + mark + st.Render(text)
			if file.Count != mod.Count {
				line += "  " + sDimS.Render(Tf("%d matches", file.Count))
			}
			if file.More > 0 {
				line += "  " + sDimS.Render(Tf("+%d matches not shown", file.More))
			}
			out = append(out, width.Truncate(line, w+2, "…"))
		default:
			l := mod.Files[r.file].Lines[r.line]
			ctx := strings.TrimSpace(l.Role + "  " + l.Within)
			line := "        " + mark + sDimS.Render(width.FillLeft(lineNo(l.Line), 5)) + "  "
			if ctx != "" {
				line += sDimS.Render(ctx) + "  "
			}
			room := max(w+2-width.StringWidth(stripANSIWidth(line)), 8)
			line += highlight(strings.TrimSpace(l.Text), f.at, room, cur)
			out = append(out, line)
		}
	}
	if len(f.rows) == 0 {
		out = append(out, "  "+sSecond.Render(T("nothing found")))
		if p.Hint != "" {
			out = append(out, "  "+sDimS.Render(width.Truncate(p.Hint, w, "…")))
		}
	}
	for _, note := range p.Notes {
		out = append(out, "  "+sDimS.Render(width.Truncate(note, w, "…")))
	}
	if p.Page > 1 || p.More {
		var parts []string
		if p.Page > 1 {
			parts = append(parts, "[ "+T("previous page"))
		}
		parts = append(parts, Tf("page %d", p.Page))
		if p.More {
			parts = append(parts, "] "+T("next page"))
		}
		out = append(out, "", "  "+sDimS.Render(strings.Join(parts, "   ")))
	}
	return out
}

func lineNo(n int) string {
	if n == 0 {
		return ""
	}
	return itoa(n)
}

// highlight marks what q matched in text, cut to w cells.
func highlight(text string, q view.FileSearchReq, w int, cur bool) string {
	text = width.Truncate(text, w, "…")
	base := sSecond
	if cur {
		base = sText
	}
	var ranges [][2]int
	switch {
	case q.Pattern == "":
	case q.Regex && !q.Def:
		expr := q.Pattern
		if !q.Case {
			expr = "(?i)" + expr
		}
		if re, err := regexp.Compile(expr); err == nil {
			for _, r := range re.FindAllStringIndex(text, -1) {
				if r[1] > r[0] {
					ranges = append(ranges, [2]int{r[0], r[1]})
				}
			}
		}
	default:
		src, pat := text, q.Pattern
		if !q.Case && !q.Def {
			src, pat = strings.ToLower(text), strings.ToLower(pat)
		}
		for at := 0; len(pat) > 0 && len(src) == len(text); {
			i := strings.Index(src[at:], pat)
			if i < 0 {
				break
			}
			ranges = append(ranges, [2]int{at + i, at + i + len(pat)})
			at += i + len(pat)
		}
	}
	var b strings.Builder
	at := 0
	for _, r := range ranges {
		b.WriteString(base.Render(text[at:r[0]]))
		b.WriteString(sWarnS.Bold(true).Render(text[r[0]:r[1]]))
		at = r[1]
	}
	b.WriteString(base.Render(text[at:]))
	return b.String()
}

// fsPairs: the hint line of Search files.
func (m *model) fsPairs() [][2]string {
	f := &m.fs
	switch {
	case f.picking:
		return [][2]string{{T("type"), T("mod name")}, {"↑↓", T("choose")}, {"enter", T("add")}, {"esc", T("close")}}
	case !f.results && f.row == 0:
		pairs := [][2]string{{"enter", T("search")}, {"↓", T("kind") + " / " + T("scope")}, {"alt+c alt+w alt+r alt+d", T("case, whole word, regex, Def name")}}
		if f.running {
			pairs = append(pairs, [2]string{"esc", T("stop")})
		}
		return append(pairs, [2]string{"?", T("all keys")})
	case !f.results:
		return [][2]string{{"←→", T("choose")}, {"space", T("switch it")}, {"↑↓", T("kind") + " / " + T("scope")}, {"/", T("search")}, {"?", T("all keys")}}
	}
	pairs := [][2]string{{"/", T("search")}}
	if f.running {
		pairs = append(pairs, [2]string{"esc", T("stop")})
	}
	if len(f.rows) > 0 {
		pairs = append(pairs, [2]string{"↑↓", T("move the cursor")}, [2]string{"enter ctrl+o", T("open the folder")})
	}
	if f.page != nil && (f.page.Page > 1 || f.page.More) {
		pairs = append(pairs, [2]string{"[ ]", T("previous / next page")})
	}
	return append(pairs, [2]string{"?", T("all keys")})
}
