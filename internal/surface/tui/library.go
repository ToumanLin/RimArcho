package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod 库存 › 全部 (06 §2.9, §9.1), laid out as the window's page: the filter bar on top (the
// editor's, view.BuildLibrary's groups), the table of Lineages that open to their copies
// (→ ←), and the detail of the cursor's copy on the right when the terminal is wide enough
// (enter shows it full screen). Every op of a copy is a key or in the row menu (view.CopyOps,
// the same table as the editor's): space adds and enables or takes out of the current plan,
// m enables at a place, t stages, x uses this copy, g tags, n the copy's note, ctrl+o
// ctrl+w, Del, F10 the menu; u undoes the plan edits.

type libState struct {
	page        *view.LibPage
	err         string
	cursor, off int
	bar         barState
	sort        string
	open        map[string]bool // lineages shown open
	flat        []libLine
	marked      map[string]bool // selected copies, by ref
	anchor      int             // where shift+↑↓ started (-1 none)
	detail      bool            // the detail column has the keys (↑↓ scroll it)
	detailOff   int
	full        bool // the detail fills the screen
	rowY        int  // the screen row of the table's first row, as last drawn
}

// libLine is one table row: a Lineage (inst -1), or one of its copies.
type libLine struct {
	row  int // index into page.Rows
	inst int // index into its Insts; -1 the Lineage's own row
	lone bool
}

func (m *model) loadLibrary() {
	l := &m.libr
	if l.open == nil {
		l.open, l.marked, l.anchor = map[string]bool{}, map[string]bool{}, -1
	}
	keep := l.lineKey(l.cursor)
	l.err = ""
	lib := m.library()
	if lib == nil {
		l.page, l.err = nil, m.status
		return
	}
	p, err := view.BuildLibrary(lib, l.bar.search, l.bar.filters, l.sort)
	if err != nil {
		l.err = err.Error()
		return
	}
	l.page = p
	b := &l.bar
	b.conds, b.ghost, b.groups = p.Conds, p.Complete, nil
	if b.open {
		for _, g := range p.Groups {
			if len(b.shownValues(g)) > 0 || b.find != "" {
				b.groups = append(b.groups, g)
			}
		}
	}
	l.flatten()
	if at := slices.IndexFunc(l.flat, func(x libLine) bool { return l.keyOf(x) == keep }); keep != "" && at >= 0 {
		l.cursor = at
	}
	l.cursor = max(0, min(l.cursor, len(l.flat)-1))
	// a copy no longer listed is no longer selected
	for ref := range l.marked {
		if l.copyByRef(ref) == nil {
			delete(l.marked, ref)
		}
	}
}

func (l *libState) flatten() {
	l.flat = l.flat[:0]
	for ri, r := range l.page.Rows {
		if r.Single {
			l.flat = append(l.flat, libLine{row: ri, inst: 0, lone: true})
			continue
		}
		l.flat = append(l.flat, libLine{row: ri, inst: -1})
		if l.open[r.ID] || len(r.Insts) < r.Copies {
			for ii := range r.Insts {
				l.flat = append(l.flat, libLine{row: ri, inst: ii})
			}
		}
	}
}

func (l *libState) keyOf(x libLine) string {
	r := l.page.Rows[x.row]
	if x.inst < 0 {
		return "L|" + r.ID
	}
	return "I|" + r.Insts[x.inst].Ref
}

func (l *libState) lineKey(i int) string {
	if l.page == nil || i < 0 || i >= len(l.flat) {
		return ""
	}
	return l.keyOf(l.flat[i])
}

func (l *libState) cur() (libLine, bool) {
	if l.page == nil || l.cursor < 0 || l.cursor >= len(l.flat) {
		return libLine{}, false
	}
	return l.flat[l.cursor], true
}

func (l *libState) copyByRef(ref string) *view.LibInstRow {
	for ri := range l.page.Rows {
		for ii := range l.page.Rows[ri].Insts {
			if l.page.Rows[ri].Insts[ii].Ref == ref {
				return &l.page.Rows[ri].Insts[ii]
			}
		}
	}
	return nil
}

// copiesOf: a copy's row is that copy; a Lineage's row is all its copies listed.
func (l *libState) copiesOf(x libLine) []*view.LibInstRow {
	r := &l.page.Rows[x.row]
	if x.inst >= 0 {
		return []*view.LibInstRow{&r.Insts[x.inst]}
	}
	var out []*view.LibInstRow
	for ii := range r.Insts {
		out = append(out, &r.Insts[ii])
	}
	return out
}

// cursorCopy is the copy the cursor stands for: its row's; on a Lineage of several copies
// the one in the current plan, else the first RimArcho has seen.
func (m *model) cursorCopy() (*view.LibInstRow, string) {
	l := &m.libr
	x, ok := l.cur()
	if !ok {
		return nil, ""
	}
	r := &l.page.Rows[x.row]
	if x.inst >= 0 {
		return &r.Insts[x.inst], r.Name
	}
	var pick *view.LibInstRow
	for ii := range r.Insts {
		i := &r.Insts[ii]
		if pick == nil || i.InPlan && !pick.InPlan || pick.ID == "" && i.ID != "" {
			pick = i
		}
	}
	return pick, r.Name
}

// libCopy is the cursor's copy (shortcuts.go's ctrl+o, ctrl+w, C).
func (m *model) libCopy() (view.LibInstRow, string, bool) {
	i, name := m.cursorCopy()
	if i == nil {
		return view.LibInstRow{}, name, false
	}
	return *i, name, true
}

// targets are the copies an op acts on: the selected ones, else the cursor's.
func (m *model) targets() []*view.LibInstRow {
	l := &m.libr
	if len(l.marked) > 0 {
		var out []*view.LibInstRow
		for ri := range l.page.Rows {
			for ii := range l.page.Rows[ri].Insts {
				if i := &l.page.Rows[ri].Insts[ii]; l.marked[i.Ref] {
					out = append(out, i)
				}
			}
		}
		return out
	}
	if i, _ := m.cursorCopy(); i != nil {
		return []*view.LibInstRow{i}
	}
	return nil
}

// targetOps: every op any target has, in menu order.
func targetOps(ts []*view.LibInstRow) []string {
	seen := map[string]bool{}
	for _, i := range ts {
		for _, c := range i.Ops {
			seen[c] = true
		}
	}
	var out []string
	for _, o := range view.OpTable() {
		if seen[o.Code] {
			out = append(out, o.Code)
		}
	}
	return out
}

func (m *model) openLibrary() tea.Cmd {
	m.tab = tabLibrary
	m.loadLibrary()
	return nil
}

// ---- running ops ----

// runLibOp runs an op on the targets that have it (06 §2.9).
func (m *model) runLibOp(code string) tea.Cmd {
	var ts []*view.LibInstRow
	for _, i := range m.targets() {
		if slices.Contains(i.Ops, code) {
			ts = append(ts, i)
		}
	}
	if len(ts) == 0 {
		m.say(sayInfo, T("nothing to do with this row"))
		return nil
	}
	_, name := m.cursorCopy()
	what := name
	if len(ts) > 1 {
		what = plural(len(ts), "mod")
	}
	i0 := ts[0]
	refs := make([]string, len(ts))
	ids := []string{}
	for k, i := range ts {
		refs[k] = i.Ref
		if i.ID != "" {
			ids = append(ids, i.ID)
		}
	}
	switch code {
	case view.OpFolder:
		return m.openFolderOf(i0.ID)
	case view.OpWorkshop, view.OpSteam:
		ws := strings.TrimPrefix(i0.Ref, "ws:")
		if i0.ID != "" {
			ws = m.workshopOf(i0.ID)
		}
		return m.openWorkshopPage(ws, code == view.OpSteam)
	case view.OpNote:
		return m.editCopyNote(name, *i0)
	case view.OpTags:
		return m.libTagKey(refs, what)
	case view.OpEnableAt, view.OpMoveTo:
		if len(ids) == 0 || m.st == nil {
			return nil
		}
		p := &posInput{ids: ids, what: what, from: cInactive}
		if code == view.OpMoveTo {
			p.from = cActive
		}
		for _, i := range ts {
			if i.ID != "" {
				p.refs = append(p.refs, m.st.Ix.Ref(i.ID))
			}
		}
		m.pos = p
		m.posOptions()
		return nil
	}
	return m.libOp(code, refs, what)
}

// libSpace is space: the copy takes effect in the current plan, or stops (06 §3.5).
func (m *model) libSpace() tea.Cmd {
	ops := targetOps(m.targets())
	for _, c := range []string{view.OpAdd, view.OpTakeOut, view.OpPutBack, view.OpUse} {
		if slices.Contains(ops, c) {
			return m.runLibOp(c)
		}
	}
	m.say(sayInfo, T("this copy cannot be used in the current list"))
	return nil
}

// libFirst runs the first of codes the targets have.
func (m *model) libFirst(codes ...string) tea.Cmd {
	ops := targetOps(m.targets())
	for _, c := range codes {
		if slices.Contains(ops, c) {
			return m.runLibOp(c)
		}
	}
	m.say(sayInfo, T("nothing to do with this row"))
	return nil
}

// libTagKey: the tag bar for copies (their mods' tags).
func (m *model) libTagKey(refs []string, what string) tea.Cmd {
	if m.st == nil {
		return nil
	}
	tb := &tagBar{mods: refs, names: what}
	var selTags [][]string
	for _, ref := range refs {
		if mod, err := m.st.Ix.ResolveOne(ref); err == nil {
			selTags = append(selTags, mod.Tags)
		}
	}
	for _, c := range view.TagCandidates(m.app, m.st.Ix.Env.ID, selTags) {
		tb.cands = append(tb.cands, tagCand{tag: c.Tag, n: c.N, have: c.Have})
	}
	if len(tb.cands) == 0 {
		tb.sel = -1
	}
	m.tags = tb
	return nil
}

// libMenu: the row menu of the cursor's copy (or the selection), beside the row.
func (m *model) libMenu() tea.Cmd {
	y := m.libr.rowY + m.libr.cursor - m.libr.off + 1
	return m.openMenu(targetOps(m.targets()), 8, y, func(m *model, code string) tea.Cmd { return m.runLibOp(code) })
}

// libRightClick: the cursor goes to the row under the pointer, then its menu opens there.
func (m *model) libRightClick(x, y int) tea.Cmd {
	l := &m.libr
	i := l.off + y - l.rowY
	if l.page == nil || i < 0 || i >= len(l.flat) {
		return nil
	}
	if !m.libRowSelected(i) {
		l.marked = map[string]bool{}
	}
	l.cursor = i
	return m.openMenu(targetOps(m.targets()), x+1, y+1, func(m *model, code string) tea.Cmd { return m.runLibOp(code) })
}

// libClick: a left click moves the cursor (ctrl adds or removes, shift a range); on the
// detail column it gives it the keys.
func (m *model) libClick(e tea.MouseMsg) tea.Cmd {
	l := &m.libr
	if tw, _ := m.libWidths(); tw < m.w && e.X >= tw+2 {
		l.detail = true
		return nil
	}
	i := l.off + e.Y - l.rowY
	if l.page == nil || i < 0 || i >= len(l.flat) {
		return nil
	}
	l.detail = false
	switch {
	case e.Ctrl:
		l.cursor = i
		m.libToggleSel()
	case e.Shift:
		if l.anchor < 0 {
			l.anchor = l.cursor
		}
		l.cursor = i
		m.libRange()
	default:
		if i == l.cursor {
			if x := l.flat[i]; x.inst < 0 {
				m.libToggleOpen(x) // a second click opens or closes a Lineage
			}
		}
		l.cursor, l.anchor = i, -1
	}
	return nil
}

func (m *model) libRowSelected(i int) bool {
	l := &m.libr
	cs := l.copiesOf(l.flat[i])
	return len(cs) > 0 && !slices.ContainsFunc(cs, func(c *view.LibInstRow) bool { return !l.marked[c.Ref] })
}

func (m *model) libToggleSel() {
	l := &m.libr
	x, ok := l.cur()
	if !ok {
		return
	}
	all := m.libRowSelected(l.cursor)
	for _, c := range l.copiesOf(x) {
		if all {
			delete(l.marked, c.Ref)
		} else {
			l.marked[c.Ref] = true
		}
	}
}

func (m *model) libRange() {
	l := &m.libr
	l.marked = map[string]bool{}
	a, b := min(l.anchor, l.cursor), max(l.anchor, l.cursor)
	for j := a; j <= b && j < len(l.flat); j++ {
		for _, c := range l.copiesOf(l.flat[j]) {
			l.marked[c.Ref] = true
		}
	}
}

func (m *model) libToggleOpen(x libLine) {
	l := &m.libr
	r := l.page.Rows[x.row]
	if r.Single || len(r.Insts) < r.Copies {
		return
	}
	keep := l.keyOf(l.flat[l.cursor])
	if l.open[r.ID] {
		delete(l.open, r.ID)
		keep = l.keyOf(libLine{row: x.row, inst: -1})
	} else {
		l.open[r.ID] = true
	}
	l.flatten()
	if at := slices.IndexFunc(l.flat, func(y libLine) bool { return l.keyOf(y) == keep }); at >= 0 {
		l.cursor = at
	}
}

// ---- keys ----

func (m *model) libraryKey(s string) tea.Cmd {
	l := &m.libr
	if l.page == nil {
		if s == "esc" {
			m.tab = tabList
		}
		return nil
	}
	n := len(l.flat)
	page := max(m.h-8-l.bar.lineCount(), 1)
	if l.full || l.detail {
		switch s {
		case "down", "j":
			l.detailOff++
			return nil
		case "up", "k":
			l.detailOff = max(l.detailOff-1, 0)
			return nil
		case "pgdown":
			l.detailOff += page
			return nil
		case "pgup":
			l.detailOff = max(l.detailOff-page, 0)
			return nil
		case "home":
			l.detailOff = 0
			return nil
		case "esc", "left", "h":
			if l.full {
				l.full = false
			}
			l.detail = false
			return nil
		case "enter":
			l.full, l.detail = !l.full, false
			return nil
		}
	}
	move := func(d int, extend bool) tea.Cmd {
		from := l.cursor
		l.cursor = max(0, min(l.cursor+d, n-1))
		if l.cursor != from {
			l.detailOff = 0
		}
		if extend {
			if l.anchor < 0 {
				l.anchor = from
			}
			m.libRange()
		} else {
			l.anchor = -1
		}
		return nil
	}
	switch s {
	case "down", "j":
		return move(1, false)
	case "up", "k":
		return move(-1, false)
	case "shift+down":
		return move(1, true)
	case "shift+up":
		return move(-1, true)
	case "pgdown":
		return move(page, false)
	case "pgup":
		return move(-page, false)
	case "home":
		return move(-n, false)
	case "end":
		return move(n, false)
	case "right", "l":
		x, ok := l.cur()
		if !ok {
			return nil
		}
		r := l.page.Rows[x.row]
		if x.inst < 0 && !l.open[r.ID] && len(r.Insts) == r.Copies {
			m.libToggleOpen(x)
			return nil
		}
		if x.inst < 0 {
			return move(1, false)
		}
		if tw, _ := m.libWidths(); tw < m.w {
			l.detail = true // → on a copy: its detail has the keys
		}
		return nil
	case "left", "h":
		x, ok := l.cur()
		if !ok {
			return nil
		}
		if x.inst >= 0 && !x.lone {
			l.cursor = slices.IndexFunc(l.flat, func(y libLine) bool { return y.row == x.row && y.inst < 0 })
			return nil
		}
		if x.inst < 0 && l.open[l.page.Rows[x.row].ID] {
			m.libToggleOpen(x)
		}
		return nil
	case "enter":
		l.full = true
		return nil
	case "esc":
		switch {
		case len(l.marked) > 0:
			l.marked, l.anchor = map[string]bool{}, -1
		case l.bar.active():
			l.bar.search, l.bar.filters = "", nil
			return m.barChanged()
		default:
			m.tab = tabList // the TUI has no sidebar: esc goes back to the editor
			return m.requestDetail()
		}
		return nil
	case "ctrl+@", "ctrl+ ", "v":
		m.libToggleSel()
		return nil
	case "ctrl+a":
		for _, x := range l.flat {
			for _, c := range l.copiesOf(x) {
				l.marked[c.Ref] = true
			}
		}
		return nil
	case "/", "ctrl+f":
		l.bar.typing = true
		return nil
	case `\`:
		return m.toggleBar()
	// the page's areas (06 §3.5): 1 the filter, 2 the table, 3 the details
	case "1":
		if !l.bar.open {
			return m.toggleBar()
		}
		return nil
	case "2":
		l.detail, l.full = false, false
		return nil
	case "3":
		if tw, _ := m.libWidths(); tw < m.w {
			l.detail = true
		} else {
			l.full = true // no room beside the table: the details over the page
		}
		return nil
	case " ":
		return m.libSpace()
	case "m":
		return m.libFirst(view.OpEnableAt, view.OpMoveTo)
	case "t":
		return m.libFirst(view.OpStage, view.OpPutBack)
	case "x":
		return m.libFirst(view.OpUse, view.OpSwap)
	case "g":
		return m.libFirst(view.OpTags)
	case "n":
		return m.libFirst(view.OpNote)
	case "ctrl+o":
		return m.libFirst(view.OpFolder)
	case "ctrl+w":
		if i, _ := m.cursorCopy(); i != nil && !slices.Contains(i.Ops, view.OpWorkshop) {
			m.say(sayInfo, T("no Workshop ID: not a Workshop mod"))
			return nil
		}
		return m.libFirst(view.OpWorkshop)
	case "ctrl+s":
		if i, _ := m.cursorCopy(); i != nil && !slices.Contains(i.Ops, view.OpSteam) {
			m.say(sayInfo, T("no Workshop ID: not a Workshop mod"))
			return nil
		}
		return m.libFirst(view.OpSteam)
	case "delete":
		return m.delKey(targetOps(m.targets()), m.runLibOp)
	case "u", "ctrl+z":
		m.act(T("undid the last change"), "undo")
		m.loadLibrary()
		return nil
	case "ctrl+r", "ctrl+y":
		m.act(T("redid the change"), "redo")
		m.loadLibrary()
		return nil
	case "s":
		return m.libSortChooser()
	case "C":
		if i, name := m.cursorCopy(); i != nil && i.ID != "" {
			return m.openChangesOf(i.ID, name)
		}
	}
	return nil
}

// libSortChooser is s: the table's sort (the window's header click); the same column again
// reverses it.
func (m *model) libSortChooser() tea.Cmd {
	l := &m.libr
	p := &panel{title: T("sort by")}
	keys := []string{}
	for _, c := range l.page.Columns {
		label := c.Title
		if strings.TrimPrefix(l.sort, "-") == c.Key || l.sort == "" && c.Key == "name" {
			label += "  " + T("again reverses")
			p.choice = len(p.choices)
		}
		p.choices = append(p.choices, label)
		keys = append(keys, c.Key)
	}
	p.onPick = func(m *model, label string) tea.Cmd {
		k := keys[slices.Index(p.choices, label)]
		cur := l.sort
		if cur == "" {
			cur = "name"
		}
		if cur == k {
			k = "-" + k
		}
		l.sort = k
		m.loadLibrary()
		return nil
	}
	m.panel = p
	return nil
}

// editCopyNote: the input line starts from the note, or from what RimArcho wrote; enter
// saves, an empty line clears it, esc leaves it.
func (m *model) editCopyNote(name string, i view.LibInstRow) tea.Cmd {
	if i.ID == "" {
		m.say(sayInfo, T("RimArcho has not seen this copy: it has no note"))
		return nil
	}
	text := i.Note
	if text == "" {
		text = i.Auto
	}
	ref := i.Ref
	m.prompt = &prompt{label: Tf("note on %s (%s): ", name, i.Label), text: text, submit: func(m *model, text string) tea.Cmd {
		msg, err := view.LibSetNote(m.app, ref, text)
		if err != nil {
			m.say(sayFail, err.Error())
			return nil
		}
		m.say(sayOK, msg)
		m.reload()
		m.loadLibrary()
		return nil
	}}
	return nil
}

// ---- drawing ----

// libWidths: the table's width and the detail column's (0: no column, enter shows it).
func (m *model) libWidths() (table, detail int) {
	if m.w < 110 {
		return m.w, 0
	}
	detail = max(44, m.w*2/5)
	return m.w - detail - 2, detail
}

func (m *model) libraryLines(n int) []string {
	l := &m.libr
	var sheet []string
	if m.pos != nil {
		sheet = m.posLines(m.w)
		n = max(n-len(sheet)-1, 4)
	}
	out := m.barView(m.w)
	switch {
	case l.err != "":
		return append(out, sErrS.Render(fit(l.err, m.w)))
	case l.page == nil:
		return out
	}
	if l.full {
		return append(out, m.libDetailLines(m.w, n-len(out))...)
	}
	tw, dw := m.libWidths()
	p := l.page
	count := Tf("%d mods", p.Total)
	if p.Shown != p.Total {
		count = Tf("%d of %d mods", p.Shown, p.Total)
	}
	if k := len(l.marked); k > 0 {
		count += "  " + Tf("selected %d", k)
	}
	top := len(out)
	table := []string{sDimS.Render(fit("2 "+count+"  "+p.Size, tw))}
	rows := max(n-top-1, 1)
	l.rowY = 2 + top + 1 // screen row of the first table row (0-based)
	if len(l.flat) == 0 {
		table = append(table, sSecond.Render(fit(T("no mod matches"), tw)))
	}
	if l.cursor < l.off {
		l.off = l.cursor
	}
	if l.cursor >= l.off+rows {
		l.off = l.cursor - rows + 1
	}
	right := min(52, tw/2)
	for i := l.off; i < len(l.flat) && len(table) < rows+1; i++ {
		table = append(table, m.libRowLine(i, tw, right))
	}
	var det []string
	if dw > 0 {
		det = m.libDetailLines(dw, n-top)
	}
	for i := 0; i < len(table) || i < len(det); i++ {
		line := strings.Repeat(" ", tw)
		if i < len(table) {
			line = table[i]
		}
		if dw > 0 && i < len(det) {
			line += "  " + det[i]
		}
		out = append(out, line)
	}
	if sheet != nil {
		out = append(append(out, ""), sheet...)
	}
	return out
}

// libRowLine: a Lineage (its copies' count, open or not) or a copy (└ its folder); then
// the default columns: size, state, source, used in (06 §2.9).
func (m *model) libRowLine(i, w, right int) string {
	l := &m.libr
	x := l.flat[i]
	r := l.page.Rows[x.row]
	cells := r.LibCells
	name := r.Name
	sel := m.libRowSelected(i)
	mark := "  "
	if i == l.cursor && !l.detail {
		mark = markSel + " "
	}
	switch {
	case x.lone:
		cells = r.Insts[0].LibCells
		name = "  " + r.Name
	case x.inst < 0:
		chev := "▸ "
		if l.open[r.ID] || len(r.Insts) < r.Copies {
			chev = "▾ "
		}
		name = chev + r.Name + "  " + sDimS.Render(Tf("%d copies", r.Copies))
	default:
		in := r.Insts[x.inst]
		cells = in.LibCells
		name = "    └ " + in.Name
		if in.InPlan {
			name += " " + ic.unsaved
		}
	}
	var info []string
	for _, c := range []string{cells.Size, cells.State, cells.Source, cells.Used} {
		if c != "" {
			info = append(info, c)
		}
	}
	line := mark + fit(name, w-right-4) + "  " + fit(strings.Join(info, "  "), right)
	st := plainStyle
	switch {
	case i == l.cursor:
		st = sSel
	case sel:
		st = sAccent
	}
	return rowBackground(st, sel, i == l.cursor && !l.detail).Render(line)
}

// libDetailLines: the cursor's copy (06 §2.9): its note (RimArcho's grey first version
// when the player wrote none), what it can do with its keys, its states, its fields.
func (m *model) libDetailLines(w, n int) []string {
	l := &m.libr
	i, name := m.cursorCopy()
	st := sDimS
	if l.detail || l.full {
		st = sAccent
	}
	if i == nil {
		return []string{st.Render(fit("3 "+T("details"), w))}
	}
	var body []string
	body = append(body, sBold.Render(fit(name, w)), sSecond.Render(fit(i.Label, w)), "")
	note, ns := i.Note, sText
	if note == "" {
		note, ns = i.Auto, sDimS
	}
	if note == "" && i.ID != "" {
		note, ns = T("n writes a note on this copy"), sDimS
	}
	for _, ln := range wrap(note, w-2) {
		body = append(body, "  "+ns.Render(ln))
	}
	body = append(body, "")
	var acts []string
	for _, o := range view.OpTable() {
		if slices.Contains(i.Ops, o.Code) && o.Key != "" {
			acts = append(acts, o.Key+" "+o.Label)
		}
	}
	acts = append(acts, "F10 "+T("menu"))
	for _, ln := range wrap(strings.Join(acts, "  "), w-2) {
		body = append(body, "  "+sSecond.Render(ln))
	}
	if len(i.States) > 0 {
		body = append(body, "")
		for _, s := range i.States {
			body = append(body, "  "+sWarnS.Render(fit(s, w-2)))
		}
	}
	if lib := m.library(); lib != nil {
		if inst := lib.Instance(i.Ref); inst != nil {
			body = append(body, "")
			for _, ln := range fieldLines(view.LibFields(m.app, lib, inst, nil), w-2) {
				body = append(body, "  "+ln)
			}
		}
	}
	off := max(0, min(l.detailOff, len(body)-(n-1)))
	l.detailOff = off
	title := "3 " + T("details")
	if off > 0 {
		title = "▲ " + name
	}
	out := []string{st.Render(fit(title, w))}
	for k := off; k < len(body) && len(out) < n; k++ {
		out = append(out, body[k])
	}
	return out
}

var _ = fmt.Sprint

// libPairs: the hint line of Mod 库存: what the cursor's copy can do now (06 §2.2).
func (m *model) libPairs() [][2]string {
	l := &m.libr
	pairs := [][2]string{}
	if x, ok := l.cur(); ok && x.inst < 0 {
		pairs = append(pairs, [2]string{"→ ←", T("open / close")})
	}
	ops := targetOps(m.targets())
	for _, o := range view.OpTable() {
		if slices.Contains(ops, o.Code) && o.Key != "" && o.Group == 1 {
			pairs = append(pairs, [2]string{o.Key, o.Label})
			if len(pairs) >= 3 {
				break
			}
		}
	}
	pairs = append(pairs, [2]string{"enter", T("details")}, [2]string{"F10", T("menu")}, [2]string{"/", T("search")}, [2]string{"?", T("all keys")})
	return pairs
}
