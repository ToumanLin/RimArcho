package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/rimarcho/rimarcho/internal/env"
)

// width uses the same East Asian setting as the CLI renderer (07 §6.1 item 3).
var width = &runewidth.Condition{EastAsianWidth: false}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return width.FillRight(width.Truncate(s, w, "…"), w)
}

var (
	stSel      = lipgloss.NewStyle().Reverse(true)
	stHeader   = lipgloss.NewStyle().Bold(true)
	stDim      = lipgloss.NewStyle().Faint(true)
	stOn       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	stWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	stErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stTabOn    = lipgloss.NewStyle().Bold(true).Underline(true)
	stPanelBox = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
)

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.headerLine() + "\n")
	body := m.h - 3
	var lines []string
	switch {
	case m.loadErr != nil:
		lines = []string{stErr.Render(T("error: ") + m.loadErr.Error()), "", T("Set up an environment first: archo env detect, archo env add <name> --detected")}
	case m.inspect != nil:
		lines = window(m.inspect, m.inspectOff, body)
	case m.tab != tabList:
		lines = window(m.tabText, m.tabOff, body)
	default:
		lines = m.listLines(body)
	}
	for i := 0; i < body; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		b.WriteString(l + "\n")
	}
	b.WriteString(m.statusLine() + "\n")
	b.WriteString(m.footerLine())
	out := b.String()
	if m.panel != nil {
		out = m.overlay(out)
	}
	return out
}

func window(lines []string, off, n int) []string {
	if off > len(lines) {
		off = len(lines)
	}
	end := min(off+n, len(lines))
	return lines[off:end]
}

func (m *model) headerLine() string {
	envPart := T("no environment")
	if m.envName != "" {
		envPart = T("env: ") + m.envName + " " + env.MajorMinor(m.gameVer)
	}
	tabs := []string{T("[1]list"), T("[2]problems"), T("[3]changes")}
	for i := range tabs {
		if i == m.tab {
			tabs[i] = stTabOn.Render(tabs[i])
		}
	}
	left := "RimArcho ─ " + envPart
	right := strings.Join(tabs, " ")
	pad := m.w - width.StringWidth(left) - lipgloss.Width(right)
	return stHeader.Render(left) + strings.Repeat(" ", max(pad, 1)) + right
}

func (m *model) statusLine() string {
	active := 0
	for _, r := range m.rows {
		if r.pos > 0 {
			active++
		}
	}
	parts := []string{Tf("active %d/%d", active, len(m.rows))}
	if m.unsaved > 0 {
		parts = append(parts, stWarn.Render(Tf("unsaved %d", m.unsaved)))
	} else {
		parts = append(parts, T("saved"))
	}
	parts = append(parts, T("problems —"))
	parts = append(parts, Tf("changes %d", m.changes))
	if m.question > 0 {
		parts = append(parts, stWarn.Render(Tf("questions %d", m.question)))
	}
	if m.st != nil {
		if v := m.st.FileVersionMismatch(); v != "" {
			parts = append(parts, stErr.Render(Tf("list saved for %s: game resets it (w to save)", env.MajorMinor(v))))
		}
		if m.st.WS.Conflict != 0 {
			parts = append(parts, stWarn.Render(Tf("external change v%d", m.st.WS.Conflict)))
		}
	}
	if len(m.marked) > 0 {
		parts = append(parts, Tf("marked %d", len(m.marked)))
	}
	if m.scanning {
		parts = append(parts, T("scanning…"))
	}
	return fit(strings.Join(parts, " · "), m.w)
}

func (m *model) footerLine() string {
	switch {
	case m.prompt != nil:
		return m.prompt.label + m.prompt.text + "_"
	case m.editing:
		return "/" + m.search + "_"
	case m.status != "":
		return fit(m.status, m.w)
	case m.search != "":
		return fit(T("search: ")+m.search+T("  (esc clears)"), m.w)
	}
	return stDim.Render(fit(T(": command   / search   space toggle   S sort   w save   L launch   ? help   q quit"), m.w))
}

// listLines renders the list (and the detail pane when wide enough).
func (m *model) listLines(n int) []string {
	listW := m.w
	if m.showDetail() {
		listW = m.w * 55 / 100
	}
	const wPos, wSrc, wVer = 5, 9, 6
	wRef := 7
	wFlags := min(18, max(listW/6, 8))
	wName := max(listW-wPos-wRef-wSrc-wVer-wFlags-7, 8)
	line := func(mark, pos, ref, name, src, ver, flags string) string {
		return mark + fit(pos, wPos) + " " + fit(ref, wRef) + " " + fit(name, wName) + " " + fit(src, wSrc) + " " + fit(ver, wVer) + " " + fit(flags, wFlags)
	}
	out := []string{stHeader.Render(line(" ", "#", T("ref"), T("name"), T("source"), T("ver"), T("flags")))}
	end := min(m.off+n-1, len(m.view))
	for i := m.off; i < end; i++ {
		r := m.rows[m.view[i]]
		pos, dot := "—", "○"
		if r.pos > 0 {
			pos, dot = fmt.Sprint(r.pos), "●"
		}
		mark := " "
		switch {
		case m.marked[r.id]:
			mark = "+"
		case r.unsaved:
			mark = "*"
		}
		l := line(mark, pos+" "+dot, r.ref, r.name, r.source, r.version, trFlags(r.flags))
		switch {
		case i == m.cursor:
			l = stSel.Render(l)
		case r.flags == "missing":
			l = stErr.Render(l)
		case r.disabled:
			l = stDim.Render(l)
		case r.pos > 0:
			l = stOn.Render(l)
		}
		out = append(out, l)
	}
	if len(m.view) == 0 {
		out = append(out, stDim.Render(T("  (nothing matches; esc clears the search, f changes the filter)")))
	}
	if !m.showDetail() {
		return out
	}
	detailW := m.w - listW - 3
	for i := range out {
		d := ""
		if i < len(m.detail) {
			d = m.detail[i]
		}
		out[i] = padTo(out[i], listW) + " │ " + width.Truncate(d, detailW, "…")
	}
	for i := len(out); i < min(len(m.detail), n); i++ {
		out = append(out, strings.Repeat(" ", listW)+" │ "+width.Truncate(m.detail[i], detailW, "…"))
	}
	return out
}

func padTo(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// overlay draws the panel over the middle of the screen.
func (m *model) overlay(base string) string {
	p := m.panel
	inner := max(min(m.w-6, 100), 20)
	maxLines := max(m.h-8, 3)
	var lines []string
	lines = append(lines, stHeader.Render(width.Truncate(p.title, inner, "…")))
	if p.choices != nil {
		for i, c := range p.choices {
			l := "  " + c
			if i == p.choice {
				l = stSel.Render("> " + c)
			}
			lines = append(lines, l)
		}
	} else {
		for _, l := range window(p.lines, p.off, maxLines) {
			lines = append(lines, width.Truncate(l, inner, "…"))
		}
		if len(p.lines) > maxLines {
			lines = append(lines, stDim.Render(Tf("(%d–%d of %d, ↑/↓)", p.off+1, min(p.off+maxLines, len(p.lines)), len(p.lines))))
		}
		if p.confirm != nil {
			lines = append(lines, "", stWarn.Render("y: archo "+strings.Join(p.confirm, " ")+T("    n: cancel")))
		} else {
			lines = append(lines, "", stDim.Render(T("enter/esc: close")))
		}
	}
	box := stPanelBox.Width(inner + 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box, lipgloss.WithWhitespaceChars(" "))
}
