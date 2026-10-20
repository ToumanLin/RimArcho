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
	case m.pal != nil:
		content = m.paletteLines(body)
	case m.panel != nil:
		content = m.panelLines(body)
	case m.insp != nil:
		content = m.inspectLines(body)
	case m.tab == tabProblems:
		m.problemCounts() // reloads the items when the database changed (an edit, a rule, an answer)
		content = m.needLines(body)
	case m.tab == tabChanges:
		content = m.changeLines(body)
	case m.full:
		content, ps = m.fullLines(body)
	case m.pos != nil:
		// The position input sits under the columns, which give it their last lines.
		sheet := m.posLines(m.w)
		content, ps = m.columnLines(max(body-len(sheet)-1, 3))
		content = append(append(content, ""), sheet...)
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

// topBar: brand · environment pill · needs attention · unsaved, and on the right the
// position line (06 §13.3) and launch. Secondary screens show a breadcrumb that is the
// position line.
func (m *model) topBar() string {
	left := sBold.Render("RimArcho")
	lw := len("RimArcho")
	add := func(s string, w int) { left += s; lw += w }
	pos := m.position()
	if m.full || m.insp != nil || m.tab != tabList {
		c := "  › " + pos
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
	launch := T("launch") + " " + ic.play
	launchW := width.StringWidth(launch) + 2 // "L "
	pw := width.StringWidth(pos)
	room := m.w - lw - 3 // at least three cells between the left side and the position
	right, rw := "", 0
	if pw+3+launchW <= room {
		right = m.renderPos(pos) + "   " + sText.Render("L") + " " + sSecond.Render(launch)
		rw = pw + 3 + launchW
	} else if room >= 8 {
		pw = min(pw, room)
		right, rw = m.renderPos(fit(pos, pw)), pw
	}
	return left + strings.Repeat(" ", max(m.w-lw-rw, 0)) + right
}

// renderPos styles the position line: the area in the accent colour, the rest secondary,
// an open popup in the text colour.
func (m *model) renderPos(p string) string {
	head, rest := p, ""
	if i := strings.Index(p, " "+markDot+" "); i >= 0 {
		head, rest = p[:i], p[i:]
	}
	pop := ""
	if i := strings.Index(rest, " › "); i >= 0 {
		rest, pop = rest[:i], rest[i:]
	} else if i := strings.Index(head, " › "); i >= 0 {
		head, pop = head[:i], head[i:]
	}
	return sAccent.Render(head) + sSecond.Render(rest) + sText.Render(pop)
}

// statusLine: one sentence, ✓ done (green), ✗ failed (red), … in progress (06 §9).
func (m *model) statusLine() string {
	if m.tags != nil {
		return m.tagBarLine(m.w)
	}
	text, kind := m.status, m.sayKind
	if m.editing && text == "" {
		if l := m.searchLine(); l != "" {
			return sSecond.Render(fit(l, m.w))
		}
	}
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

// hintLine: "key label · key label", only what the object can do now, at most 7 (06 §9,
// §13.2 rule 4); ? lists everything.
func (m *model) hintLine() string {
	var pairs [][2]string
	prefix := ""
	switch {
	case m.pal != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("run")}, {"esc", T("close")}}
	case m.pos != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("move")}, {"tab", T("before / after")}, {"esc", T("close")}}
	case m.prompt != nil:
		return sAccent.Render(markPrompt+" ") + sSecond.Render(m.prompt.label) + sText.Render(m.prompt.text+"_")
	case m.editing:
		pairs = [][2]string{{"enter", T("done")}, {"esc", T("clear")}, {"tab", T("next list with matches")}, {"↑↓", T("move")}}
	case m.panel != nil && m.panel.choices != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("confirm")}, {"esc", T("back")}}
	case m.tags != nil:
		pairs = [][2]string{{"←→", T("choose")}, {"enter", T("apply")}, {"esc", T("close")}, {"←", T("type a new tag")}}
	case m.panel != nil && m.panel.confirm != nil:
		pairs = [][2]string{{"y", T("confirm")}, {"n", T("cancel")}, {"↑↓", T("scroll")}}
	case m.panel != nil:
		pairs = [][2]string{{"esc", T("back")}, {"↑↓", T("scroll")}}
	case m.insp != nil:
		pairs = [][2]string{{"esc", T("back")}, {"backspace", T("previous page")}, {"tab", T("section")}, {"↑↓", T("scroll")}}
	case m.full:
		pairs = [][2]string{{"↑↓", T("scroll")}, {"←→", T("previous / next mod")}, {"tab", T("sections")}, {"#", T("tags")}, {"o", T("Workshop page")}, {"esc", T("back")}, {"?", T("all keys")}}
	case m.tab == tabProblems:
		pairs = [][2]string{{"↑↓", T("choose")}, {"esc", T("back")}, {"l", T("choose a log")}, {"A", T("check the files again")}} // each item shows its own keys
	case m.tab == tabChanges:
		pairs = [][2]string{{"esc", T("back")}, {"enter", T("file changes")}}
	case m.selCount() > 0:
		// 06 §13.5: what the selection can do, and that another list drops it.
		prefix = Tf("selected %d", m.selCount())
		pairs = append(m.verbPairs(), [2]string{"esc", T("clear the selection")}, [2]string{"←→", T("another list clears it")})
	case m.detailFocus:
		pairs = [][2]string{{"↑↓", T("scroll")}, {"←", T("back to the list")}, {"enter", T("full screen")}, {"#", T("tags")}, {"o", T("Workshop page")}, {"?", T("all keys")}}
	default:
		pairs = append(m.verbPairs(), [2]string{"/", T("search")})
		if m.layout().tabs {
			pairs = append([][2]string{{"[ ]", T("lists")}}, pairs...) // the only way to the other lists
		} else if m.layout().detailW == 0 {
			pairs = append(pairs, [2]string{"enter", T("details")})
		}
		pairs = append(pairs, [2]string{"?", T("all keys")})
	}
	if len(pairs) > 7 {
		pairs = append(pairs[:6], pairs[len(pairs)-1])
	}
	var b strings.Builder
	used := 0
	if prefix != "" {
		b.WriteString(sAccent.Render(prefix))
		used += width.StringWidth(prefix)
	}
	for i, p := range pairs {
		sep := ""
		if i > 0 || prefix != "" {
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

// verbPairs: the verbs the focused list's object can use now (06 §13.6).
func (m *model) verbPairs() [][2]string {
	if m.col().cur() == nil {
		return nil
	}
	switch m.focus {
	case cInactive:
		return [][2]string{{"space", T("enable")}, {"m", T("enable at…")}, {"t", T("stage")}, {"#", T("tags")}}
	case cActive:
		return [][2]string{{"space", T("disable")}, {"J K", T("move")}, {"m", T("move to…")}, {"t", T("stage")}, {"#", T("tags")}}
	}
	ps := [][2]string{{"space", T("put back")}, {"m", T("enable at…")}, {"#", T("tags")}}
	if m.selCount() == 0 {
		ps = append(ps, [2]string{"T", T("put all back")})
	}
	return ps
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

// descMax is how many description lines the detail column shows (06 §6.1): all of them
// once it is focused or scrolled.
func (m *model) descMax() int {
	if m.detailFocus || m.detailOff > 0 {
		return 0
	}
	return 12
}

// detailLines draws the detail column (w wide, n lines, starting at 0-based column x) and
// returns its image placements. The first line is the area title, "4 details", or the
// mod's name once scrolled; in the accent colour when area 4 has the focus (06 §13.3).
func (m *model) detailLines(w, n, x int) ([]string, []placement) {
	m.syncDetailOff()
	lines := m.detailLayout(detailOpts{width: w, descMax: m.descMax(), images: m.inlineImages(), techText: m.detail})
	m.lastLines = lines
	// The scroll is kept as asked until the mod's details have loaded: PgDn pressed while
	// the description is still on its way would otherwise be lost.
	off := max(0, min(m.detailOff, len(lines)-(n-1)))
	if it := m.current(); it == nil || (m.dd != nil && m.dd.id == it.id) {
		m.detailOff = off
	}
	title := T("details")
	if off > 0 {
		if it := m.current(); it != nil {
			title = "▲ " + it.name
		}
	}
	st := sDimS
	if m.detailFocus {
		st = sAccent
	}
	out := []string{sDimS.Render("4 ") + st.Render(fit(title, w-2))}
	var ps []placement
	for i := off; i < len(lines) && len(out) < n; i++ {
		out = append(out, lines[i].text)
		if p, ok := m.place(lines[i], len(out)-1, n, x+1); ok { // x is 0-based
			ps = append(ps, p)
		}
	}
	return out, ps
}

var _ = fmt.Sprint
