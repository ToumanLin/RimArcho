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

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/bbcode"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/rimarcho/rimarcho/internal/store"
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
	imgMax  int // the rows the image may take (fewer in the column than in the full view)
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
	addWrapped(it.name, sBold, 0)
	if a := strings.TrimSpace(mod.Authors); a != "" {
		addWrapped(a, sSecond, 0)
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
	addWrapped(it.icon+" "+strings.Join(facts, " "+markDot+" "), sDimS, 0)
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
		kvs = append(kvs, kv{T("workshop"), ws + sDimS.Render("  o "+T("opens the page"))})
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
		add(label + clip(l, w-lw))
	}
	add("")
	// description
	out = append(out, m.descLines(d, o, w)...)
	add("")
	if !m.detailTech && !o.full {
		add(sDimS.Render(fit(T("technical information · i"), w)))
		return out
	}
	if !m.detailTech {
		add(sDimS.Render(fit(T("technical information · i"), w)))
		return out
	}
	add(sDimS.Render(fit(T("technical information"), w)))
	for _, l := range o.techText {
		for _, x := range wrap(l, w-2) {
			add("  " + sSecond.Render(fit(x, w-2)))
		}
	}
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
		t := sDimS.Render(title)
		if note != "" {
			t += sDimS.Render("  " + markDot + "  " + note)
		}
		out = append(out, dline{text: clip(t, w)})
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
				out = append(out, dline{text: strings.Repeat(" ", ind) + sDimS.Render(fit(T("image loading… ")+host, bw)), imgURL: b.URL, imgRows: 1, imgCols: cols, imgMax: maxR})
			case e.state == imgFailed:
				add(strings.Repeat(" ", ind) + sDimS.Render(fit("["+T("image")+" "+markDot+" "+host+" "+markDot+" "+e.err+"]", bw)))
			default:
				out = append(out, dline{text: "", imgURL: b.URL, imgRows: e.rows, imgCols: cols, imgMax: maxR})
				for r := 1; r < e.rows; r++ {
					add("")
				}
			}
		}
	}
	return out
}

// wrapSpans wraps styled spans to w cells: at spaces, and between CJK characters.
func wrapSpans(spans []bbcode.Span, w int, base lipgloss.Style) []string {
	type tok struct {
		s     string
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
				toks = append(toks, tok{s: string(word), st: st})
				word = word[:0]
			}
		}
		for _, r := range sp.Text {
			switch {
			case r == ' ' || r == '\t':
				flush()
				toks = append(toks, tok{s: " ", st: st, space: true})
			case width.RuneWidth(r) == 2 || unicode.Is(unicode.Han, r):
				flush()
				toks = append(toks, tok{s: string(r), st: st})
			default:
				word = append(word, r)
			}
		}
		flush()
	}
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
		for tw > w { // a word longer than the line
			part := width.Truncate(t.s, w, "")
			cur.WriteString(t.st.Render(part))
			cw = width.StringWidth(part)
			emit()
			t.s = strings.TrimPrefix(t.s, part)
			tw = width.StringWidth(t.s)
		}
		cur.WriteString(t.st.Render(t.s))
		cw += tw
	}
	if cw > 0 || len(lines) == 0 {
		emit()
	}
	return lines
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
	if m.imgLoading != "" || !m.inlineImages() {
		return nil
	}
	for _, l := range lines {
		if l.imgURL == "" {
			continue
		}
		key := imgKey(l.imgURL, l.imgCols, l.imgMax)
		if _, ok := m.imgs[key]; ok {
			continue
		}
		m.imgs[key] = &imgEntry{state: imgLoading}
		m.imgLoading = key
		f := loadImage(m.app, l.imgURL, l.imgCols, l.imgMax)
		return func() tea.Msg { return f() }
	}
	return nil
}

// inlineImages: images are drawn in the WezTerm host (06 §6.4 item 6).
func (m *model) inlineImages() bool { return m.images != nil }

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
		if p, ok := m.place(l, len(out)-1, n, 3); ok {
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
// entirely in the n body lines; x is the 1-based column of the slot's text.
func (m *model) place(l dline, y, n, x int) (placement, bool) {
	if l.imgURL == "" || y+l.imgRows > n {
		return placement{}, false
	}
	e := m.imgs[imgKey(l.imgURL, l.imgCols, l.imgMax)]
	if e == nil || e.state != imgOK {
		return placement{}, false
	}
	// body line y is screen row 3+y (1-based): top bar, blank line, then the body
	return placement{row: 3 + y, col: x + 2, cols: l.imgCols, rows: l.imgRows, seq: e.seq}, true
}
