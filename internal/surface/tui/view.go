package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Screen structure (06 §4.2): top bar, blank, body, blank, status line, hint line.

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	h := max(m.h, 6)
	lines := make([]string, h)
	lines[0] = m.topBar()
	body := h - 4 // rows 2 … h-3
	var content []string
	var ps []placement
	switch {
	case m.loadErr != nil:
		content = []string{"", "  " + sErrS.Render(fit(T("error: ")+m.loadErr.Error(), m.w-2)),
			"  " + sSecond.Render(fit(T("Set up an environment first: archo env detect, archo env add <name> --detected"), m.w-2))}
	case m.panel != nil:
		content = m.panelLines(body)
	case m.insp != nil:
		content = m.inspectLines(body)
	case m.tab == tabProblems:
		m.problemCounts() // reloads the list when the database changed (a rule, an ignore)
		content = m.problemLines(body)
	case m.tab == tabChanges:
		content = m.changeLines(body)
	case m.full:
		content, ps = m.fullLines(body)
	default:
		content, ps = m.columnLines(body)
	}
	if m.images != nil {
		m.images.set(ps) // an empty list clears the images of the previous screen
	}
	for i := 0; i < body && i < len(content); i++ {
		lines[2+i] = content[i]
	}
	lines[h-2] = m.statusLine()
	lines[h-1] = m.hintLine()
	for i, l := range lines {
		if lipgloss.Width(l) > m.w {
			lines[i] = width.Truncate(l, m.w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// topBar: brand · environment pill · needs attention · unsaved … launch (06 §4.2); a
// breadcrumb on secondary screens.
func (m *model) topBar() string {
	left := sBold.Render("RimArcho")
	lw := len("RimArcho")
	add := func(s string, w int) { left += s; lw += w }
	if crumb := m.crumb(); crumb != "" {
		c := "  › " + crumb
		cw := width.StringWidth(c)
		add(sSecond.Render(fit(c, min(cw, m.w-lw))), min(cw, m.w-lw))
		return left + strings.Repeat(" ", max(m.w-lw, 0))
	}
	envText := T("no environment")
	if m.envName != "" {
		envText = m.envName
		if mm := env.MajorMinor(m.gameVer); mm != "" {
			envText += " " + markDot + " " + mm
		}
	}
	pill := " " + envText + " "
	add("  ", 2)
	add(sPill.Render(pill), width.StringWidth(pill))
	if n := m.needCount(); n > 0 {
		t := Tf("needs attention %d", n)
		add("  "+sWarnS.Render(t), 2+width.StringWidth(t))
	}
	if m.unsaved > 0 {
		t := Tf("unsaved %d", m.unsaved)
		add("  "+sAccent.Render(ic.unsaved+" "+t), 2+2+width.StringWidth(t))
	}
	right := T("launch") + " " + ic.play
	rw := width.StringWidth(right) + 2
	if lw+rw+2 > m.w {
		return left + strings.Repeat(" ", max(m.w-lw, 0))
	}
	return left + strings.Repeat(" ", m.w-lw-rw) + sText.Render("L") + " " + sSecond.Render(right)
}

// crumb is the breadcrumb of a secondary screen ("" on the main screen).
func (m *model) crumb() string {
	switch {
	case m.panel != nil:
		return m.panel.title
	case m.insp != nil || m.full:
		if it := m.current(); it != nil {
			return m.focus.title() + " › " + it.name
		}
		return T("details")
	case m.tab == tabProblems:
		return T("needs attention")
	case m.tab == tabChanges:
		return T("recent changes")
	}
	return ""
}

// statusLine: one sentence, ✓ done (green), ✗ failed (red), … in progress (06 §9).
func (m *model) statusLine() string {
	if m.tagging != nil && m.prompt != nil {
		return sSecond.Render(fit(m.tagStatus(), m.w))
	}
	text, kind := m.status, m.sayKind
	if text == "" && m.scanning {
		text, kind = T("scanning…"), sayBusy
	}
	if text == "" && m.analyzing {
		text, kind = T("analyzing…"), sayBusy
	}
	if text == "" {
		return ""
	}
	var mark string
	var ms = sSecond
	switch kind {
	case sayOK:
		mark, ms = markOK, sOK
	case sayFail:
		mark, ms = ic.fail, sErrS
	case sayBusy:
		mark = markBusy
	}
	if mark == "" {
		return sSecond.Render(fit(text, m.w))
	}
	return ms.Render(mark) + " " + sText.Render(fit(text, m.w-2))
}

// hintLine: "key label · key label" for the current place (06 §9).
func (m *model) hintLine() string {
	var pairs [][2]string
	switch {
	case m.prompt != nil:
		return sAccent.Render(markPrompt+" ") + sSecond.Render(m.prompt.label) + sText.Render(m.prompt.text+"_")
	case m.editing:
		pairs = [][2]string{{"enter", T("done")}, {"esc", T("clear")}, {"↑↓", T("move")}}
	case m.panel != nil && m.panel.choices != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("confirm")}, {"esc", T("back")}}
	case m.panel != nil && m.panel.confirm != nil:
		pairs = [][2]string{{"y", T("confirm")}, {"n", T("cancel")}, {"↑↓", T("scroll")}}
	case m.panel != nil:
		pairs = [][2]string{{"esc", T("back")}, {"↑↓", T("scroll")}}
	case m.insp != nil:
		pairs = [][2]string{{"esc", T("back")}, {"tab", T("section")}, {"↑↓", T("scroll")}}
	case m.full:
		pairs = [][2]string{{"esc", T("back")}, {"↑↓", T("scroll")}, {"1–5", T("sections")}, {"#", T("tags")}, {"o", T("Workshop page")}, {"r", T("refresh")}, {"i", T("technical information")}, {"tab", T("technical pages")}}
	case m.tab == tabProblems:
		pairs = [][2]string{{"esc", T("back")}, {"enter", T("expand")}, {"a", T("run the suggestion")}, {"i", T("ignore")}, {"l", T("choose a log")}, {"A", T("re-analyze")}}
	case m.tab == tabChanges:
		pairs = [][2]string{{"esc", T("back")}, {"enter", T("file changes")}}
	default:
		switch m.focus {
		case cInactive:
			pairs = [][2]string{{"space", T("enable")}, {"t", T("stage")}, {"/", T("filter")}, {"←→", T("columns")}}
		case cActive:
			pairs = [][2]string{{"space", T("disable")}, {"J K", T("move")}, {"t", T("stage")}, {"S", T("sort")}, {"w", T("save")}, {"u", T("undo")}}
		case cStaging:
			pairs = [][2]string{{"space", T("put back")}, {"T", T("put all back")}, {"t", T("unstage")}, {"←→", T("columns")}}
		}
		if m.layout().tabs {
			pairs = append([][2]string{{"[ ]", T("lists")}}, pairs...) // the only way to the other lists
		} else if m.layout().detailW == 0 {
			pairs = append(pairs, [2]string{"enter", T("details")})
		}
		pairs = append(pairs, [2]string{"!", T("needs attention")}, [2]string{"?", T("help")})
	}
	var b strings.Builder
	used := 0
	for i, p := range pairs {
		sep := ""
		if i > 0 {
			sep = "  " + markDot + "  "
		}
		w := width.StringWidth(sep + p[0] + " " + p[1])
		if used+w > m.w {
			break
		}
		if sep != "" {
			b.WriteString(sDimS.Render(sep))
		}
		b.WriteString(sText.Render(p[0]) + " " + sSecond.Render(p[1]))
		used += w
	}
	return b.String()
}

// panelLines draws a secondary screen without a frame: the text, indented, or a chooser.
func (m *model) panelLines(n int) []string {
	p := m.panel
	inner := max(m.w-4, 10)
	var out []string
	if p.choices != nil {
		for i, c := range p.choices {
			if i == p.choice {
				out = append(out, "  "+sAccent.Render(markSel+" ")+sSel.Render(fit(c, inner-2)))
			} else {
				out = append(out, "    "+sText.Render(fit(c, inner-2)))
			}
		}
		return out
	}
	rows := max(n-2, 1)
	for _, l := range window(p.lines, p.off, rows) {
		out = append(out, "  "+sText.Render(width.Truncate(m.humanize(l), inner, "…")))
	}
	if len(p.lines) > rows {
		out = append(out, "", "  "+sDimS.Render(Tf("%d–%d of %d", p.off+1, min(p.off+rows, len(p.lines)), len(p.lines))))
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

// descMax is how many description lines the detail column shows (06 §6.1).
func (m *model) descMax() int { return 12 }

// detailLines draws the detail column (w wide, n lines, starting at 0-based column x) and
// returns its image placements.
func (m *model) detailLines(w, n, x int) ([]string, []placement) {
	lines := m.detailLayout(detailOpts{width: w, descMax: m.descMax(), images: m.inlineImages(), techText: m.detail})
	m.lastLines = lines
	var out []string
	var ps []placement
	for i := 0; i < len(lines) && i < n; i++ {
		out = append(out, lines[i].text)
		if p, ok := m.place(lines[i], i, n, x+1); ok { // x is 0-based
			ps = append(ps, p)
		}
	}
	return out, ps
}

var _ = fmt.Sprint
