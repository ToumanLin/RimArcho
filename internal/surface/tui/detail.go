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

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Details (06 §6): who the mod is, its problems, the facts a player uses (package id,
// version, Workshop page, tags, alias, note, dependencies), the description, and the
// technical information folded behind i.

// detailData is what the detail shows for one mod, loaded off the key path.
type detailData struct {
	id       string
	decl     scan.Decl
	desc     *app.Description
	about    []bbcode.Block
	ws       []bbcode.Block
	events   []*store.Event // the mod's change events, oldest first
	fetching bool
}

type detailDataMsg struct{ d *detailData }
type fetchTick struct{ id string }

// fetchDwell: the Workshop page is fetched once the cursor rests this long (06 §6.4 item 3).
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
	return func() tea.Msg {
		d := &detailData{id: id, decl: mod.Decl()}
		d.desc = a.Describe(ix, mod, fetch, refresh)
		d.about, d.ws = bbcode.Parse(d.desc.About), bbcode.Parse(d.desc.Workshop)
		d.events, _ = a.Store.Events(store.EventFilter{EnvID: ix.Env.ID, InstanceIDs: []string{id}})
		return detailDataMsg{d}
	}
}

// onData stores loaded data and schedules the Workshop fetch when it is missing.
func (m *model) onData(d *detailData) tea.Cmd {
	if it := m.current(); it == nil || it.id != d.id {
		return nil
	}
	m.dd = d
	if d.desc.WorkshopState == "not cached" {
		id := d.id
		return tea.Tick(fetchDwell, func(time.Time) tea.Msg { return fetchTick{id} })
	}
	return nil
}

func (m *model) onFetchTick(id string) tea.Cmd {
	if it := m.current(); it == nil || it.id != id || m.dd == nil || m.dd.id != id {
		return nil
	}
	m.dd.fetching = true
	return m.loadData(id, true, false)
}

// ---- layout ----

// dline is one line of the detail; an image slot is rows lines that start with img set.
type dline struct {
	text    string
	imgURL  string // first line of an image slot
	imgRows int
	imgCols int
	imgMax  int    // the rows the image may take (fewer in the column than in the full view)
	imgInd  int    // cells from the start of the line to the image
	thumb   string // the mod id, for a thumbnail slot (drawn only after the dwell)
	sec     int    // 1–5 on a sub-section's header in the full-screen detail
}

type detailOpts struct {
	width    int
	descMax  int  // description lines at most (0: all)
	full     bool // the full-screen detail
	images   bool // draw images (WezTerm); else placeholders
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
	facts := []string{sourceText(mod)}
	if mod.Kind != source.Official {
		if v := strings.Fields(mod.SupportedVersions); len(v) > 0 {
			facts = append(facts, T("supports")+" "+strings.Join(v, " "))
		}
	}
	switch {
	case it.pos > 0:
		facts = append(facts, Tf("number %d", it.pos))
	case m.focus == cStaging:
		facts = append(facts, T("staged"))
	default:
		facts = append(facts, T("not enabled"))
	}
	out = append(out, m.headerLines(it, mod, strings.Join(facts, " "+markDot+" "), w, o)...)
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
		why := T("the game cannot load it")
		if mod.Unloadable != "" {
			why = T("About.xml is renamed: the game cannot load it")
		} else if mod.Residue {
			why = T("no longer subscribed: the game ignores this folder")
		}
		marked(statusIcon(s), why, sSecond)
	default:
		marked(statusIcon(s), T("needs attention · ! shows what"), sText)
	}
	add("")
	// facts
	var d *detailData
	if m.dd != nil && m.dd.id == it.id {
		d = m.dd
	}
	type kv struct{ k, v string }
	var kvs []kv
	kvs = append(kvs, kv{T("package"), mod.PackageID})
	if d != nil && d.decl.ModVersion != "" {
		kvs = append(kvs, kv{T("version"), d.decl.ModVersion})
	}
	if ws := mod.WorkshopID(); ws != "" && isNumber(ws) {
		kvs = append(kvs, kv{T("workshop"), ws + sDimS.Render(" "+markDot+" o "+T("opens the page"))})
	}
	if len(mod.Tags) > 0 {
		kvs = append(kvs, kv{T("tags"), strings.Join(mod.Tags, " "+markDot+" ")})
	} else {
		kvs = append(kvs, kv{T("tags"), sDimS.Render(T("none · # adds"))})
	}
	if mod.Alias != "" {
		kvs = append(kvs, kv{T("alias"), mod.Alias + sDimS.Render("  ("+mod.Name+")")})
	}
	if mod.Note != "" {
		kvs = append(kvs, kv{T("note"), mod.Note})
	}
	lw := 0
	for _, x := range kvs {
		lw = max(lw, width.StringWidth(x.k))
	}
	deps := m.depLines(d)
	if len(deps) > 0 {
		lw = max(lw, width.StringWidth(T("requires")))
	}
	lw += 2
	for _, x := range kvs {
		label := sDimS.Render(fit(x.k, lw))
		lines := wrap(stripANSIWidth(x.v), w-lw)
		if len(lines) <= 1 {
			add(label + clip(x.v, w-lw))
			continue
		}
		for i, l := range lines {
			if i == 0 {
				add(label + sText.Render(fit(l, w-lw)))
			} else {
				add(strings.Repeat(" ", lw) + sText.Render(fit(l, w-lw)))
			}
		}
	}
	for i, l := range deps {
		label := strings.Repeat(" ", lw)
		if i == 0 {
			label = sDimS.Render(fit(T("requires"), lw))
		}
		if lipgloss.Width(l) <= w-lw {
			add(label + clip(l, w-lw))
			continue
		}
		for j, x := range wrap(stripANSIWidth(l), w-lw) {
			if j > 0 {
				label = strings.Repeat(" ", lw)
			}
			add(label + sText.Render(fit(x, w-lw)))
		}
	}
	add("")
	// description
	out = append(out, m.descLines(d, o, w)...)
	add("")
	if o.full {
		out = append(out, m.secLines(w)...)
	} else if d != nil && len(d.events) > 0 {
		add(sDimS.Render(fit(T("history"), w)))
		for _, l := range wrap(historyLine(d.events), w-2) {
			add("  " + sSecond.Render(fit(l, w-2)))
		}
		add("")
	}
	if !m.detailTech {
		addWrapped(T("technical information · i"), sDimS, 0)
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

// Thumbnail block sizes in cells (06 §6.2, §6.3): the column and the full-screen detail.
const (
	thumbCols, thumbRows         = 18, 9
	thumbFullCols, thumbFullRows = 36, 18
	thumbMinText                 = 16 // text cells beside the block, else no block
)

// headerLines: name, authors and facts, beside the thumbnail block in the WezTerm host.
// The block is always reserved at full size; the image takes the fewest rows that hold
// it, and a mod without Preview.png shows a faint "no preview" in it (06 §6.1, §6.2).
func (m *model) headerLines(it *item, mod *store.Mod, facts string, w int, o detailOpts) []dline {
	tc, tr := thumbCols, thumbRows
	if o.full {
		tc, tr = thumbFullCols, thumbFullRows
	}
	block := o.images && w-tc-2 >= thumbMinText
	tw := w
	if block {
		tw = w - tc - 2
	}
	var text []string
	put := func(s string, st lipgloss.Style) {
		for _, l := range wrap(s, tw) {
			text = append(text, st.Render(fit(l, tw)))
		}
	}
	put(it.name, sBold)
	if a := strings.TrimSpace(mod.Authors); a != "" {
		put(a, sSecond)
	}
	put(it.icon+" "+facts, sDimS)
	if !block {
		out := make([]dline, len(text))
		for i, l := range text {
			out[i] = dline{text: l}
		}
		return out
	}
	url := previewURL(mod.ID)
	e := m.imgs[imgKey(url, tc, tr)]
	empty := strings.Repeat(" ", tc)
	n := max(tr, len(text))
	out := make([]dline, n)
	for i := range out {
		left := empty
		if i == tr/2 && e != nil && e.state == imgFailed {
			left = sDimS.Render(fit(strings.Repeat(" ", (tc-width.StringWidth(T("no preview")))/2)+T("no preview"), tc))
		}
		right := strings.Repeat(" ", tw)
		if i < len(text) {
			right = text[i]
		}
		out[i] = dline{text: left + "  " + right}
	}
	out[0].imgURL, out[0].imgCols, out[0].imgMax, out[0].thumb = url, tc, tr, mod.ID
	return out
}

func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
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

// depLines: the declared dependencies with their state (06 §6.1).
func (m *model) depLines(d *detailData) []string {
	if d == nil {
		return nil
	}
	mm := ""
	if m.st != nil {
		mm = "v" + m.st.GameMM()
	}
	seen := map[string]bool{}
	var pkgs []scan.Dependency
	for _, x := range append(append([]scan.Dependency{}, d.decl.Dependencies...), d.decl.DependenciesByVersion[mm]...) {
		p := strings.ToLower(x.PackageID)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		pkgs = append(pkgs, x)
	}
	var out []string
	for _, x := range pkgs {
		p := strings.ToLower(x.PackageID)
		var inst []*store.Mod
		for _, mod := range m.st.Ix.Mods {
			if mod.PackageID == p && mod.Status == identity.Live {
				inst = append(inst, mod)
			}
		}
		name := x.DisplayName
		if name == "" {
			name = p
		}
		enabled := false
		for _, mod := range inst {
			if m.st.List.Index(mod.ID) >= 0 {
				enabled, name = true, mod.DisplayName()
			}
		}
		switch {
		case enabled:
			out = append(out, sOK.Render(markOK)+" "+sText.Render(name))
		case len(inst) > 0:
			out = append(out, sWarnS.Render(ic.warn)+" "+sText.Render(inst[0].DisplayName())+sDimS.Render(T(" (not enabled)")))
		default:
			out = append(out, sErrS.Render(ic.err)+" "+sText.Render(name)+sDimS.Render(T(" (not installed)")))
		}
	}
	return out
}

// descLines: the description heading and text (06 §6.4 item 2).
func (m *model) descLines(d *detailData, o detailOpts, w int) []dline {
	var out []dline
	head := func(title, note string) {
		if note != "" {
			title += "  " + markDot + "  " + note
		}
		for _, l := range wrap(title, w) {
			out = append(out, dline{text: sDimS.Render(fit(l, w))})
		}
	}
	if d == nil {
		head(T("description"), T("loading…"))
		return out
	}
	note := ""
	switch st := d.desc.WorkshopState; {
	case d.fetching && st == "not cached":
		note = T("fetching the Workshop page…")
	case st == "not cached":
		note = T("the Workshop page is fetched in a moment")
	case st == "offline":
		note = T("offline (workshop.fetch)")
	case st != "" && st != "fetched" && st != "cached":
		note = T("Workshop page: ") + st
	}
	show := d.desc.Show
	var parts [][]dline
	if show == "about" || show == "both" {
		title := T("description · About.xml")
		if len(d.about) == 0 {
			head(title, note)
			out = append(out, dline{text: "  " + sDimS.Render(fit(T("(empty)"), w-2))})
		} else {
			head(title, note)
			parts = append(parts, m.renderBlocks(d.about, w, o))
			out = append(out, parts[len(parts)-1]...)
		}
		note = ""
	}
	if show == "workshop" || show == "both" {
		if show == "both" {
			out = append(out, dline{})
		}
		head(T("description · Workshop page"), note)
		out = append(out, m.renderBlocks(d.ws, w, o)...)
	}
	if o.descMax > 0 && len(out) > o.descMax {
		out = append(out[:o.descMax-1], dline{text: sDimS.Render(fit(T("… enter shows all"), w))})
		// an image slot cut at the end is dropped
		for i := len(out) - 2; i >= 0; i-- {
			if out[i].imgURL != "" && i+out[i].imgRows > len(out)-1 {
				out = append(out[:i], out[len(out)-1])
				break
			}
		}
	}
	return out
}

// renderBlocks lays out BBCode blocks at width w (06 §6.4 item 5).
func (m *model) renderBlocks(bs []bbcode.Block, w int, o detailOpts) []dline {
	var out []dline
	add := func(s string) { out = append(out, dline{text: s}) }
	// The compact detail draws no description images: one faint line counts them, and the
	// full-screen detail shows them (06 §12.4).
	nimg, imgAt := 0, -1
	fill := func() {
		if nimg == 0 {
			return
		}
		t := Tf("[images ×%d · enter shows them full screen]", nimg)
		if !o.images {
			t = Tf("[images ×%d]", nimg)
		}
		out[imgAt].text = "  " + sDimS.Render(fit(t, max(w-2, 4)))
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
			if !o.images {
				add(strings.Repeat(" ", ind) + sDimS.Render(fit("["+T("image")+" "+markDot+" "+host+"]", bw)))
				continue
			}
			cols := min(bw, 60)
			// In the column an image takes at most half the description, so text stays visible.
			maxR := 20
			if o.descMax > 0 {
				maxR = max(o.descMax/2, 3)
			}
			e := m.imgs[imgKey(b.URL, cols, maxR)]
			switch {
			case e == nil || e.state == imgLoading:
				// one line until the size is known
				out = append(out, dline{text: strings.Repeat(" ", ind) + sDimS.Render(fit(T("image loading… ")+host, bw)), imgURL: b.URL, imgRows: 1, imgCols: cols, imgMax: maxR, imgInd: ind})
			case e.state == imgFailed:
				add(strings.Repeat(" ", ind) + sDimS.Render(fit("["+T("image")+" "+markDot+" "+host+" "+markDot+" "+e.err+"]", bw)))
			default:
				out = append(out, dline{text: "", imgURL: b.URL, imgRows: e.rows, imgCols: cols, imgMax: maxR, imgInd: ind})
				for r := 1; r < e.rows; r++ {
					add("")
				}
			}
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
	for _, sp := range spans {
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
	}
	// Kinsoku: closing punctuation is glued to the token before it and opening punctuation
	// to the token after it, so no line starts with "。" or ends with "「"; a break there
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
	closingPunct = "。，、；：！？）」』】〉》〕］｝”’．・…ー,.;:!?)]}%"
	openingPunct = "（「『【〈《〔［｛“‘([{"
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

// imageCmds starts loading the images of the detail that are not known yet (one at a
// time, so the newest selection is not stuck behind a queue).
func (m *model) imageCmds(lines []dline) tea.Cmd {
	if !m.inlineImages() {
		return nil
	}
	var cmds []tea.Cmd
	if m.imgLoading == "" {
		for _, l := range lines {
			if l.imgURL == "" || (l.thumb != "" && m.thumbShown != l.thumb) {
				continue // with a dwell set, a thumbnail waits until the cursor rests
			}
			key := imgKey(l.imgURL, l.imgCols, l.imgMax)
			if _, ok := m.imgs[key]; ok {
				continue
			}
			m.imgLoading = key
			cmds = append(cmds, m.startImage(key, l.imgURL, l.imgCols, l.imgMax))
			break
		}
	}
	if m.prefetching == "" {
		if c := m.prefetchCmd(lines); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) startImage(key, url string, cols, maxRows int) tea.Cmd {
	m.imgs[key] = &imgEntry{state: imgLoading}
	var ix *app.Index
	if m.st != nil {
		ix = m.st.Ix
	}
	f := loadImage(m.app, ix, url, cols, maxRows)
	return func() tea.Msg { return f() }
}

// prefetchRange: thumbnails are made ahead for this many rows above and below the cursor.
const prefetchRange = 10

// prefetchCmd loads the nearest thumbnail around the cursor that is not cached yet, one
// at a time beside the visible images, so stopping on a row usually finds it ready.
func (m *model) prefetchCmd(lines []dline) tea.Cmd {
	tc, tr := 0, 0
	for _, l := range lines {
		if l.thumb != "" {
			tc, tr = l.imgCols, l.imgMax
			break
		}
	}
	if tc == 0 || m.full {
		return nil
	}
	c := m.col()
	for d := 1; d <= prefetchRange; d++ {
		for _, i := range []int{c.cursor + d, c.cursor - d} {
			if i < 0 || i >= len(c.view) {
				continue
			}
			id := c.items[c.view[i]].id
			if id == "" {
				continue
			}
			key := imgKey(previewURL(id), tc, tr)
			if _, ok := m.imgs[key]; ok {
				continue
			}
			m.prefetching = key
			return m.startImage(key, previewURL(id), tc, tr)
		}
	}
	return nil
}

// inlineImages: images are drawn in the WezTerm host (06 §6.4 item 6), unless ui.previews
// is off (06 §12.4).
func (m *model) inlineImages() bool { return m.images != nil && !m.previewsOff }

// openURL opens a page in the default browser.
func openURL(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}

// ---- full-screen detail (06 §6.3) ----

func (m *model) fullKey(s string) tea.Cmd {
	page := max(m.h-6, 1)
	switch s {
	case "esc", "q", "backspace":
		m.full = false
		return nil
	case "down", "j":
		m.fullOff++
	case "up", "k":
		m.fullOff--
	case "pgdown", " ":
		m.fullOff += page
	case "pgup":
		m.fullOff -= page
	case "home", "g":
		m.fullOff = 0
	case "end", "G":
		m.fullOff = 1 << 30
	case "1", "2", "3", "4", "5":
		for i, l := range m.lastLines {
			if l.sec == int(s[0]-'0') {
				m.fullOff = i
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
	case "o":
		return m.openWorkshop()
	case "#":
		return m.tagKey()
	case "tab":
		if it := m.current(); it != nil && it.id != "" {
			m.insp, m.inspHist = nil, nil
			return m.openInspect([]string{"mod", it.ref})
		}
	}
	return nil
}

func (m *model) openWorkshop() tea.Cmd {
	it := m.current()
	if it == nil || m.st == nil {
		return nil
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil || !isNumber(mod.WorkshopID()) {
		m.say(sayInfo, T("not a workshop mod"))
		return nil
	}
	u := "https://steamcommunity.com/sharedfiles/filedetails/?id=" + mod.WorkshopID()
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	if err := open(u); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the Workshop page"))
	}
	return nil
}

// fullLines draws the full-screen detail, scrolled.
func (m *model) fullLines(n int) ([]string, []placement) {
	w := max(m.w-4, 20)
	lines := m.detailLayout(detailOpts{width: w, full: true, images: m.inlineImages(), techText: m.detail})
	m.lastLines = lines
	maxOff := max(len(lines)-n, 0)
	m.fullOff = max(0, min(m.fullOff, maxOff))
	var out []string
	var ps []placement
	for i := m.fullOff; i < len(lines) && len(out) < n; i++ {
		l := lines[i]
		out = append(out, "  "+l.text)
		if p, ok := m.place(l, len(out)-1, n, 3); ok { // the line starts at column 3
			ps = append(ps, p)
		}
	}
	if maxOff > 0 && len(out) == n {
		pct := 100 * m.fullOff / maxOff
		out[n-1] = "  " + sDimS.Render(fit(Tf("%d%% · ↑↓ PgUp PgDn scroll", pct), w))
	}
	return out, ps
}

// place returns the placement of an image slot starting at body line y when it fits
// entirely in the n body lines; x is the 1-based screen column where the line starts.
func (m *model) place(l dline, y, n, x int) (placement, bool) {
	if l.imgURL == "" || (l.thumb != "" && m.thumbShown != l.thumb) {
		return placement{}, false
	}
	key := imgKey(l.imgURL, l.imgCols, l.imgMax)
	e := m.imgs[key]
	if e == nil || e.state != imgOK || y+e.rows > n {
		return placement{}, false
	}
	m.touchImage(key)
	// body line y is screen row 3+y (1-based): top bar, blank line, then the body
	p := placement{row: 3 + y, col: x + l.imgInd, cols: l.imgCols, rows: e.rows, seq: e.seq}
	if l.thumb != "" {
		p.clr = l.imgMax // the whole reserved block is erased (06 §6.2)
	}
	return p, true
}
