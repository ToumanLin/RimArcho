package tui

import (
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Details (16 §4): who the mod is, its problems, the facts a player uses (package id,
// version, Workshop page, tags, alias, note, dependencies), the description, and the
// technical information folded behind i.

// detailData is what the detail shows for one mod beyond its row (view.DetailData).
type detailData = view.DetailData

type detailDataMsg struct{ d *detailData }
type fetchTick struct{ id string }

// fetchDwell: the Workshop page is fetched once the cursor rests this long (16 §4.3 item 3).
const fetchDwell = 400 * time.Millisecond

// loadData reads the declaration, About.xml and the cached Workshop text (no network).
func (m *model) loadData(id string, fetch, refresh bool) tea.Cmd {
	if m.st == nil {
		return nil
	}
	mod := m.st.Ix.ByID[id]
	if mod == nil {
		return nil
	}
	ix, a := m.st.Ix, m.app
	return func() tea.Msg { return detailDataMsg{view.LoadDetail(a, ix, mod, fetch, refresh)} }
}

// onData stores loaded data and schedules the Workshop fetch when it is missing.
func (m *model) onData(d *detailData) tea.Cmd {
	if it := m.current(); it == nil || it.id != d.ID {
		return nil
	}
	m.dd = d
	if d.Desc.WorkshopState == "not cached" {
		id := d.ID
		return tea.Tick(fetchDwell, func(time.Time) tea.Msg { return fetchTick{id} })
	}
	return nil
}

func (m *model) onFetchTick(id string) tea.Cmd {
	if it := m.current(); it == nil || it.id != id || m.dd == nil || m.dd.ID != id {
		return nil
	}
	m.dd.Fetching = true
	return m.loadData(id, true, false)
}

// ---- layout ----

// dline is one text line of the detail.
type dline struct {
	text string
	sec  int // 1–5 on a sub-section header in the full-screen detail
}

type detailOpts struct {
	width    int
	descMax  int  // description lines at most (0: all)
	full     bool // the full-screen detail
	techText []string
}

// detailLayout lays out the detail of the current item.
func (m *model) detailLayout(o detailOpts) []dline {
	it := m.current()
	if it == nil || m.st == nil {
		return nil
	}
	w := max(o.width, 10)
	var out []dline
	add := func(s string) { out = append(out, dline{text: s}) }
	addWrapped := func(s string, st lipgloss.Style, indent int) {
		for _, l := range wrap(s, w-indent) {
			add(strings.Repeat(" ", indent) + st.Render(fit(l, w-indent)))
		}
	}
	if it.missing {
		addWrapped(it.name, sBold, 0)
		addWrapped(T("listed in ModsConfig.xml, but not installed"), sSecond, 0)
		return out
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil {
		return nil
	}
	facts := view.HeaderFacts(mod, it.pos, m.focus == cStaging)
	out = append(out, m.headerLines(it, mod, strings.Join(facts, "  "), w, o)...)
	add("")
	marked := func(mark, text string, st lipgloss.Style) {
		for i, l := range wrap(text, w-2) {
			if i == 0 {
				add(mark + " " + st.Render(fit(l, w-2)))
			} else {
				add("  " + st.Render(fit(l, w-2)))
			}
		}
	}
	switch s := m.rowStatus(it); s {
	case statusNone:
		marked(sOK.Render(markOK), T("no problems found"), sSecond)
	case statusUnloadable:
		why := view.UnloadableWhy(m.st, mod)
		marked(statusIcon(s), why, sSecond)
	default:
		marked(statusIcon(s), T("needs attention, shift+2 shows what"), sText)
	}
	add("")
	var d *detailData
	if m.dd != nil && m.dd.ID == it.id {
		d = m.dd
	}
	if d != nil && d.Version != nil {
		r := d.Version
		addWrapped(r.Text, sSecond, 0)
		for _, a := range r.Actions {
			addWrapped(a.Key+"  "+a.Label, sSecond, 0)
		}
		add("")
	}
	// the field list (16 §4.1), the GUI's one
	for _, l := range fieldLines(view.ModFields(m.app, m.st, mod, d), w) {
		add(l)
	}
	add("")
	// description
	out = append(out, m.descLines(d, o, w)...)
	add("")
	if o.full {
		out = append(out, m.secLines(w)...)
	} else if d != nil && len(d.Events) > 0 {
		add(sDimS.Render(fit(T("history"), w)))
		for _, l := range wrap(view.HistoryLine(d.Events), w-2) {
			add("  " + sSecond.Render(fit(l, w-2)))
		}
		// the whole history of its Lineage is on the Updates page (16 §2.9)
		for _, l := range wrap(T("see all in change history")+", C", w-2) {
			add("  " + sDimS.Render(l))
		}
		add("")
	}
	if !m.detailTech {
		addWrapped(T("technical information, i"), sDimS, 0)
		return out
	}
	addWrapped(T("technical information"), sDimS, 0)
	for _, l := range o.techText {
		for _, x := range wrap(l, w-2) {
			add("  " + sSecond.Render(fit(x, w-2)))
		}
	}
	return out
}

// headerLines lays out the name, authors and facts at the left edge.
func (m *model) headerLines(it *item, mod *store.Mod, facts string, w int, o detailOpts) []dline {
	var text []string
	put := func(s string, st lipgloss.Style) {
		for _, l := range wrap(s, w) {
			text = append(text, st.Render(fit(l, w)))
		}
	}
	if m.detailFocus && !o.full {
		put(it.name, sAccent.Bold(true))
	} else {
		put(it.name, sBold)
	}
	if a := strings.TrimSpace(mod.Authors); a != "" {
		put(a, sSecond)
	}
	put(it.icon+" "+facts, sDimS)
	out := make([]dline, len(text))
	for i, l := range text {
		out[i] = dline{text: l}
	}
	return out
}

// stripANSIWidth: the text of a possibly styled value, for wrapping.
func stripANSIWidth(s string) string { return ansi.Strip(s) }

// clip cuts a styled string to w cells and pads it.
func clip(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = width.Truncate(stripANSIWidth(s), w, "…")
		return sText.Render(s) + strings.Repeat(" ", max(w-width.StringWidth(s), 0))
	}
	return sText.Render("") + s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// depLines: the declared dependencies with their state (16 §4.1).
// descLines: the description heading and text (16 §4.3 item 2).
func (m *model) descLines(d *detailData, o detailOpts, w int) []dline {
	var out []dline
	head := func(title, note string) {
		if note != "" {
			title += "  " + note
		}
		for _, l := range wrap(title, w) {
			out = append(out, dline{text: sDimS.Render(fit(l, w))})
		}
	}
	if d == nil {
		head(T("description"), T("loading…"))
		return out
	}
	note := view.DescNote(d)
	show := d.Desc.Show
	var parts [][]dline
	if show == "about" || show == "both" {
		title := T("description / About.xml")
		if len(d.About) == 0 {
			head(title, note)
			out = append(out, dline{text: "  " + sDimS.Render(fit(T("(empty)"), w-2))})
		} else {
			head(title, note)
			parts = append(parts, m.renderBlocks(d.About, w, o))
			out = append(out, parts[len(parts)-1]...)
		}
		note = ""
	}
	if show == "workshop" || show == "both" {
		if show == "both" {
			out = append(out, dline{})
		}
		head(T("description / Workshop page"), note)
		out = append(out, m.renderBlocks(d.Workshop, w, o)...)
	}
	if o.descMax > 0 && len(out) > o.descMax {
		out = append(out[:o.descMax-1], dline{text: sDimS.Render(fit(T("… enter shows all"), w))})
	}
	return out
}

// renderBlocks lays out BBCode blocks at width w (16 §4.3 item 5).
func (m *model) renderBlocks(bs []bbcode.Block, w int, o detailOpts) []dline {
	var out []dline
	add := func(s string) { out = append(out, dline{text: s}) }
	// The compact detail counts description images in one faint line; the
	// full-screen detail lists each placeholder.
	nimg, imgAt := 0, -1
	fill := func() {
		if nimg == 0 {
			return
		}
		out[imgAt].text = "  " + sDimS.Render(fit(Tf("[images ×%d]", nimg), max(w-2, 4)))
	}
	for i, b := range bs {
		ind := 2
		if b.Quote {
			ind += 2
		}
		bw := max(w-ind, 4)
		switch b.Kind {
		case bbcode.Para:
			if len(b.Spans) == 0 {
				add("")
				continue
			}
			base := sText
			if b.Quote {
				base = sSecond
			}
			for _, l := range wrapSpans(b.Spans, bw, base) {
				add(strings.Repeat(" ", ind) + l)
			}
		case bbcode.Heading:
			if i > 0 && len(out) > 0 && out[len(out)-1].text != "" {
				add("")
			}
			for _, l := range wrapSpans(b.Spans, bw, sBold) {
				add(strings.Repeat(" ", ind) + l)
			}
		case bbcode.Item:
			lead := ind + (b.Level-1)*2
			mw := width.StringWidth(b.Marker) + 1
			for j, l := range wrapSpans(b.Spans, max(w-lead-mw, 4), sText) {
				if j == 0 {
					add(strings.Repeat(" ", lead) + sDimS.Render(b.Marker+" ") + l)
				} else {
					add(strings.Repeat(" ", lead+mw) + l)
				}
			}
		case bbcode.Table:
			for _, l := range tableLines(b.Rows, bw) {
				add(strings.Repeat(" ", ind) + l)
			}
		case bbcode.Image:
			if !o.full {
				if nimg == 0 {
					imgAt = len(out)
					add("")
				}
				nimg++
				continue
			}
			host := ""
			if u, err := url.Parse(b.URL); err == nil {
				host = u.Host
			}
			add(strings.Repeat(" ", ind) + sDimS.Render(fit("["+T("image")+"  "+host+"]", bw)))
		}
	}
	fill()
	return out
}

// wrapSpans wraps styled spans to w cells: at spaces, and between CJK characters.
func wrapSpans(spans []bbcode.Span, w int, base lipgloss.Style) []string {
	type tok struct {
		s     string // the text
		out   string // the text styled (several styles once glued)
		st    lipgloss.Style
		space bool
	}
	var toks []tok
	for i, sp := range spans {
		st := base
		switch {
		case sp.Strike:
			st = sDimS
		case sp.Link != "":
			st = sSecond
		case sp.Bold:
			st = sBold
		}
		var word []rune
		flush := func() {
			if len(word) > 0 {
				toks = append(toks, tok{s: string(word), out: st.Render(string(word)), st: st})
				word = word[:0]
			}
		}
		for _, r := range sp.Text {
			switch {
			case r == ' ' || r == '\t':
				flush()
				toks = append(toks, tok{s: " ", out: st.Render(" "), st: st, space: true})
			case width.RuneWidth(r) == 2 || unicode.Is(unicode.Han, r):
				flush()
				toks = append(toks, tok{s: string(r), out: st.Render(string(r)), st: st})
			default:
				word = append(word, r)
			}
		}
		flush()
		// A link's domain follows its text, faint (16 §9.3), unless the text already shows it.
		if sp.Link != "" && (i+1 == len(spans) || spans[i+1].Link != sp.Link) {
			if u, err := url.Parse(sp.Link); err == nil && u.Host != "" && !strings.Contains(sp.Text, u.Host) {
				domain := "(" + u.Host + ")"
				toks = append(toks, tok{s: " ", out: " ", space: true},
					tok{s: domain, out: sDimS.Render(domain), st: sDimS})
			}
		}
	}
	// Kinsoku: closing punctuation is glued to the token before it and opening punctuation
	// to the token after it, so no line starts with a CJK closing mark or ends with an opening mark; a break there
	// takes the previous character down with it.
	glued := toks[:0:0]
	for _, t := range toks {
		if n := len(glued); n > 0 && !t.space && !glued[n-1].space && (allIn(t.s, closingPunct) || allIn(glued[n-1].s, openingPunct)) {
			glued[n-1].s += t.s
			glued[n-1].out += t.out
			continue
		}
		glued = append(glued, t)
	}
	toks = glued
	var lines []string
	var cur strings.Builder
	cw := 0
	emit := func() {
		lines = append(lines, cur.String()+strings.Repeat(" ", max(w-cw, 0)))
		cur.Reset()
		cw = 0
	}
	for _, t := range toks {
		tw := width.StringWidth(t.s)
		if t.space {
			if cw == 0 || cw+1 > w {
				continue
			}
		}
		if cw+tw > w && cw > 0 {
			emit()
			if t.space {
				continue
			}
		}
		for tw > w { // a word longer than the line (its pieces take the first piece's style)
			part := width.Truncate(t.s, w, "")
			if i := strings.LastIndexAny(part, `/\`); i > 0 && i < len(part)-1 {
				part = part[:i+1] // a path breaks after a separator
			}
			cur.WriteString(t.st.Render(part))
			cw = width.StringWidth(part)
			emit()
			t.s = strings.TrimPrefix(t.s, part)
			t.out = t.st.Render(t.s)
			tw = width.StringWidth(t.s)
		}
		cur.WriteString(t.out)
		cw += tw
	}
	if cw > 0 || len(lines) == 0 {
		emit()
	}
	return lines
}

// Punctuation that must not start a line, and that must not end one (CJK and ASCII).
const (
	closingPunct = "\u3002\uff0c\u3001\uff1b\uff1a\uff01\uff1f\uff09\u300d\u300f\u3011\u3009\u300b\u3015\uff3d\uff5d”’\uff0e・…ー,.;:!?)]}%"
	openingPunct = "\uff08\u300c\u300e\u3010\u3008\u300a\u3014\uff3b\uff5b“‘([{"
)

// allIn reports whether s is non-empty and made only of runes in set.
func allIn(s, set string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(set, r) {
			return false
		}
	}
	return true
}

// tableLines aligns a table's columns; the first row is bold; wide columns are cut.
func tableLines(rows [][]string, w int) []string {
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	if n == 0 {
		return nil
	}
	ws := make([]int, n)
	for _, r := range rows {
		for i, c := range r {
			ws[i] = max(ws[i], width.StringWidth(c))
		}
	}
	gap := 2
	total := func() int {
		t := gap * (n - 1)
		for _, x := range ws {
			t += x
		}
		return t
	}
	for total() > w { // shrink the widest column
		wi := 0
		for i := range ws {
			if ws[i] > ws[wi] {
				wi = i
			}
		}
		if ws[wi] <= 3 {
			break
		}
		ws[wi]--
	}
	var out []string
	for ri, r := range rows {
		var b strings.Builder
		used := 0
		for i := 0; i < n; i++ {
			c := ""
			if i < len(r) {
				c = r[i]
			}
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap))
				used += gap
			}
			cell := fit(c, ws[i])
			if ri == 0 {
				b.WriteString(sBold.Render(cell))
			} else {
				b.WriteString(sText.Render(cell))
			}
			used += ws[i]
		}
		if used > w {
			out = append(out, width.Truncate(stripANSIWidth(b.String()), w, "…"))
			continue
		}
		out = append(out, b.String()+strings.Repeat(" ", w-used))
	}
	return out
}

// openURL opens a page in the default browser.
// A variable so tests can never start the real program (noopen_test.go).
var openURL = func(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}

// ---- full-screen detail (16 §4.2) ----

func (m *model) fullKey(s string) tea.Cmd {
	if cmd, ok := m.versionDetailKey(s); ok {
		return cmd
	}
	page := max(m.h-6, 1)
	switch s {
	case "esc", "q", "backspace", "enter":
		m.full = false // the cursor is on the last mod shown (enter opens and closes, 16 §3.5)
		return m.requestDetail()
	case "down", "j":
		m.fullOff++
		m.syncFullSec()
	case "up", "k":
		m.fullOff--
		m.syncFullSec()
	case "pgdown", " ":
		m.fullOff += page
		m.syncFullSec()
	case "pgup":
		m.fullOff -= page
		m.syncFullSec()
	case "home":
		m.fullOff, m.fullSec = 0, 0
	case "end":
		m.fullOff = 1 << 30
		m.syncFullSec()
	case "left", "h", "right", "l":
		// the previous / next mod of the list, from the top
		d := 1
		if s == "left" || s == "h" {
			d = -1
		}
		c := m.col()
		i := c.cursor + d
		if i < 0 || i >= len(c.view) || c.items[c.view[i]].id == "" {
			return nil
		}
		c.cursor = i
		c.scroll(m.rowsShown() - 1)
		m.fullOff, m.fullSec = 0, 0
		m.detailRef = ""
		return tea.Batch(m.requestDetail(), m.loadFullSecs())
	case "tab", "shift+tab":
		starts := m.fullStarts()
		if s == "tab" {
			m.fullSec = min(m.fullSec+1, len(starts)-1)
		} else {
			m.fullSec = max(m.fullSec-1, 0)
		}
		m.fullOff = starts[m.fullSec]
		m.say(sayInfo, m.fullSecName(m.fullSec))
	case "1", "2", "3", "4", "5":
		for i, l := range m.lastLines {
			if l.sec == int(s[0]-'0') {
				m.fullOff = i
				m.fullSec = l.sec
			}
		}
	case "i":
		m.detailTech = !m.detailTech
		m.detailRef = ""
		return m.requestDetail()
	case "r":
		if it := m.current(); it != nil && it.id != "" {
			m.say(sayBusy, T("fetching the Workshop page…"))
			return m.loadData(it.id, true, true)
		}
	case "ctrl+o":
		if it := m.current(); it != nil {
			return m.openFolderOf(it.id)
		}
	case "ctrl+w":
		if it := m.current(); it != nil {
			return m.openWorkshopID(m.workshopOf(it.id))
		}
	case "ctrl+s":
		if it := m.current(); it != nil {
			return m.openWorkshopSteamID(m.workshopOf(it.id))
		}
	case "g":
		return m.tagKey()
	case "n":
		return m.noteKey()
	case "I":
		if it := m.current(); it != nil && it.id != "" {
			m.insp, m.inspHist = nil, nil
			return m.openInspect([]string{"mod", it.ref})
		}
	}
	return nil
}

// fullStarts: where the summary and each sub-section start in the full-screen detail.
func (m *model) fullStarts() []int {
	starts := []int{0}
	for i, l := range m.lastLines {
		if l.sec > 0 {
			starts = append(starts, i)
		}
	}
	return starts
}

func (m *model) fullSecName(k int) string {
	if k == 0 {
		return T("summary")
	}
	starts := m.fullStarts()
	if k < len(starts) {
		return strings.TrimSpace(ansi.Strip(m.lastLines[starts[k]].text))
	}
	return ""
}

// syncFullSec: after scrolling, the sub-section at the top of the screen.
func (m *model) syncFullSec() {
	m.fullSec = 0
	for k, i := range m.fullStarts() {
		if i <= m.fullOff {
			m.fullSec = k
		}
	}
}

// detailKey: the keys of area 4, the compact detail (16 §3.3): ↑↓ scroll it, ← and esc
// go back to the list whose mod it shows. Verbs and the other keys work as in that list
// (they act on the mod shown, or on the list's selection) and keep the focus here.
func (m *model) detailKey(s string) (tea.Cmd, bool) {
	if cmd, ok := m.versionDetailKey(s); ok {
		return cmd, true
	}
	m.syncDetailOff()
	switch s {
	case "down", "j":
		m.detailOff++
	case "up", "k":
		m.detailOff = max(m.detailOff-1, 0)
	case "home":
		m.detailOff = 0
	case "end":
		m.detailOff = 1 << 30
	case "pgdown":
		m.detailOff += m.detailPage() // PgUp PgDn scroll the detail only while it has the focus
	case "pgup":
		m.detailOff = max(m.detailOff-m.detailPage(), 0)
	case "left", "h", "esc":
		m.detailFocus = false
	case "right", "l", "shift+up", "shift+down":
	default:
		return nil, false
	}
	return nil, true
}

func (m *model) versionDetailKey(s string) (tea.Cmd, bool) {
	it := m.current()
	if it == nil || m.dd == nil || m.dd.ID != it.id || m.dd.Version == nil {
		return nil, false
	}
	for _, a := range m.dd.Version.Actions {
		if a.Key == s {
			m.doAction(a, 0)
			return m.requestDetail(), true
		}
	}
	return nil, false
}

// fullLines draws the full-screen detail, scrolled.
func (m *model) fullLines(n int) []string {
	w := max(m.w-4, 20)
	lines := m.detailLayout(detailOpts{width: w, full: true, techText: m.detail})
	m.lastLines = lines
	maxOff := max(len(lines)-n, 0)
	m.fullOff = max(0, min(m.fullOff, maxOff))
	var out []string
	for i := m.fullOff; i < len(lines) && len(out) < n; i++ {
		l := lines[i]
		out = append(out, "  "+l.text)
	}
	if maxOff > 0 && len(out) == n {
		pct := 100 * m.fullOff / maxOff
		out[n-1] = "  " + sDimS.Render(fit(Tf("%d%%, ↑↓ PgUp PgDn scroll", pct), w))
	}
	return out
}
