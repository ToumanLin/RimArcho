package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/source"
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
	default:
		content = m.columnLines(body)
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
	case m.insp != nil:
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

// detailLines: the detail column (06 §6). V2 shows who the mod is and whether it has
// problems; the layered detail with the thumbnail is V3.
func (m *model) detailLines(w, n int) []string {
	it := m.current()
	var out []string
	add := func(s string) { out = append(out, s) }
	if it == nil {
		return nil
	}
	if it.missing {
		add(sBold.Render(fit(it.name, w)))
		for _, l := range wrap(T("listed in ModsConfig.xml, but not installed"), w) {
			add(sSecond.Render(fit(l, w)))
		}
		return out
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil {
		return nil
	}
	for _, l := range wrap(it.name, w) {
		add(sBold.Render(fit(l, w)))
	}
	if a := strings.TrimSpace(mod.Authors); a != "" {
		add(sSecond.Render(fit(a, w)))
	}
	facts := []string{sourceText(mod)}
	switch k := mod.Kind; {
	case k == source.Official:
	default:
		if v := strings.Fields(mod.SupportedVersions); len(v) > 0 {
			facts = append(facts, T("supports")+" "+strings.Join(v, " "))
		}
	}
	switch m.focus {
	case cActive:
		facts = append(facts, Tf("number %d", it.pos))
	case cStaging:
		facts = append(facts, T("staged"))
	default:
		facts = append(facts, T("not enabled"))
	}
	line := it.icon + " " + strings.Join(facts, " "+markDot+" ")
	for _, l := range wrap(line, w) {
		add(sDimS.Render(fit(l, w)))
	}
	add("")
	switch s := m.rowStatus(it); s {
	case statusNone:
		add(sOK.Render(markOK) + " " + sSecond.Render(fit(T("no problems found"), w-2)))
	case statusUnloadable:
		why := T("the game cannot load it")
		if mod.Unloadable != "" {
			why = T("About.xml is renamed: the game cannot load it")
		} else if mod.Residue {
			why = T("no longer subscribed: the game ignores this folder")
		}
		for _, l := range wrap(why, w-2) {
			add(statusIcon(s) + " " + sSecond.Render(fit(l, w-2)))
		}
	default:
		add(statusIcon(s) + " " + sText.Render(fit(T("needs attention · ! shows what"), w-2)))
	}
	add("")
	if !m.detailTech {
		add(sDimS.Render(fit(T("technical information · i"), w)))
		return out
	}
	add(sDimS.Render(fit(T("technical information"), w)))
	for _, l := range m.detail {
		for _, x := range wrap(l, w-2) {
			if len(out) >= n {
				return out
			}
			add("  " + sSecond.Render(fit(x, w-2)))
		}
	}
	return out
}

var _ = fmt.Sprint
