package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The row menu (16 §8.5): the same groups and order on the editor and Mod library, each op
// with its key on the right, only the ops that apply, unsubscribe / delete last in red. The menu key
// or shift+F10 opens it at the cursor's row, a right click at the pointer; ↑↓ choose, enter
// runs, esc closes.
//
// What terminals send: xterm-style terminals send the menu key as F16 (CSI 29 ~) and
// shift+F10 as CSI 21;2 ~, which Bubble Tea v1.3 does not name; the Windows console reports
// F10 without its shift. So F10, shift+F10 and F16 all open the menu.
var menuKeys = []string{"f10", "shift+f10", "f16"}

type menuItem struct {
	code, label, key string
	sep, danger      bool
}

type rowMenu struct {
	items []menuItem
	sel   int
	x, y  int // the top left cell on the screen (0-based)
	run   func(m *model, code string) tea.Cmd
}

// openMenu opens the menu of codes at the screen cell x, y.
func (m *model) openMenu(codes []string, x, y int, run func(m *model, code string) tea.Cmd) tea.Cmd {
	if len(codes) == 0 {
		m.say(sayInfo, T("nothing to do with this row"))
		return nil
	}
	mn := &rowMenu{x: x, y: y, run: run, sel: -1}
	group := 0
	for _, o := range view.OpTable() {
		if !slices.Contains(codes, o.Code) {
			continue
		}
		if group != 0 && o.Group != group {
			mn.items = append(mn.items, menuItem{sep: true})
		}
		group = o.Group
		mn.items = append(mn.items, menuItem{code: o.Code, label: o.Label, key: o.Key, danger: o.Danger})
		if mn.sel < 0 {
			mn.sel = len(mn.items) - 1
		}
	}
	m.menu = mn
	return nil
}

// openRowMenu: the menu of the cursor's row, beside it.
func (m *model) openRowMenu() tea.Cmd {
	if m.tab == tabLibrary {
		return m.libMenu()
	}
	if m.tab != tabList {
		return nil
	}
	x, y := 6, 4
	l := m.layout()
	for _, s := range l.slots {
		if s.kind != m.focus {
			continue
		}
		x = s.x + 8
		for i, vi := range m.rowMap[s.kind] {
			if vi == m.col().cursor {
				y = bodyTop + m.barLines() + i + 1
				break
			}
		}
	}
	return m.openMenu(m.editorOps(), x, y, func(m *model, code string) tea.Cmd { return m.runEditorOp(code) })
}

func (mn *rowMenu) step(d int) {
	for i := mn.sel + d; i >= 0 && i < len(mn.items); i += d {
		if !mn.items[i].sep {
			mn.sel = i
			return
		}
	}
}

// menuKey: the keys of the open menu.
func (m *model) menuKey(s string) tea.Cmd {
	mn := m.menu
	switch {
	case s == "up" || s == "k" || s == "shift+tab":
		mn.step(-1)
	case s == "down" || s == "j" || s == "tab":
		mn.step(1)
	case s == "home":
		mn.sel = -1
		mn.step(1)
	case s == "end":
		mn.sel = len(mn.items)
		mn.step(-1)
	case s == "enter" || s == " ":
		m.menu = nil
		if mn.sel >= 0 && mn.sel < len(mn.items) {
			return mn.run(m, mn.items[mn.sel].code)
		}
	case s == "esc" || slices.Contains(menuKeys, s):
		m.menu = nil
	}
	return nil
}

// menuBox draws the menu: one line an item, the key at the right.
func (mn *rowMenu) box() []string {
	lw, kw := 0, 0
	for _, it := range mn.items {
		lw = max(lw, width.StringWidth(it.label))
		kw = max(kw, width.StringWidth(it.key))
	}
	inner := lw + 4 + kw
	out := []string{sDimS.Render("╭" + strings.Repeat("─", inner+2) + "╮")}
	for i, it := range mn.items {
		if it.sep {
			out = append(out, sDimS.Render("├"+strings.Repeat("─", inner+2)+"┤"))
			continue
		}
		st, ks := sText, sDimS
		if it.danger {
			st = sErrS
		}
		mark := " "
		if i == mn.sel {
			mark = markSel
			if !it.danger {
				st = sSel
			}
		}
		line := sAccent.Render(mark) + " " + st.Render(fit(it.label, lw)) + "    " + ks.Render(width.FillLeft(it.key, kw)) + " "
		out = append(out, sDimS.Render("│")+line+sDimS.Render("│"))
	}
	return append(out, sDimS.Render("╰"+strings.Repeat("─", inner+2)+"╯"))
}

// overlayMenu draws the open menu over the screen lines (it moves up or left to fit).
func (m *model) overlayMenu(lines []string) {
	box := m.menu.box()
	bw := lipglossWidth(box[0])
	x := max(0, min(m.menu.x, m.w-bw))
	y := max(1, min(m.menu.y, len(lines)-2-len(box)))
	for i, b := range box {
		r := y + i
		if r >= len(lines) {
			break
		}
		l := lines[r]
		if w := lipglossWidth(l); w < x {
			l += strings.Repeat(" ", x-w)
		}
		lines[r] = ansi.Truncate(l, x, "") + b + ansi.TruncateLeft(l, x+bw, "")
	}
}

// menuClick: a left click on an item runs it; anywhere else closes the menu.
func (m *model) menuClick(x, y int) tea.Cmd {
	mn := m.menu
	box := mn.box()
	bw := lipglossWidth(box[0])
	bx := max(0, min(mn.x, m.w-bw))
	by := max(1, min(mn.y, m.h-2-len(box)))
	i := y - by - 1
	m.menu = nil
	if x < bx || x >= bx+bw || i < 0 || i >= len(mn.items) || mn.items[i].sep {
		return nil
	}
	return mn.run(m, mn.items[i].code)
}

func lipglossWidth(s string) int { return ansi.StringWidth(s) }
