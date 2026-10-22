package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod 库存 (06 §2.9, §9.1), from the palette: the TUI shows the Lineage rows with the right
// column's size · state · used-in, and one filter line: / takes words, key:value keeps,
// -key:value leaves out (the GUI's filter column, typed). enter shows the Lineage's copies
// as `archo library --lineage` text; the actions are the CLI commands it lists.

type libState struct {
	page        *view.LibPage
	err         string
	cursor, off int
	query       string
	typing      bool
}

func (m *model) loadLibrary() {
	l := &m.libr
	l.err = ""
	lib, err := m.app.Library()
	if err != nil {
		l.page, l.err = nil, err.Error()
		return
	}
	if l.page, err = view.BuildLibrary(lib, l.query, nil, ""); err != nil {
		l.err = err.Error()
	}
	if l.page != nil && l.cursor >= len(l.page.Rows) {
		l.cursor = max(len(l.page.Rows)-1, 0)
	}
}

func (m *model) openLibrary() tea.Cmd {
	m.tab = tabLibrary
	m.loadLibrary()
	return nil
}

// libTypeKey edits the filter line; the list follows each key.
func (m *model) libTypeKey(k tea.KeyMsg) tea.Cmd {
	l := &m.libr
	switch k.Type {
	case tea.KeyEnter, tea.KeyEsc:
		l.typing = false
		if k.Type == tea.KeyEsc {
			l.query = ""
		}
	case tea.KeyBackspace:
		if r := []rune(l.query); len(r) > 0 {
			l.query = string(r[:len(r)-1])
		}
	case tea.KeyTab:
		// tab takes the grey completion (06 §3.7)
		if l.page == nil || l.page.Complete == "" {
			return nil
		}
		l.query += l.page.Complete
	case tea.KeySpace:
		l.query += " "
	case tea.KeyRunes:
		l.query += string(k.Runes)
	default:
		return nil
	}
	l.cursor, l.off = 0, 0
	m.loadLibrary()
	return nil
}

func (m *model) libraryKey(s string) tea.Cmd {
	l := &m.libr
	n := 0
	if l.page != nil {
		n = len(l.page.Rows)
	}
	page := max(m.h-6, 1)
	switch s {
	case "down", "j":
		l.cursor = min(l.cursor+1, max(n-1, 0))
	case "up", "k":
		l.cursor = max(l.cursor-1, 0)
	case "pgdown":
		l.cursor = min(l.cursor+page, max(n-1, 0))
	case "pgup":
		l.cursor = max(l.cursor-page, 0)
	case "home", "g":
		l.cursor = 0
	case "end", "G":
		l.cursor = max(n-1, 0)
	case "/":
		l.typing = true
	case "enter":
		if l.cursor < n {
			r := l.page.Rows[l.cursor]
			out, _ := cmds.Execute(m.app, []string{"library", "--lineage", r.Ref})
			m.openPanel(r.Name, out)
		}
	}
	return nil
}

func (m *model) libraryLines(n int) []string {
	l := &m.libr
	q := l.query
	ghost := ""
	if l.typing {
		if l.page != nil {
			ghost = l.page.Complete
		}
		if ghost == "" {
			ghost = "▌"
		}
	}
	// the typed text, then the grey completion tab takes (or the cursor mark)
	head := fit("/ "+q, m.w) + sDimS.Render(ghost)
	if ghost != "" {
		head = "/ " + q + sDimS.Render(ghost)
	}
	if q == "" && !l.typing {
		head = sDimS.Render(fit("/ "+T("search: words, tag:, state:, source:, used:, -not"), m.w))
	}
	out := []string{head}
	switch {
	case l.err != "":
		return append(out, sErrS.Render(fit(l.err, m.w)))
	case l.page == nil:
		return out
	}
	p := l.page
	count := Tf("%d mods", p.Total)
	if p.Shown != p.Total {
		count = Tf("%d of %d mods", p.Shown, p.Total)
	}
	out = append(out, sDimS.Render(fit(count+"  "+markDot+"  "+p.Size, m.w)))
	if len(p.Rows) == 0 {
		return append(out, sSecond.Render(T("no mod matches")))
	}
	rows := max(n-len(out), 1)
	if l.cursor < l.off {
		l.off = l.cursor
	}
	if l.cursor >= l.off+rows {
		l.off = l.cursor - rows + 1
	}
	right := min(44, m.w/2)
	for i := l.off; i < len(p.Rows) && len(out) < n; i++ {
		r := p.Rows[i]
		// The table's default columns (06 §2.9): name, size, state, source, used in.
		name := r.Name
		if !r.Single {
			name += "  " + Tf("%d copies", r.Copies)
		}
		info := r.Size
		for _, c := range []string{r.State, r.Source, r.Used} {
			if c != "" {
				info += "  " + c
			}
		}
		line := fit(name, m.w-right-2) + "  " + fit(info, right)
		if i == l.cursor {
			line = sSel.Render(line)
		}
		out = append(out, line)
	}
	return out
}
