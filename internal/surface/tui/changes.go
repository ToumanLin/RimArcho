package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 订阅与更新 (06 §2.9, §9.1): three simple lists. 1 the sources (enter checks one, K every
// one with mods); 2 the updates waiting for the player (u updates one, U every one shown —
// the dry run first, then a background task, refused while a game runs; Del removes the
// subscription; p locks this version first; ctrl+w opens the source page); 3 the change
// history: one row per mod with what its changes mean, by tier — the current plan's mods
// since that plan was last played (a: the whole library); new and gone mods are one summary
// row; older changes follow by day. enter expands a row (the grouped summary, then every
// fact), i shows its raw file list. ctrl+w opens the Workshop change notes, ctrl+o the
// folder, S sorts, D goes to 能不能跑 for dependencies, p locks this version (twice: it
// copies files), Del removes a subscription RimArcho keeps. + (or a paste) goes to 添加订阅;
// tab / shift+tab go to the other pages of Mod 库存.

const (
	chHead    = iota + 1 // a heading
	chDim                // detail text
	chMod                // a mod's row
	chGroup              // new and gone (expands)
	chDay                // an older day (expands)
	chSource             // a subscription source (enter checks it)
	chPending            // an update waiting for the player
)

type chRow struct {
	text string
	kind int
	mod  *modchanges.Row
	src  *view.SubscriptionSource
	pend *view.PendingUpdate
	key  string // what enter expands; the row's identity across reloads
}

type chState struct {
	rows          []chRow
	cursor, off   int
	page          *modchanges.Page
	subscriptions *view.SubscriptionsPage
	open          map[string]bool     // expanded rows, days and groups
	raw           map[string][]string // raw file lists shown (i)
	library       bool                // the whole library (a)
	of, ofName    string              // only this mod's Lineage (C); "" every mod
}

func rowKey(r *modchanges.Row) string { return fmt.Sprintf("%s|%d|%d", r.ID, r.Before, r.After) }

func (m *model) loadChanges() {
	c := &m.chg
	if c.open == nil {
		c.open, c.raw = map[string]bool{}, map[string][]string{}
	}
	key := c.cursorKey() // the cursor stays on its row when the page is read again
	c.rows, c.page = nil, nil
	if m.st == nil {
		return
	}
	o := modchanges.Opts{Library: c.library}
	if c.of != "" {
		o.Of, o.OfName = view.LineageIDs(m.st.Ix, c.of), c.ofName
	}
	p, err := view.BuildSubscriptions(m.app, m.st, o)
	if err != nil {
		c.rows = append(c.rows, chRow{text: err.Error(), kind: chDim})
		return
	}
	c.page, c.subscriptions = p.History, p
	c.relayoutAt(key)
}

// build lays the page out as rows.
func (c *chState) build() {
	p := c.page
	add := func(text string, kind int) { c.rows = append(c.rows, chRow{text: text, kind: kind}) }
	if c.subscriptions != nil {
		add("1 "+T("sources"), chHead)
		for i := range c.subscriptions.Sources {
			s := &c.subscriptions.Sources[i]
			c.rows = append(c.rows, chRow{text: fmt.Sprintf("  %s  %d  %s  %s  %s", s.Name, s.Count, s.Checked, s.Result, s.Account), kind: chSource, src: s, key: "src:" + s.Key})
		}
		add("2 "+T("pending updates"), chHead)
		for i := range c.subscriptions.Pending {
			r := &c.subscriptions.Pending[i]
			c.rows = append(c.rows, chRow{text: "  " + r.Name + "  " + r.Source + "  " + r.Installed + " → " + r.Available + "  " + strings.Join(r.UsedIn, ", "), kind: chPending, pend: r, key: "pend:" + r.ID})
		}
		if len(c.subscriptions.Pending) == 0 {
			add("  "+T("no known updates; check a source above"), chDim)
		}
	}
	add("3 "+T("change history"), chHead)
	switch {
	case c.of != "":
		add(Tf("only %s", c.ofName)+"  "+T("esc shows every mod"), chHead)
	case c.library:
		add(T("the whole library")+"  "+p.Since+"  "+T("a: only the current list"), chHead)
	default:
		add(Tf("list %s", p.Plan)+"  "+p.Since+"  "+T("a: the whole library"), chHead)
	}
	if p.NoScan {
		add("  "+T("no scan yet (R scans)"), chDim)
		return
	}
	c.tiered(p.Rows, "  ")
	c.group(p.Arrivals, "now", "  ")
	if len(p.Rows) == 0 && p.Arrivals == nil && c.of == "" {
		add("  "+T("nothing changed"), chDim)
	}
	if len(p.Older) > 0 && c.of == "" {
		add(T("earlier"), chHead)
	}
	for _, d := range p.Older {
		k := "day:" + d.Day
		n := len(d.Rows)
		if d.Arrivals != nil {
			n += len(d.Arrivals.Rows)
		}
		open := c.of != "" || c.open[k]
		mark := "▸ "
		if open {
			mark = "▾ "
		}
		c.rows = append(c.rows, chRow{text: "  " + mark + d.Day + "  " + Tf("%d mods", n), kind: chDay, key: k})
		if open {
			c.tiered(d.Rows, "    ")
			c.group(d.Arrivals, d.Day, "    ")
		}
	}
}

// tiered lists rows under their tier's heading.
func (c *chState) tiered(rs []modchanges.Row, ind string) {
	tier := 0
	for i := range rs {
		r := &rs[i]
		if r.Tier != tier {
			tier = r.Tier
			c.rows = append(c.rows, chRow{text: ind + r.TierText, kind: chHead})
		}
		c.mod(r, ind+"  ")
	}
}

func (c *chState) mod(r *modchanges.Row, ind string) {
	k := rowKey(r)
	c.rows = append(c.rows, chRow{text: ind + r.Mod + "  " + r.Text, kind: chMod, mod: r, key: k})
	if c.open[k] {
		for _, d := range r.Details {
			c.rows = append(c.rows, chRow{text: ind + "    " + d, kind: chDim})
		}
		if r.HasFiles() {
			if fs, ok := c.raw[k]; ok {
				for _, f := range fs {
					c.rows = append(c.rows, chRow{text: ind + "      " + f, kind: chDim})
				}
			} else {
				c.rows = append(c.rows, chRow{text: ind + "    " + T("i shows the files"), kind: chDim})
			}
		}
	}
}

// group is the one row of new and gone mods; enter lists them.
func (c *chState) group(g *modchanges.Group, where, ind string) {
	if g == nil {
		return
	}
	k := "group:" + where
	mark := "▸ "
	if c.open[k] {
		mark = "▾ "
	}
	c.rows = append(c.rows, chRow{text: ind + mark + g.Text, kind: chGroup, key: k})
	if c.open[k] {
		for i := range g.Rows {
			c.mod(&g.Rows[i], ind+"  ")
		}
	}
}

// selectable rows are the sources, the updates, the mods, the groups and the days.
func (r chRow) selectable() bool {
	return r.kind == chMod || r.kind == chGroup || r.kind == chDay || r.kind == chSource || r.kind == chPending
}

// home puts the cursor where the page starts: the first update waiting, else the first
// change (the sources are above, reached with 1 or ↑).
func (c *chState) home() {
	for i, r := range c.rows {
		if r.selectable() && r.kind != chSource {
			c.cursor = i
			return
		}
	}
	c.cursor = 0
	if c.cursor < len(c.rows) && !c.rows[c.cursor].selectable() {
		c.moveCursor(1)
	}
}

func (c *chState) moveCursor(d int) {
	for i := c.cursor + d; i >= 0 && i < len(c.rows); i += d {
		if c.rows[i].selectable() {
			c.cursor = i
			return
		}
	}
	if c.cursor < len(c.rows) && !c.rows[c.cursor].selectable() && d > 0 {
		c.moveCursor(-1)
	}
}

// cursorKey is the identity of the cursor's row ("" a heading or nothing).
func (c *chState) cursorKey() string {
	if c.cursor < len(c.rows) {
		return c.rows[c.cursor].key
	}
	return ""
}

// relayout rebuilds the rows and keeps the cursor on the same row.
func (c *chState) relayout() { c.relayoutAt(c.cursorKey()) }

// relayoutAt rebuilds the rows with the cursor on the row of key ("" where the page starts).
func (c *chState) relayoutAt(key string) {
	c.rows = nil
	if c.page != nil {
		c.build()
	}
	for i, r := range c.rows {
		if key != "" && r.key == key {
			c.cursor = i
			return
		}
	}
	if key == "" {
		c.home()
		return
	}
	c.cursor = min(c.cursor, max(len(c.rows)-1, 0))
	if c.cursor < len(c.rows) && !c.rows[c.cursor].selectable() {
		c.moveCursor(1)
	}
}

func (m *model) changesKey(s string) tea.Cmd {
	c := &m.chg
	page := max(m.h-4, 1)
	var r *chRow
	if c.cursor < len(c.rows) && c.rows[c.cursor].selectable() {
		r = &c.rows[c.cursor]
	}
	var mod *modchanges.Row
	var pend *view.PendingUpdate
	if r != nil {
		mod, pend = r.mod, r.pend
	}
	switch s {
	case "+":
		return m.openAdd("")
	case "K":
		return m.checkSources(c.allSources())
	case "U":
		if c.subscriptions != nil {
			return m.updatePending(c.shownPending())
		}
		return nil
	case "u":
		if pend != nil {
			return m.updatePending([]view.PendingUpdate{*pend})
		}
		return nil
	case "delete":
		switch {
		case pend != nil && pend.Kind != app.LibWorkshop:
			return m.removeSubscription(pend.Ref, pend.Name)
		case mod != nil && m.subscriptionOf(mod.ID):
			return m.removeSubscription(mod.Ref, mod.Mod)
		}
		m.say(sayInfo, T("not a subscription RimArcho keeps: Steam's are unsubscribed in the mod library"))
		return nil
	}
	if r != nil && r.kind == chSource && s == "enter" {
		return m.checkSources([]string{r.src.Key})
	}
	if pend != nil {
		switch s {
		case "p":
			ref, name := pend.Ref, pend.Name
			return m.arm("p", Tf("p again locks this version of %s (a copy in the lock library)", name), func(m *model) tea.Cmd {
				m.act(Tf("locked %s", name), "pin", ref)
				m.reload()
				m.loadChanges()
				return nil
			})
		case "ctrl+w":
			return m.openSource(pend)
		}
	}
	switch s {
	case "1", "2", "3":
		for i, row := range c.rows {
			if len(row.text) > 1 && row.text[:2] == s+" " {
				c.cursor, c.off = i, i
				break
			}
		}
	case "down", "j":
		c.moveCursor(1)
	case "up", "k":
		c.moveCursor(-1)
	case "pgdown":
		for i := 0; i < page; i++ {
			c.moveCursor(1)
		}
	case "pgup":
		for i := 0; i < page; i++ {
			c.moveCursor(-1)
		}
	case "enter":
		if r != nil {
			c.open[r.key] = !c.open[r.key]
			c.relayout()
		}
	case "i":
		if mod != nil && mod.HasFiles() {
			k := r.key
			if _, ok := c.raw[k]; ok {
				delete(c.raw, k)
			} else {
				c.raw[k], c.open[k] = view.ChangeFiles(m.app, mod.Before, mod.After), true
			}
			c.relayout()
		}
	case "a":
		if c.of == "" {
			c.library = !c.library
			c.cursor, c.off = 0, 0
			c.rows = nil // the cursor goes home, not to the row it was on
			m.loadChanges()
			c.home()
		}
	case "tab":
		return m.openAdd("")
	case "shift+tab":
		return m.openLibrary()
	case "ctrl+o":
		if mod != nil {
			return m.openFolderOf(mod.ID)
		}
	case "ctrl+w":
		if mod != nil {
			return m.openChangeNotes(mod.WorkshopID)
		}
	case "S":
		if has(mod, modchanges.ActSort) {
			return m.listAction(view.ListSort)
		}
	case "D":
		if has(mod, modchanges.ActDeps) {
			m.tab = tabProblems
			return m.ensureProblems()
		}
	case "p":
		if has(mod, modchanges.ActLock) {
			ref, name := mod.Ref, mod.Mod
			return m.arm("p", Tf("p again locks this version of %s (a copy in the lock library)", name), func(m *model) tea.Cmd {
				m.act(Tf("locked %s", name), "pin", ref)
				m.reload()
				m.loadChanges()
				return nil
			})
		}
	}
	return nil
}

// has: the row offers the action.
func has(r *modchanges.Row, kind string) bool {
	if r == nil {
		return false
	}
	for _, a := range r.Actions {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// openChangeNotes opens a Workshop item's change notes.
func (m *model) openChangeNotes(ws string) tea.Cmd {
	if !view.IsNumber(ws) {
		m.say(sayInfo, T("no Workshop ID: not a Workshop mod"))
		return nil
	}
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	if err := open(view.ChangeNotesURL(ws)); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the change notes"))
	}
	return nil
}

func (m *model) changeLines(n int) []string {
	c := &m.chg
	page := max(n, 1)
	if c.cursor < c.off {
		c.off = max(c.cursor-1, 0)
	}
	if c.cursor >= c.off+page {
		c.off = c.cursor - page + 1
	}
	var out []string
	for i := c.off; i < len(c.rows) && len(out) < n; i++ {
		r := c.rows[i]
		l := fit(r.text, m.w)
		switch {
		case i == c.cursor && r.selectable():
			l = sSel.Render(l)
		case r.kind == chHead || r.kind == chDim:
			l = sDimS.Render(l)
		}
		out = append(out, l)
	}
	return out
}

// shownPending: the updates the list shows (U updates all of them).
func (c *chState) shownPending() []view.PendingUpdate {
	var out []view.PendingUpdate
	for _, r := range c.rows {
		if r.pend != nil {
			out = append(out, *r.pend)
		}
	}
	return out
}

// changesPairs: the hint line of 订阅与更新 — what the cursor's row can do, then the page's
// own keys (06 §2.9 按键).
func (m *model) changesPairs() [][2]string {
	c := &m.chg
	var r *chRow
	if c.cursor < len(c.rows) && c.rows[c.cursor].selectable() {
		r = &c.rows[c.cursor]
	}
	var pairs, later [][2]string // the row's main keys, the page's, then the row's others
	switch {
	case r == nil:
	case r.kind == chSource:
		pairs = append(pairs, [2]string{"enter", T("check this source")})
	case r.kind == chPending:
		pairs = append(pairs, [2]string{"u", T("update subscription")})
		if r.pend.Kind != app.LibWorkshop {
			pairs = append(pairs, [2]string{"Del", T("remove subscription")})
		}
		later = append(later, [2]string{"p", T("lock this version first")})
		if r.pend.Opens != "" {
			later = append(later, [2]string{"ctrl+w", T("open the source page")})
		}
	case r.kind == chMod:
		pairs = append(pairs, [2]string{"enter", T("open")})
		if r.mod.HasFiles() {
			pairs = append(pairs, [2]string{"i", T("files")})
		}
		for _, a := range r.mod.Actions {
			pairs = append(pairs, [2]string{a.Key, a.Label})
		}
	default:
		pairs = append(pairs, [2]string{"enter", T("open")})
	}
	pairs = append(append(pairs, [2]string{"K", T("check for updates")}, [2]string{"+", T("add a subscription")}), later...)
	switch {
	case c.of != "":
		pairs = append(pairs, [2]string{"esc", T("every mod")})
	case c.library:
		pairs = append(pairs, [2]string{"a", T("the current plan")}, [2]string{"esc", T("back")})
	default:
		pairs = append(pairs, [2]string{"a", T("the whole library")}, [2]string{"esc", T("back")})
	}
	return pairs
}

func init() {
	view.Register(map[string]string{
		"not a subscription RimArcho keeps: Steam's are unsubscribed in the mod library": "不是 RimArcho 管的订阅：Steam 的订阅在 Mod 库里退订",
	})
}
