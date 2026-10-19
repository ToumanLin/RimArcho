package tui

import (
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/cmds"
)

const helpText = `list view (06 §3)
  ↑/↓ j/k  move cursor        PgUp/PgDn g/G  page / ends
  space    enable / disable   J / K          move down / up
  m        move to …          v              mark (multi-select)
  S        sort (diff first)  w              save to game (diff first)
  u        undo               ctrl+r         redo
  x        swap within lineage L             launch the game
  enter    inspect            r              add a rule for this mod
  /        search             f              filter: all/enabled/disabled/unloadable
  R        rescan (child process)
problems view: A analyze   i add the suggested ignore rule   enter inspect   s severity filter
views: 1 list · 2 problems · 3 changes      E switch environment
:        run any archo command               ? help   q quit`

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
	return mm, tea.Batch(cmd, next)
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.scroll()
		return m, m.requestDetail()
	case detailTick:
		if msg.seq == m.detailSeq {
			return m, m.loadDetail()
		}
		return m, nil
	case detailMsg:
		if r := m.current(); r != nil && r.ref == msg.ref {
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
			m.status = firstLine(msg.out)
			if m.tab == tabChanges {
				m.loadTab()
			}
		}
		return m, m.requestDetail()
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) key(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		m.quitting = true
		return tea.Quit
	}
	switch {
	case m.prompt != nil:
		return m.promptKey(k)
	case m.panel != nil:
		return m.panelKey(s)
	case m.inspect != nil:
		return m.inspectKey(s)
	case m.editing:
		return m.searchKey(k)
	}
	switch s {
	case "q":
		m.quitting = true
		return tea.Quit
	case "?":
		m.openPanel(T("help"), T(helpText), nil)
		return nil
	case "1":
		m.tab = tabList
		return m.requestDetail()
	case "2", "3":
		m.tab = tabProblems
		if s == "3" {
			m.tab = tabChanges
		}
		m.loadTab()
		return nil
	case ":":
		m.prompt = &prompt{label: ":", submit: func(m *model, text string) tea.Cmd {
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
	if m.tab != tabList {
		return m.tabKey(s)
	}
	return m.listKey(s)
}

func (m *model) listKey(s string) tea.Cmd {
	switch s {
	case "up", "k":
		return m.moveCursor(-1)
	case "down", "j":
		return m.moveCursor(1)
	case "pgup":
		return m.moveCursor(-m.listHeight())
	case "pgdown":
		return m.moveCursor(m.listHeight())
	case "home", "g":
		return m.moveCursor(-len(m.view))
	case "end", "G":
		return m.moveCursor(len(m.view))
	case "/":
		m.editing = true
		return nil
	case "esc":
		if m.search != "" {
			m.search = ""
			m.refilter(m.curID())
		}
		m.marked = map[string]bool{}
		return nil
	case "f":
		m.filter = (m.filter + 1) % len(filters)
		m.refilter(m.curID())
		m.status = T("filter: ") + T(filters[m.filter])
		return m.requestDetail()
	case "v":
		if r := m.current(); r != nil && r.id != "" {
			m.marked[r.id] = !m.marked[r.id]
			if !m.marked[r.id] {
				delete(m.marked, r.id)
			}
			return m.moveCursor(1)
		}
		return nil
	case " ":
		return m.toggle()
	case "J", "K":
		r := m.current()
		if r == nil || r.pos == 0 {
			m.status = T("only enabled mods can be moved")
			return nil
		}
		dir := "--down"
		if s == "K" {
			dir = "--up"
		}
		m.runToStatus("move", r.ref, dir)
		return m.requestDetail()
	case "m":
		r := m.current()
		if r == nil || r.pos == 0 {
			m.status = T("only enabled mods can be moved")
			return nil
		}
		ref := r.ref
		m.prompt = &prompt{label: Tf("move %s to (position | before <ref> | after <ref>): ", ref), submit: func(m *model, text string) tea.Cmd {
			f := strings.Fields(text)
			switch {
			case len(f) == 1:
				if _, err := strconv.Atoi(f[0]); err == nil {
					m.runToStatus("move", ref, "--to", f[0])
				} else {
					m.status = T("expected a position number")
				}
			case len(f) == 2 && (f[0] == "before" || f[0] == "after"):
				m.runToStatus("move", ref, "--"+f[0], f[1])
			default:
				m.status = T("expected: <n>, before <ref> or after <ref>")
			}
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
			if m.missing > 0 {
				out += Tf("problems: %s in the list (kept in ModsConfig.xml)\n", plural(m.missing, "missing entry"))
			}
		}
		m.openPanel(T("save to game — preview"), out, confirm)
		return nil
	case "u":
		m.runToStatus("undo")
		return m.requestDetail()
	case "ctrl+r":
		m.runToStatus("redo")
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
		if r := m.current(); r != nil && r.id != "" {
			out, code := m.run("swap", r.ref)
			if code == cmds.ExitOK {
				m.status = firstLine(out)
			} else {
				m.openPanel("swap "+r.ref, out, nil)
			}
		}
		return m.requestDetail()
	case "enter":
		if r := m.current(); r != nil && r.id != "" {
			out, _ := cmds.Execute(m.app, []string{"mod", r.ref})
			m.inspect = strings.Split(strings.TrimRight(out, "\n"), "\n")
			m.inspectHist, m.inspectOff = nil, 0
		}
		return nil
	case "r":
		r := m.current()
		if r == nil || r.id == "" {
			return nil
		}
		ref := r.ref
		m.prompt = &prompt{label: Tf("rule for %s (after|before|attach|incompatible <ref>): ", ref), submit: func(m *model, text string) tea.Cmd {
			f := strings.Fields(text)
			if len(f) != 2 {
				m.status = T("expected: <kind> <ref>")
				return nil
			}
			m.runToStatus("rule", "add", f[0], ref, f[1])
			return nil
		}}
		return nil
	}
	return nil
}

func (m *model) curID() string {
	if r := m.current(); r != nil {
		return r.id
	}
	return ""
}

// toggle enables or disables the selection (space).
func (m *model) toggle() tea.Cmd {
	refs := m.selection()
	if len(refs) == 0 {
		return nil
	}
	r := m.current()
	verb := "enable"
	if r != nil && r.pos > 0 && len(m.marked) == 0 {
		verb = "disable"
	} else if len(m.marked) > 0 {
		// Multi-select: disable when every marked mod is enabled.
		all := true
		for _, row := range m.rows {
			if m.marked[row.id] && row.pos == 0 {
				all = false
			}
		}
		if all {
			verb = "disable"
		}
	}
	m.runToStatus(append([]string{verb}, refs...)...)
	m.marked = map[string]bool{}
	return m.requestDetail()
}

func (m *model) searchKey(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEnter:
		m.editing = false
	case tea.KeyEsc:
		m.editing, m.search = false, ""
	case tea.KeyBackspace:
		if r := []rune(m.search); len(r) > 0 {
			m.search = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.search += string(k.Runes)
	case tea.KeyUp:
		return m.moveCursor(-1)
	case tea.KeyDown:
		return m.moveCursor(1)
	default:
		return nil
	}
	m.refilter(m.curID())
	return m.requestDetail()
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
				m.status = firstLine(out)
			} else {
				m.openPanel("archo "+strings.Join(p.confirm, " "), out, nil)
			}
			m.detailRef = ""
			return m.requestDetail()
		}
	case "n", "esc", "q", "enter":
		m.panel = nil
		if p.confirm != nil {
			m.status = T("cancelled")
		}
	}
	return nil
}

func (m *model) inspectKey(s string) tea.Cmd {
	page := max(m.h-3, 1)
	switch s {
	case "esc", "backspace", "q":
		if n := len(m.inspectHist); n > 0 {
			m.inspect, m.inspectHist = m.inspectHist[n-1], m.inspectHist[:n-1]
			m.inspectOff = 0
		} else {
			m.inspect = nil
		}
	case "down", "j":
		m.inspectOff = min(m.inspectOff+1, max(len(m.inspect)-page, 0))
	case "up", "k":
		m.inspectOff = max(m.inspectOff-1, 0)
	case "enter":
		// Follow the first reference on the top visible line (e.g. lineage members).
		if ref := firstRef(m.inspect[min(m.inspectOff+1, len(m.inspect)-1)]); ref != "" {
			out, code := cmds.Execute(m.app, []string{"mod", ref})
			if code == cmds.ExitOK || code == cmds.ExitAmbiguous {
				m.inspectHist = append(m.inspectHist, m.inspect)
				m.inspect, m.inspectOff = strings.Split(strings.TrimRight(out, "\n"), "\n"), 0
			}
		}
	}
	return nil
}

func firstRef(line string) string {
	for _, f := range strings.Fields(line) {
		f = strings.Trim(f, "(),[]")
		if strings.HasPrefix(f, "#") && len(f) >= 5 {
			return f
		}
	}
	return ""
}

func (m *model) tabKey(s string) tea.Cmd {
	page := max(m.h-4, 1)
	switch s {
	case "down", "j":
		m.tabOff = min(m.tabOff+1, max(len(m.tabText)-page, 0))
	case "up", "k":
		m.tabOff = max(m.tabOff-1, 0)
	case "pgdown":
		m.tabOff = min(m.tabOff+page, max(len(m.tabText)-page, 0))
	case "pgup":
		m.tabOff = max(m.tabOff-page, 0)
	}
	return nil
}

// loadTab fills the Problems / Changes views. Problems are analysed in M3; until then the
// view shows what the Loadout itself can tell (sort cycles, missing entries).
func (m *model) loadTab() {
	m.tabOff = 0
	var parts []string
	switch m.tab {
	case tabProblems:
		parts = append(parts, "Problems — full analysis arrives with M3 (dependencies, incompatibilities, defs, patches).", "")
		if m.missing > 0 {
			parts = append(parts, Tf("warning: %s in the Loadout match no installed mod", plural(m.missing, "entry")), "")
		}
		out, _ := cmds.Execute(m.app, []string{"sort", "--dry-run"})
		parts = append(parts, strings.Split(strings.TrimRight(out, "\n"), "\n")...)
	case tabChanges:
		if m.question > 0 {
			out, _ := cmds.Execute(m.app, []string{"identity", "list"})
			parts = append(parts, strings.Split(strings.TrimRight(out, "\n"), "\n")...)
			parts = append(parts, "")
		}
		out, _ := cmds.Execute(m.app, []string{"changes"})
		parts = append(parts, strings.Split(strings.TrimRight(out, "\n"), "\n")...)
		if m.st != nil && m.st.WS.Conflict != 0 {
			parts = append(parts, "", Tf("ModsConfig.xml changed outside RimArcho (v%d) — :loadout checkout v%d to load it", m.st.WS.Conflict, m.st.WS.Conflict))
		}
	}
	m.tabText = parts
}

func (m *model) chooseEnv() tea.Cmd {
	envs, err := m.app.Store.Envs()
	if err != nil || len(envs) == 0 {
		m.status = T("no environments (archo env add)")
		return nil
	}
	p := &panel{title: T("switch environment (enter to choose)")}
	for i, e := range envs {
		p.choices = append(p.choices, e.Name)
		if e.Name == m.envName {
			p.choice = i
		}
	}
	p.onPick = func(m *model, name string) tea.Cmd {
		m.app.EnvName = ""
		m.runToStatus("env", "use", name)
		m.cursor, m.off, m.detailRef = 0, 0, ""
		return m.requestDetail()
	}
	m.panel = p
	return nil
}
