package tui

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

const helpText = `four columns (06 §5)
  ↑↓ j k     move            ←→ h l tab   switch column      [ ]   switch list (narrow)
  space      enable / disable / put back from staging
  t          stage / unstage        T     put everything back from staging
  J K        move up / down         m     move to a position
  S          sort (preview first)   w     save to the game (preview first)
  u ctrl+r   undo / redo            L     launch the game
  /          filter this column     v     mark several
  enter      details                i     technical information
  !          needs attention        c     recent changes
  R          rescan                 E     switch environment
  :          run an archo command   ? help   q quit
mouse: click selects; drag between columns enables, disables or stages; drag within
"enabled" changes the order; the wheel scrolls`

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
			if it := m.current(); it != nil && it.id != "" && (m.dd == nil || m.dd.id != it.id) {
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
		if m.imgLoading == msg.key {
			m.imgLoading = ""
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
		return Tf("scan complete · %d changes · c shows them", m.changes)
	}
	return T("scan complete · nothing changed") + " (" + firstLine(out) + ")"
}

func (m *model) key(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		m.quitting = true
		return tea.Quit
	}
	if m.prompt == nil && !m.editing {
		m.status, m.sayKind = "", sayNone // the status line lasts until the next key
	}
	switch {
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
		m.openPanel(T("help"), T(helpText), nil)
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
		case "l":
			return m.chooseLog()
		}
		return m.problemsKey(s)
	case tabChanges:
		if s == "esc" {
			m.tab = tabList
			return m.requestDetail()
		}
		return m.changesKey(s)
	}
	return m.listKey(s)
}

// focusCycle: ←→ tab move between the list columns; staging is always reachable, also
// when it is hidden (then it takes the first column's place, 06 §5.1).
func (m *model) focusMove(d int) tea.Cmd {
	m.focus = colKind((int(m.focus) + d + 3) % 3)
	m.detailRef = ""
	return m.requestDetail()
}

func (m *model) listKey(s string) tea.Cmd {
	c := m.col()
	page := m.rowsShown() - 1
	switch s {
	case "up", "k":
		return m.moveCursor(-1)
	case "down", "j":
		return m.moveCursor(1)
	case "pgup":
		return m.moveCursor(-page)
	case "pgdown":
		return m.moveCursor(page)
	case "home", "g":
		return m.moveCursor(-len(c.view))
	case "end", "G":
		return m.moveCursor(len(c.view))
	case "left", "h", "shift+tab":
		return m.focusMove(-1)
	case "right", "l", "tab":
		return m.focusMove(1)
	case "[":
		return m.focusMove(-1)
	case "]":
		return m.focusMove(1)
	case "/":
		m.editing = true
		return nil
	case "esc":
		if c.filter != "" {
			c.filter = ""
			m.refilterCol(m.focus, c.curID())
		}
		m.marked = map[string]bool{}
		return nil
	case "!":
		m.tab = tabProblems
		return m.ensureProblems()
	case "c":
		m.tab = tabChanges
		m.loadChanges()
		return nil
	case "i":
		m.detailTech = !m.detailTech
		m.detailRef = ""
		return m.requestDetail()
	case "v":
		if it := c.cur(); it != nil && it.id != "" {
			if m.marked[it.id] {
				delete(m.marked, it.id)
			} else {
				m.marked[it.id] = true
			}
			return m.moveCursor(1)
		}
		return nil
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
		return m.requestDetail()
	case "J", "K":
		it := c.cur()
		if m.focus != cActive || it == nil {
			m.say(sayInfo, T("only enabled mods can be moved"))
			return nil
		}
		dir := "--down"
		if s == "K" {
			dir = "--up"
		}
		id, name := it.id, it.name // it points into the rows that the command rebuilds
		if m.act("", "move", it.ref, dir) {
			m.say(sayOK, Tf("%s is now number %d", name, m.st.Position(id)))
		}
		m.refilterCol(cActive, id)
		return m.requestDetail()
	case "m":
		it := c.cur()
		if m.focus != cActive || it == nil || it.id == "" {
			m.say(sayInfo, T("only enabled mods can be moved"))
			return nil
		}
		ref, name, id := it.ref, it.name, it.id
		m.prompt = &prompt{label: Tf("move %s to number: ", name), submit: func(m *model, text string) tea.Cmd {
			n, err := strconv.Atoi(strings.TrimSpace(text))
			if err != nil || n < 1 {
				m.say(sayFail, T("expected a number"))
				return nil
			}
			if m.act("", "move", ref, "--to", fmt.Sprint(n)) {
				m.say(sayOK, Tf("%s is now number %d", name, m.st.Position(id)))
			}
			m.refilterCol(cActive, id)
			return m.requestDetail()
		}}
		return nil
	case "S":
		out, code := m.run("sort", "--dry-run")
		var confirm []string
		if code == cmds.ExitOK && !strings.HasPrefix(out, "sort: 0 moved") {
			confirm = []string{"sort"}
		}
		m.openPanel(T("sort — preview"), out, confirm)
		return nil
	case "w":
		out, code := m.run("save", "--dry-run")
		var confirm []string
		if code == cmds.ExitOK {
			confirm = []string{"save"}
		}
		m.openPanel(T("save to game — preview"), out, confirm)
		return nil
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
			m.full, m.fullOff = true, 0
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

// space enables (not enabled), disables (enabled) or puts back (staging) the selection.
func (m *model) space() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 {
		return nil
	}
	// sel points into the rows that the command rebuilds: take what the message needs first.
	keep, what, first := sel[0].id, named(sel), sel[0].name
	refs := refsOf(sel)
	switch m.focus {
	case cInactive:
		m.act(Tf("enabled %s", what), append([]string{"enable"}, refs...)...)
	case cActive:
		m.act(Tf("disabled %s", what), append([]string{"disable"}, refs...)...)
	case cStaging:
		if m.act("", append([]string{"stage", "restore"}, refs...)...) {
			if len(refs) == 1 {
				m.say(sayOK, Tf("%s is back at number %d", first, m.st.Position(keep)))
			} else {
				m.say(sayOK, Tf("put %s back in place", what))
			}
		}
	}
	m.marked = map[string]bool{}
	m.refilterCol(m.focus, "")
	return m.requestDetail()
}

// stageKey: t stages the selection, or takes it out of staging.
func (m *model) stageKey() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 || sel[0].id == "" {
		return nil
	}
	if m.focus == cStaging {
		m.act(Tf("took %s out of staging", named(sel)), append([]string{"stage", "drop"}, refsOf(sel)...)...)
	} else {
		m.act(Tf("staged %s", named(sel)), append([]string{"stage", "add"}, refsOf(sel)...)...)
	}
	m.marked = map[string]bool{}
	return m.requestDetail()
}

func (m *model) filterKey(k tea.KeyMsg) tea.Cmd {
	c := m.col()
	keep := c.curID()
	switch k.Type {
	case tea.KeyEnter:
		m.editing = false
		return nil
	case tea.KeyEsc:
		m.editing, c.filter = false, ""
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
	default:
		return nil
	}
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
}

func (m *model) promptKey(k tea.KeyMsg) tea.Cmd {
	p := m.prompt
	switch k.Type {
	case tea.KeyEnter:
		m.prompt = nil
		return p.submit(m, strings.TrimSpace(p.text))
	case tea.KeyEsc:
		m.prompt, m.tagging = nil, nil
	case tea.KeyTab:
		if m.tagging != nil {
			p.text = m.tagging.complete(p.text)
		}
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

// ---- mouse (06 §5.3) ----

// hit returns the column slot and view index under a point (index -1: the title row or
// below the last row; ok false: not over a list column).
func (m *model) hit(x, y int) (colKind, int, bool) {
	l := m.layout()
	for _, s := range l.slots {
		if x < s.x || x >= s.x+s.w {
			continue
		}
		c := &m.cols[s.kind]
		if y < 3 || y >= 3+m.rowsShown()-1 {
			return s.kind, -1, true
		}
		i := c.off + y - 3
		if i >= len(c.view) {
			i = -1
		}
		return s.kind, i, true
	}
	return 0, -1, false
}

func (m *model) mouse(e tea.MouseMsg) tea.Cmd {
	if m.prompt != nil || m.editing || m.panel != nil || m.insp != nil || m.tab != tabList {
		switch e.Button {
		case tea.MouseButtonWheelDown:
			return m.key(tea.KeyMsg{Type: tea.KeyDown})
		case tea.MouseButtonWheelUp:
			return m.key(tea.KeyMsg{Type: tea.KeyUp})
		}
		return nil
	}
	k, i, ok := m.hit(e.X, e.Y)
	switch e.Button {
	case tea.MouseButtonWheelDown, tea.MouseButtonWheelUp:
		if ok {
			m.focus = k
		}
		d := 3
		if e.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		return m.moveCursor(d)
	}
	switch e.Action {
	case tea.MouseActionPress:
		if e.Button != tea.MouseButtonLeft || !ok {
			return nil
		}
		m.focus = k
		if i >= 0 {
			c := &m.cols[k]
			c.cursor = i
			if it := c.cur(); it != nil && it.id != "" {
				m.drag = &drag{from: k, id: it.id, ref: it.ref, name: it.name}
			}
		}
		return m.requestDetail()
	case tea.MouseActionMotion:
		if m.drag != nil {
			m.drag.active = true
			if ok {
				m.say(sayBusy, m.dropText(m.drag, k, i))
			}
		}
		return nil
	case tea.MouseActionRelease:
		d := m.drag
		m.drag = nil
		if d == nil || !d.active || !ok {
			if d != nil && d.active {
				m.status = ""
			}
			return nil
		}
		return m.drop(d, k, i)
	}
	return nil
}

// dropPos is the 1-based enabled position a drop on view index i means (0: the end).
func (m *model) dropPos(i int) int {
	c := &m.cols[cActive]
	if i < 0 || i >= len(c.view) {
		return 0
	}
	return c.items[c.view[i]].pos
}

func (m *model) dropText(d *drag, k colKind, i int) string {
	switch {
	case k == cActive && d.from == cActive:
		if p := m.dropPos(i); p > 0 {
			return Tf("move %s to number %d", d.name, p)
		}
		return Tf("move %s to the end", d.name)
	case k == cActive:
		if p := m.dropPos(i); p > 0 {
			return Tf("enable %s at number %d", d.name, p)
		}
		return Tf("enable %s", d.name)
	case k == cStaging && d.from != cStaging:
		return Tf("stage %s", d.name)
	case k == cInactive && d.from == cActive:
		return Tf("disable %s", d.name)
	case k == cInactive && d.from == cStaging:
		return Tf("take %s out of staging", d.name)
	}
	return d.name
}

// drop performs a drag: between columns it enables, disables or stages; within "enabled"
// it moves.
func (m *model) drop(d *drag, k colKind, i int) tea.Cmd {
	pos := 0
	if k == cActive {
		pos = m.dropPos(i)
	}
	at := func(flag string) []string {
		if pos > 0 {
			return []string{flag, fmt.Sprint(pos)}
		}
		return nil
	}
	switch {
	case k == cActive && d.from == cActive:
		if pos == 0 {
			pos = len(m.st.List)
		}
		if pos == m.st.Position(d.id) {
			m.status = ""
			return nil
		}
		if m.act("", "move", d.ref, "--to", fmt.Sprint(pos)) {
			m.say(sayOK, Tf("%s is now number %d", d.name, m.st.Position(d.id)))
		}
	case k == cActive && d.from == cInactive:
		if m.act("", append([]string{"enable", d.ref}, at("--at")...)...) {
			m.say(sayOK, Tf("enabled %s at number %d", d.name, m.st.Position(d.id)))
		}
	case k == cActive && d.from == cStaging:
		if pos == 0 { // below the last row: the end, not the remembered place
			pos = len(m.st.List) + 1
		}
		if m.act("", append([]string{"stage", "restore", d.ref}, at("--at")...)...) {
			m.say(sayOK, Tf("%s is back at number %d", d.name, m.st.Position(d.id)))
		}
	case k == cStaging && d.from != cStaging:
		m.act(Tf("staged %s", d.name), "stage", "add", d.ref)
	case k == cInactive && d.from == cActive:
		m.act(Tf("disabled %s", d.name), "disable", d.ref)
	case k == cInactive && d.from == cStaging:
		m.act(Tf("took %s out of staging", d.name), "stage", "drop", d.ref)
	default:
		m.status = ""
		return nil
	}
	m.focus = k
	m.refilterCol(k, d.id)
	return m.requestDetail()
}
