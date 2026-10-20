package tui

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

const helpText = `the arrows only move: the cursor, the focus, the scroll; keys that change things act at once, u undoes
areas      1 Disabled  2 Enabled  3 Staging  4 detail      ←→ the next area    tab round all areas
  ↑↓ j k   cursor (in the detail: scroll)    PgUp PgDn  scroll the detail from anywhere   g G  top / bottom
selection  shift+↑↓ a range    ctrl+space (v) add or remove one    esc clear    (another list clears it)
verbs, on the selection or else the cursor's mod:
  space    enable where the order wants it / disable / put back from staging
  t        stage; in Staging: put back where it was     T  put everything back from staging
  m        move to…: a number, a mod's name (tab: before / after), enter alone = the suggested place
  J K      move down / up one     alt+J alt+K  ten
  #        tags                   enter  details full screen (←→ previous / next mod, tab sub-sections, I technical pages)
  o        Workshop page          i  technical information     x  swap for another copy    r  add a rule
  S        sort (preview first)   w  save to the game (preview first)    L  launch the game
  u ctrl+r undo / redo           e  export the list (import: ctrl+k import a list)
/          search the focused list; tab takes it to the next list with matches; esc clears
!          needs attention        c  recent changes     R  rescan     E  switch environment
< > =      column width (= resets)    z  wrap names    [ ]  switch list (narrow)
ctrl+k     command palette        :  run an archo command      ?  this help    q  quit
mouse: click moves the cursor; ctrl+click adds or removes one; shift+click selects a range;
the wheel scrolls the detail; drag a column gap to resize it`

// idleAfter is how long the TUI waits without input before returning freed memory to the
// OS, so the resident size stays near the live heap while idle (06 §8 T5).
const idleAfter = 2 * time.Second

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t, ok := msg.(idleTick); ok {
		if t.seq == m.idleSeq {
			debug.FreeOSMemory()
		}
		return m, nil
	}
	if _, ok := msg.(liveTick); ok {
		// Not input: the idle countdown goes on.
		m.pollLive()
		return m, liveNext()
	}
	m.idleSeq++
	seq := m.idleSeq
	next := tea.Tick(idleAfter, func(time.Time) tea.Msg { return idleTick{seq} })
	mm, cmd := m.update(msg)
	return mm, tea.Batch(cmd, next, m.imageCmds(m.visibleDetail()))
}

// visibleDetail lays out the detail as the next frame will show it (for image loads).
func (m *model) visibleDetail() []dline {
	if !m.inlineImages() || m.tab != tabList || m.panel != nil || m.insp != nil {
		return nil
	}
	if m.full {
		return m.detailLayout(detailOpts{width: max(m.w-4, 20), full: true, images: true, techText: m.detail})
	}
	l := m.layout()
	if l.detailW == 0 {
		return nil
	}
	return m.detailLayout(detailOpts{width: l.detailW, descMax: m.descMax(), images: true, techText: m.detail})
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		for k := range m.cols {
			m.cols[k].scroll(m.rowsShown() - 1)
		}
		return m, m.requestDetail()
	case detailTick:
		if msg.seq == m.detailSeq {
			var data tea.Cmd
			if it := m.current(); it != nil && it.id != "" && (m.dd == nil || m.dd.ID != it.id) {
				data = m.loadData(it.id, false, false)
			}
			return m, tea.Batch(m.loadDetail(), data)
		}
		return m, nil
	case detailDataMsg:
		if m.status == T("fetching the Workshop page…") {
			m.status, m.sayKind = "", sayNone
		}
		return m, m.onData(msg.d)
	case fetchTick:
		return m, m.onFetchTick(msg.id)
	case fullSecsMsg:
		if it := m.current(); it != nil && it.id == msg.s.id {
			m.fsecs = msg.s
		}
		return m, nil
	case thumbTick:
		if it := m.current(); msg.seq == m.thumbSeq && it != nil && it.id == msg.id {
			m.thumbShown = msg.id
		}
		return m, nil
	case imgLoadedMsg:
		e := msg.e
		m.imgs[msg.key] = &e
		m.touchImage(msg.key)
		if m.imgLoading == msg.key {
			m.imgLoading = ""
		}
		if m.prefetching == msg.key {
			m.prefetching = ""
		}
		m.evictImages()
		return m, nil
	case detailMsg:
		if it := m.current(); it != nil && it.ref == msg.ref {
			m.detailRef, m.detail = msg.ref, msg.lines
		}
		return m, nil
	case scanDoneMsg:
		m.scanning = false
		m.reload()
		m.detailRef = ""
		if msg.err != nil {
			m.openPanel(T("scan failed"), msg.out+"\n"+msg.err.Error(), nil)
		} else {
			m.say(sayOK, m.scanSummary(msg.out))
			if m.tab == tabChanges {
				m.loadChanges()
			}
		}
		return m, m.requestDetail()
	case analyzeLineMsg, analyzeDoneMsg:
		return m, m.analyzeMsg(msg)
	case tea.KeyMsg:
		return m, m.key(msg)
	case tea.MouseMsg:
		return m, m.mouse(msg)
	}
	return m, nil
}

// scanSummary: "scan complete · 12 changes" (06 §4.1); the CLI line keeps the scan id.
func (m *model) scanSummary(out string) string {
	if m.changes > 0 {
		return T("scan complete") + " " + markDot + " " + view.ChangeSummary(m.lastEvents) + " " + markDot + " " + T("c shows them")
	}
	return T("scan complete · nothing changed")
}

func (m *model) key(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		m.quitting = true
		return tea.Quit
	}
	if m.prompt == nil && !m.editing && m.pos == nil {
		m.status, m.sayKind = "", sayNone // the status line lasts until the next key
	}
	placed := m.placed // "m changes it" holds for the next key only (06 §13.6)
	m.placed = nil
	if a := m.armed; a != nil {
		m.armed = nil
		if s == a.key || (a.key == "space" && s == " ") {
			return a.run(m)
		}
		m.say(sayInfo, T("cancelled"))
		return nil
	}
	switch {
	case m.pal != nil:
		return m.paletteKey(k)
	case s == "ctrl+k" && m.prompt == nil && !m.editing && m.pos == nil:
		m.panel, m.insp, m.tags = nil, nil, nil
		return m.openPalette()
	case m.tags != nil:
		return m.tagBarKey(k)
	case m.pos != nil:
		return m.posKey(k)
	case m.prompt != nil:
		return m.promptKey(k)
	case m.panel != nil:
		return m.panelKey(s)
	case m.insp != nil:
		return m.inspectKey(s)
	case m.editing:
		return m.filterKey(k)
	case m.full:
		return m.fullKey(s)
	}
	switch s {
	case "q":
		m.quitting = true
		return tea.Quit
	case "?":
		m.openPanel(T("all keys"), T(helpText), nil)
		return nil
	case ":":
		m.prompt = &prompt{label: T("archo command: "), submit: func(m *model, text string) tea.Cmd {
			argv := strings.Fields(text)
			if len(argv) > 0 && argv[0] == "archo" {
				argv = argv[1:]
			}
			if len(argv) == 0 {
				return nil
			}
			if argv[0] == "scan" {
				return m.startScan() // never scan inside the TUI process
			}
			out, _ := m.run(argv...)
			m.openPanel("archo "+strings.Join(argv, " "), out, nil)
			return m.requestDetail()
		}}
		return nil
	case "E":
		return m.chooseEnv()
	case "R":
		return m.startScan()
	}
	switch m.tab {
	case tabProblems:
		switch s {
		case "esc":
			m.tab = tabList
			return m.requestDetail()
		}
		return m.needsKey(s)
	case tabChanges:
		if s == "esc" {
			m.tab = tabList
			return m.requestDetail()
		}
		return m.changesKey(s)
	}
	if m.detailFocus {
		if cmd, ok := m.detailKey(s); ok {
			return cmd
		}
	}
	if m.histActive() {
		if cmd, ok := m.histKey(s); ok {
			return cmd
		}
	}
	if s == "m" {
		return m.openPos(placed)
	}
	return m.listKey(s)
}

// listKey: the keys of the list areas (06 §13.4, §13.6). The arrows only navigate.
func (m *model) listKey(s string) tea.Cmd {
	c := m.col()
	switch s {
	case "up", "k", "ctrl+up":
		m.sel.anchor = ""
		return m.moveCursor(-1)
	case "down", "j", "ctrl+down":
		m.sel.anchor = ""
		return m.moveCursor(1)
	case "shift+up":
		return m.selExtend(-1)
	case "shift+down":
		return m.selExtend(1)
	case "home", "g":
		return m.moveCursor(-len(c.view))
	case "end", "G":
		return m.moveCursor(len(c.view))
	case "pgup":
		return m.scrollDetail(-m.detailPage())
	case "pgdown":
		return m.scrollDetail(m.detailPage())
	case "left", "h":
		return m.areaStep(-1)
	case "right", "l":
		return m.areaStep(1)
	case "tab":
		return m.areaCycle(1)
	case "shift+tab":
		return m.areaCycle(-1)
	case "1", "2", "3", "4":
		return m.goArea(int(s[0] - '0'))
	case "[":
		return m.goArea((int(m.focus)+2)%3 + 1)
	case "]":
		return m.goArea((int(m.focus)+1)%3 + 1)
	case "/":
		m.editing, m.detailFocus = true, false
		return nil
	case "esc":
		if c.filter != "" {
			c.filter = ""
			m.refilterCol(m.focus, c.curID())
		}
		m.clearSel()
		return nil
	case "ctrl+@", "ctrl+ ", "v":
		return m.selToggle()
	case "!":
		m.tab = tabProblems
		return m.ensureProblems()
	case "c":
		m.tab = tabChanges
		m.loadChanges()
		return nil
	case "e":
		return m.exportChoice()
	case "i":
		m.detailTech = !m.detailTech
		m.detailRef = ""
		return m.requestDetail()
	case " ":
		return m.space()
	case "t":
		return m.stageKey()
	case "#":
		return m.tagKey()
	case "T":
		n := len(m.cols[cStaging].items)
		if n == 0 {
			m.say(sayInfo, T("staging is empty"))
			return nil
		}
		m.act(Tf("put %s back in place", plural(n, "mod")), "stage", "restore")
		m.clearSel()
		return m.requestDetail()
	case "K":
		return m.moveBy(-1)
	case "J":
		return m.moveBy(1)
	case "alt+K", "alt+k":
		return m.moveBy(-10)
	case "alt+J", "alt+j":
		return m.moveBy(10)
	case "<", ">", "=":
		m.resizeKey(s)
		return nil
	case "z":
		m.wrapKey()
		return nil
	case "S":
		return m.sortPreview()
	case "w":
		return m.savePreview()
	case "u":
		m.act(T("undid the last change"), "undo")
		return m.requestDetail()
	case "ctrl+r":
		m.act(T("redid the change"), "redo")
		return m.requestDetail()
	case "L":
		out, code := m.run("launch", "--dry-run")
		var confirm []string
		if code == cmds.ExitOK {
			confirm = []string{"launch"}
		}
		m.openPanel(T("launch"), out, confirm)
		return nil
	case "x":
		if it := c.cur(); it != nil && it.id != "" {
			m.act(Tf("swapped %s for another copy", it.name), "swap", it.ref)
		}
		return m.requestDetail()
	case "enter":
		if it := c.cur(); it != nil && it.id != "" {
			m.full, m.fullOff, m.fullSec = true, 0, 0
			return tea.Batch(m.requestDetail(), m.loadFullSecs())
		}
		return nil
	case "o":
		return m.openWorkshop()
	case "r":
		it := c.cur()
		if it == nil || it.id == "" {
			return nil
		}
		ref, name := it.ref, it.name
		m.prompt = &prompt{label: Tf("rule for %s (after|before|attach|incompatible <ref>): ", name), submit: func(m *model, text string) tea.Cmd {
			f := strings.Fields(text)
			if len(f) != 2 {
				m.say(sayFail, T("expected: <kind> <ref>"))
				return nil
			}
			m.act(T("rule added"), "rule", "add", f[0], ref, f[1])
			return nil
		}}
		return nil
	}
	return nil
}

// filterKey: typing the search of the focused list (06 §13.8). The status line counts the
// matches in the other lists; tab takes the search to the next list that has any.
func (m *model) filterKey(k tea.KeyMsg) tea.Cmd {
	c := m.col()
	keep := c.curID()
	switch k.Type {
	case tea.KeyEnter:
		m.editing = false
		return nil
	case tea.KeyEsc:
		m.editing, c.filter = false, ""
	case tea.KeyTab:
		return m.searchJump()
	case tea.KeyBackspace:
		if r := []rune(c.filter); len(r) > 0 {
			c.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		c.filter += string(k.Runes)
	case tea.KeyUp:
		return m.moveCursor(-1)
	case tea.KeyDown:
		return m.moveCursor(1)
	case tea.KeyPgUp:
		return m.scrollDetail(-m.detailPage())
	case tea.KeyPgDown:
		return m.scrollDetail(m.detailPage())
	default:
		return nil
	}
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
}

// otherMatches counts what the focused list's search finds in each other list.
func (m *model) otherMatches() [3]int {
	var n [3]int
	q := m.col().filter
	match, fallback := m.matcher(q)
	if match == nil {
		return n
	}
	for k := range m.cols {
		if colKind(k) == m.focus {
			continue
		}
		for i := range m.cols[k].items {
			if match(&m.cols[k].items[i]) {
				n[k]++
			}
		}
		if n[k] == 0 && fallback != nil {
			for i := range m.cols[k].items {
				if fallback(&m.cols[k].items[i]) {
					n[k]++
				}
			}
		}
	}
	return n
}

// searchJump: tab while searching takes the search to the next list (to the right, round)
// with matches, on its first match, and ends the typing there.
func (m *model) searchJump() tea.Cmd {
	n := m.otherMatches()
	q := m.col().filter
	for d := 1; d < 3; d++ {
		k := colKind((int(m.focus) + d) % 3)
		if n[k] == 0 {
			continue
		}
		from := m.focus
		m.cols[from].filter = ""
		m.refilterCol(from, m.cols[from].curID())
		m.setFocus(k)
		c := m.col()
		c.filter, c.cursor = q, 0
		m.refilterCol(k, "")
		m.editing = false
		return m.requestDetail()
	}
	m.say(sayInfo, T("no other list has a match"))
	return nil
}

// searchLine: "other lists: Disabled 2 · Staging 0 · tab jumps to the next one with matches".
func (m *model) searchLine() string {
	if m.col().filter == "" {
		return ""
	}
	n := m.otherMatches()
	var parts []string
	for k := cInactive; k <= cStaging; k++ {
		if k != m.focus {
			parts = append(parts, fmt.Sprintf("%s %d", k.title(), n[k]))
		}
	}
	return T("other lists: ") + strings.Join(parts, " "+markDot+" ") + " " + markDot + " " + T("tab jumps to the next list with matches")
}

func (m *model) promptKey(k tea.KeyMsg) tea.Cmd {
	p := m.prompt
	switch k.Type {
	case tea.KeyEnter:
		m.prompt = nil
		return p.submit(m, strings.TrimSpace(p.text))
	case tea.KeyEsc:
		m.prompt = nil
	case tea.KeyBackspace:
		if r := []rune(p.text); len(r) > 0 {
			p.text = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		p.text += string(k.Runes)
	}
	return nil
}

func (m *model) panelKey(s string) tea.Cmd {
	p := m.panel
	page := max(m.h-6, 1)
	if p.choices != nil {
		switch s {
		case "up", "k":
			p.choice = max(0, p.choice-1)
		case "down", "j":
			p.choice = min(len(p.choices)-1, p.choice+1)
		case "enter":
			m.panel = nil
			return p.onPick(m, p.choices[p.choice])
		case "esc", "q":
			m.panel = nil
		}
		return nil
	}
	switch s {
	case "down", "j":
		p.off = min(p.off+1, max(len(p.lines)-page, 0))
	case "up", "k":
		p.off = max(p.off-1, 0)
	case "pgdown", " ":
		p.off = min(p.off+page, max(len(p.lines)-page, 0))
	case "pgup":
		p.off = max(p.off-page, 0)
	case "y":
		if p.confirm != nil {
			m.panel = nil
			out, code := m.run(p.confirm...)
			if code == cmds.ExitOK {
				m.say(sayOK, m.confirmedText(p.confirm, out))
			} else {
				m.openPanel("archo "+strings.Join(p.confirm, " "), out, nil)
			}
			if f := m.afterPanel; f != nil {
				m.afterPanel = nil
				f(m)
			}
			m.detailRef = ""
			return m.requestDetail()
		}
	case "n", "esc", "q", "enter":
		m.panel = nil
		m.afterPanel = nil
		if p.confirm != nil {
			m.say(sayInfo, T("cancelled"))
		}
	}
	return nil
}

// confirmedText words the result of a confirmed command.
func (m *model) confirmedText(argv []string, out string) string {
	switch argv[0] {
	case "sort":
		return T("sorted the list")
	case "save":
		return T("saved to the game")
	case "launch":
		return T("the game is starting")
	}
	return m.humanize(firstLine(out))
}

func (m *model) chooseEnv() tea.Cmd {
	envs, err := m.app.Store.Envs()
	if err != nil || len(envs) == 0 {
		m.say(sayInfo, T("no environments (archo env add)"))
		return nil
	}
	p := &panel{title: T("switch environment")}
	for i, e := range envs {
		p.choices = append(p.choices, e.Name)
		if e.Name == m.envName {
			p.choice = i
		}
	}
	p.onPick = func(m *model, name string) tea.Cmd {
		m.app.EnvName = ""
		if m.act(Tf("switched to %s", name), "env", "use", name) {
			for k := range m.cols {
				m.cols[k].cursor, m.cols[k].off, m.cols[k].filter = 0, 0, ""
				m.refilterCol(colKind(k), "")
			}
			m.detailRef = ""
			m.prob.loaded = false
		}
		return m.requestDetail()
	}
	m.panel = p
	return nil
}

// ---- mouse (06 §13.4, §13.5) ----

// hit returns the column slot and view index under a point (index -1: the title row or
// below the last row; ok false: not over a list column).
func (m *model) hit(x, y int) (colKind, int, bool) {
	l := m.layout()
	for _, s := range l.slots {
		if x < s.x || x >= s.x+s.w {
			continue
		}
		rm := m.rowMap[s.kind]
		if y-2 < 0 || y-2 >= len(rm) {
			return s.kind, -1, true
		}
		return s.kind, rm[y-2], true
	}
	return 0, -1, false
}

func (m *model) mouse(e tea.MouseMsg) tea.Cmd {
	wheel := 0
	switch e.Button {
	case tea.MouseButtonWheelDown:
		wheel = 3
	case tea.MouseButtonWheelUp:
		wheel = -3
	}
	if m.prompt != nil || m.editing || m.panel != nil || m.insp != nil || m.tab != tabList || m.full || m.pal != nil || m.tags != nil || m.pos != nil {
		var cmds []tea.Cmd
		for i := 0; i < 3 && wheel != 0; i++ { // the wheel scrolls three lines
			if wheel > 0 {
				cmds = append(cmds, m.key(tea.KeyMsg{Type: tea.KeyDown}))
			} else {
				cmds = append(cmds, m.key(tea.KeyMsg{Type: tea.KeyUp}))
			}
		}
		return tea.Batch(cmds...)
	}
	// The wheel scrolls the detail wherever the pointer is; the focus stays (06 §13.4).
	if wheel != 0 {
		return m.scrollDetail(wheel)
	}
	// A click on the detail column focuses it (area 4).
	if l := m.layout(); l.detailW > 0 && e.X >= l.detailX && e.Y >= 2 {
		if e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft {
			m.detailFocus = true
		}
		return nil
	}
	// Dragging the gap after a list column sets its width (06 §12.1).
	if r := m.resizing; r != nil {
		switch e.Action {
		case tea.MouseActionMotion:
			m.setColW(r.kind, r.w0+e.X-r.x0)
		case tea.MouseActionRelease:
			m.setColW(r.kind, r.w0+e.X-r.x0)
			m.resizing = nil
			m.saveColPrefs()
		}
		return nil
	}
	if e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft && e.Y >= 2 {
		if k, w, ok := m.gapAt(e.X); ok {
			m.resizing = &resizeDrag{kind: k, x0: e.X, w0: w}
			return nil
		}
	}
	k, i, ok := m.hit(e.X, e.Y)
	if e.Action != tea.MouseActionPress || e.Button != tea.MouseButtonLeft || !ok {
		return nil
	}
	// Click: the cursor; shift+click: a range from the cursor; ctrl+click: add or remove
	// one row. Another list drops the selection (06 §13.5). Moving is done with verbs.
	m.setFocus(k)
	c := &m.cols[k]
	if i < 0 {
		return m.requestDetail()
	}
	switch {
	case e.Ctrl:
		c.cursor = i
		m.selToggle()
	case e.Shift:
		if m.sel.anchor == "" {
			m.sel.anchor, m.sel.base = c.curID(), map[string]bool{}
			for id := range m.marked {
				m.sel.base[id] = true
			}
		}
		c.cursor = i
		m.selRangeTo()
	default:
		c.cursor = i
		m.sel.anchor = ""
	}
	return m.requestDetail()
}
