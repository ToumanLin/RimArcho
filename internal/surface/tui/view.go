package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Screen structure (16 §2.2, §9.1): navigation row, context row, blank, body, status line,
// hint line.

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	h := max(m.h, 6)
	lines := make([]string, h)
	lines[0] = m.topBar()
	lines[1] = m.contextRow()
	body := h - bodyTop - 2 // rows bodyTop … h-3
	var content []string
	switch {
	case m.loadErr != nil:
		content = []string{"", "  " + sErrS.Render(fit(T("error: ")+m.loadErr.Error(), m.w-2)),
			"  " + sSecond.Render(fit(T("Add a game first: archo env detect, archo env add <name> --detected"), m.w-2))}
		if windowNextToUs() {
			content = append(content, "  "+sSecond.Render(fit(T("Or open RimArcho.exe in this folder: the window has a guide that finds your games."), m.w-2)))
		}
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
	case m.tab == tabLibrary:
		content = m.libraryLines(body)
	case m.tab == tabSubscribe:
		content = m.addLines(body)
	case m.tab == tabSearch:
		content = m.fsLines(body)
	case m.tab == tabRules:
		content = m.rulesLines(body)
	case m.tab == tabHome:
		content = m.homeLines(body)
	case m.full:
		content = m.fullLines(body)
	case m.pos != nil:
		// The position input sits under the columns, which give it their last lines.
		sheet := m.posLines(m.w)
		bar := m.barView(m.w)
		content = m.columnLines(max(body-len(bar)-len(sheet)-1, 3))
		content = append(append(append(bar, content...), ""), sheet...)
	default:
		bar := m.barView(m.w)
		content = m.columnLines(max(body-len(bar), 3))
		content = append(bar, content...)
	}
	for i := 0; i < body && i < len(content); i++ {
		lines[bodyTop+i] = content[i]
	}
	lines[h-2] = m.statusLine()
	lines[h-1] = m.hintLine()
	if m.cmdOn {
		// the command line over the last lines: candidates, explanation, input (16 §9.4)
		cl := m.cmdLines()
		for i, l := range cl {
			if r := h - len(cl) + i; r >= 1 {
				lines[r] = l
			}
		}
	}
	if m.menu != nil {
		m.overlayMenu(lines)
	}
	for i, l := range lines {
		if lipgloss.Width(l) > m.w {
			lines[i] = width.Truncate(l, m.w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// positionText is the status line when it has nothing to say: where the player is (16 §2.2,
// §3.2), as in the GUI. Launch has none; a page shown as a panel, a technical page and the
// full detail show their place in the context row. A popup panel over a page (help, a
// preview) keeps the page's place with its own name after it.
func (m *model) positionText(w int) string {
	if m.tab == tabHome || (m.panel != nil && m.panel.nav > 0) || m.insp != nil || m.full || w < 8 {
		return ""
	}
	return m.renderPos(width.Truncate(m.position(), w, "…"))
}

// renderPos styles the position line: the area secondary, the place in it faint, the open
// popup (position's last " › " part) in the text colour.
func (m *model) renderPos(p string) string {
	pop := ""
	if name := m.popupName(); name != "" {
		if i := strings.LastIndex(p, " › "+name); i >= 0 {
			p, pop = p[:i], p[i:]
		}
	}
	head, rest := p, ""
	if i := strings.Index(p, "  "); i >= 0 {
		head, rest = p[:i], p[i:]
	}
	return sSecond.Render(head) + sDimS.Render(rest) + sText.Render(pop)
}

// statusLine: the sentence (statusText) and, on its right, the running game while there is
// something to say (16 §9.1; the sidebar's foot in the GUI): "⬤ game running since 14:05"
// with this run's errors in amber, or after the game "⬤ analyse this run's log". The dot is
// the unsaved mark's one-cell glyph (16 §7), not ●, which is two cells in some terminals.
func (m *model) statusLine() string {
	if m.tags != nil {
		return m.tagBarLine(m.w)
	}
	live, lw := m.liveStatus()
	if live == "" || lw+3 > m.w {
		return m.statusText(m.w)
	}
	left := m.statusText(m.w - lw - 3)
	return left + strings.Repeat(" ", max(m.w-lw-3-lipgloss.Width(left), 0)) + "   " + live
}

// liveStatus is the running game's part of the status line and its width; "" when there is
// nothing to say ("the game is not running" is not news).
func (m *model) liveStatus() (string, int) {
	l := m.live
	switch {
	case l != nil && l.Running:
		t := view.LiveText(l)
		s, w := sOK.Render(ic.unsaved)+" "+sSecond.Render(t), 2+width.StringWidth(t)
		if l.Errors > 0 {
			n := itoa(l.Errors)
			s, w = s+" "+sWarnS.Render(n), w+1+len(n)
		}
		return s, w
	case l != nil && l.NewLog != "":
		// 09 §2 "after the game exits": a new Player.log; Can it run (shift+2) analyses it
		return sAccent.Render(ic.unsaved+" "+l.NewLog) + sDimS.Render("  shift+2"), 2 + width.StringWidth(l.NewLog) + 9
	}
	return "", 0
}

// statusText is the status line's sentence in w cells: ✓ done (green), ✗ failed (red), …
// in progress (16 §6).
func (m *model) statusText(w int) string {
	text, kind := m.status, m.sayKind
	if m.editing && text == "" {
		if l := m.searchLine(); l != "" {
			return sSecond.Render(fit(l, w))
		}
	}
	if text == "" && m.scanning {
		text, kind = T("scanning…"), sayBusy
	}
	if text == "" && m.analyzing {
		text, kind = T("analyzing…"), sayBusy
	}
	if text == "" && m.cmdBusy != "" {
		text, kind = Tf("running archo %s…", m.cmdBusy), sayBusy
	}
	if text == "" {
		return m.positionText(w)
	}
	var mark string
	var ms = sSecond
	switch kind {
	case sayOK:
		mark, ms = markOK, sOK
	case sayFail:
		mark, ms = ic.fail, sErrS
	case sayWarn:
		mark, ms = ic.warn, sWarnS
	case sayBusy:
		mark = markBusy
	}
	if mark == "" {
		return sSecond.Render(fit(text, w))
	}
	return ms.Render(mark) + " " + sText.Render(fit(text, w-2))
}

// hintLine: "key label   key label", only what the object can do now, at most 7 (16 §6,
// §13.2 rule 4); ? lists everything and is never the one left out.
func (m *model) hintLine() string {
	var pairs [][2]string
	prefix := ""
	switch {
	case m.menu != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("run")}, {"esc", T("close")}}
	case m.pal != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("run")}, {"esc", T("close")}}
	case m.pos != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("move")}, {"tab", T("before / after")}, {"esc", T("close")}}
	case m.prompt != nil:
		return sAccent.Render(markPrompt+" ") + sSecond.Render(m.prompt.label) + sText.Render(m.prompt.text+"_")
	case m.editing:
		pairs = [][2]string{{"enter", T("done")}, {"esc", T("clear")}, {"tab", T("next area with matches")}, {"↑↓", T("move")}}
	case (m.tab == tabList || m.tab == tabLibrary) && m.curBar().typing:
		pairs = [][2]string{{"enter", T("done")}, {"esc", T("clear")}, {"tab", T("complete")}, {"backspace", T("drops the last filter")}, {"↑↓", T("move")}}
	case (m.tab == tabList || m.tab == tabLibrary) && m.curBar().open:
		pairs = [][2]string{{"↑↓", T("group")}, {"←→", T("value")}, {"enter", T("only these")}, {"x", T("leave out")}, {T("type"), T("find a value")}, {"esc", T("close")}}
	case m.panel != nil && m.panel.choices != nil:
		pairs = [][2]string{{"↑↓", T("choose")}, {"enter", T("confirm")}, {"esc", T("back")}}
		if m.panel.onDelete != nil {
			pairs = append(pairs, [2]string{"d", T("remove the game")})
		}
	case m.tags != nil:
		pairs = [][2]string{{"←→", T("choose")}, {"enter", T("apply")}, {"esc", T("close")}, {"←", T("type a new tag")}}
	case m.panel != nil:
		pairs = [][2]string{{"esc", T("back")}, {"↑↓", T("scroll")}}
	case m.insp != nil:
		pairs = [][2]string{{"esc", T("back")}, {"backspace", T("previous page")}, {"tab", T("section")}, {"↑↓", T("scroll")}}
	case m.full:
		pairs = [][2]string{{"↑↓", T("scroll")}, {"←→", T("previous / next mod")}, {"tab", T("sections")}, {"g", T("tags")}, {"ctrl+w", T("Workshop page")}, {"esc", T("back")}, {"?", T("all keys")}}
	case m.tab == tabProblems:
		// each item shows its own keys; F, A and l are the page's, in the context row unless
		// the conclusion takes its room (needsLayout)
		pairs = [][2]string{{"↑↓", T("choose")}, {"esc", T("back")}, {"?", T("all keys")}}
		if _, acts := m.needsLayout(); !acts {
			pairs = append(m.needsActions(), pairs...)
		}
	case m.tab == tabSubscribe:
		pairs = m.addPairs()
	case m.tab == tabSearch:
		pairs = m.fsPairs()
	case m.tab == tabRules:
		pairs = m.rulesPairs()
	case m.tab == tabHome:
		pairs = m.homePairs()
	case m.tab == tabChanges:
		pairs = m.changesPairs()
	case m.tab == tabLibrary && (m.libr.full || m.libr.detail):
		pairs = [][2]string{{"↑↓", T("scroll")}, {"enter", T("full screen")}, {"esc", T("back")}, {"F10", T("menu")}, {"?", T("all keys")}}
	case m.tab == tabLibrary:
		pairs = m.libPairs()
	case m.selCount() > 0:
		// 16 §3.4: what the selection can do, and that another list drops it.
		prefix = Tf("selected %d", m.selCount())
		pairs = append(m.verbPairs(), [2]string{"esc", T("clear the selection")}, [2]string{"←→", T("another area clears it")})
	case m.histActive():
		pairs = [][2]string{{"↑↓", T("choose")}}
		if l := m.histLabel(); l != "" {
			pairs = append(pairs, [2]string{"enter", l})
		}
		pairs = append(pairs, [2]string{"?", T("all keys")})
	case m.detailFocus:
		pairs = [][2]string{{"↑↓", T("scroll")}, {"←", T("back to the list")}, {"enter", T("full screen")}, {"g", T("tags")}, {"ctrl+o ctrl+w", T("folder / Workshop page")}, {"C", T("this mod's changes")}, {"?", T("all keys")}}
	default:
		pairs = append(m.verbPairs(), [2]string{"/", T("search")})
		if m.layout().tabs {
			pairs = append([][2]string{{"[ ]", T("areas")}}, pairs...) // the only way to the other lists
		} else if m.layout().detailW == 0 {
			pairs = append(pairs, [2]string{"enter", T("details")})
		}
		pairs = append(pairs, [2]string{"?", T("all keys")})
	}
	if len(pairs) > 7 {
		pairs = append(pairs[:6], pairs[len(pairs)-1])
	}
	// too wide: drop from the middle, so the first actions and "? all keys" stay
	cells := func() int {
		n := width.StringWidth(prefix)
		for i, p := range pairs {
			if i > 0 || prefix != "" {
				n += 3
			}
			n += width.StringWidth(p[0] + " " + p[1])
		}
		return n
	}
	last := len(pairs) - 1
	keepLast := last > 0 && pairs[last][0] == "?"
	for len(pairs) > 1 && cells() > m.w {
		if keepLast {
			pairs = append(pairs[:len(pairs)-2], pairs[len(pairs)-1])
		} else {
			pairs = pairs[:len(pairs)-1]
		}
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
			sep = "   "
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

// verbPairs: the verbs the focused list's object can use now (16 §3.5).
func (m *model) verbPairs() [][2]string {
	if m.col().cur() == nil {
		return nil
	}
	// ctrl+o / ctrl+w: the cursor's mod's folder and Workshop page (16 §3.5)
	one := [2]string{"ctrl+o ctrl+w", T("folder / Workshop page")}
	switch m.focus {
	case cInactive:
		return [][2]string{{"space", T("enable")}, {"m", T("enable at…")}, {"t", T("stage")}, one, {"g", T("tags")}}
	case cActive:
		return [][2]string{{"space", T("disable")}, {"J K", T("move")}, {"m", T("move to…")}, {"t", T("stage")}, one, {"g", T("tags")}}
	}
	ps := [][2]string{{"space", T("put back")}, {"m", T("enable at…")}, one, {"g", T("tags")}}
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
		for _, line := range p.lines {
			out = append(out, "  "+sText.Render(fit(line, inner)))
		}
		if len(p.lines) > 0 {
			out = append(out, "")
		}
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
	lines := m.panelText()
	for _, l := range window(lines, p.off, rows) {
		out = append(out, "  "+sText.Render(width.Truncate(l, inner, "…")))
	}
	if len(lines) > rows {
		out = append(out, "", "  "+sDimS.Render(Tf("%d–%d of %d", p.off+1, min(p.off+rows, len(lines)), len(lines))))
	}
	return out
}

// panelText is a text panel's lines as drawn: a prose panel (wrap) wraps them to the width;
// command output keeps its lines, and its columns, and is cut at the edge (16 §9.4).
func (m *model) panelText() []string {
	p := m.panel
	out := make([]string, 0, len(p.lines))
	for _, l := range p.lines {
		l = m.humanize(l)
		if !p.wrap {
			out = append(out, l)
			continue
		}
		ls := wrap(l, max(m.w-4, 10))
		if len(ls) == 0 {
			ls = []string{""}
		}
		out = append(out, ls...)
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

// descMax is how many description lines the detail column shows (16 §4.1): all of them
// once it is focused or scrolled.
func (m *model) descMax() int {
	if m.detailFocus || m.detailOff > 0 {
		return 0
	}
	return 12
}

// detailLines draws the detail column (w wide, n lines). Its first line is "4 details", or the
// mod's name once scrolled; in the accent colour when area 4 has the focus (16 §3.2).
func (m *model) detailLines(w, n int) []string {
	m.syncDetailOff()
	lines := m.detailLayout(detailOpts{width: w, descMax: m.descMax(), techText: m.detail})
	m.lastLines = lines
	// The scroll is kept as asked until the mod's details have loaded: PgDn pressed while
	// the description is still on its way would otherwise be lost.
	off := max(0, min(m.detailOff, len(lines)-(n-1)))
	if it := m.current(); it == nil || (m.dd != nil && m.dd.ID == it.id) {
		m.detailOff = off
	}
	title := T("details")
	if off > 0 {
		if it := m.current(); it != nil {
			title = "▲ " + it.name
		}
	}
	out := []string{areaHead("4", title, "", m.detailFocus, w)}
	for i := off; i < len(lines) && len(out) < n; i++ {
		out = append(out, lines[i].text)
	}
	return out
}

var _ = fmt.Sprint

// windowNextToUs reports whether RimArcho.exe sits next to this program: a player who
// double-clicked archo.exe often meant the window (16 §9.2).
var windowNextToUs = sync.OnceValue(func() bool {
	exe, err := os.Executable()
	if err != nil || strings.EqualFold(filepath.Base(exe), "RimArcho.exe") {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(exe), "RimArcho.exe"))
	return err == nil
})
