package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Changes (06 §2.9): one row per mod with what its changes mean, by tier — the current
// plan's mods since that plan was last played (a: the whole library); new and gone mods
// are one summary row; older changes follow by day. enter expands a row (the grouped
// summary, then every fact), i shows its raw file list. ctrl+w opens the Workshop change
// notes, ctrl+o the folder, S sorts, ! goes to 能不能跑 for dependencies, p locks this
// version (twice: it copies files). tab / shift+tab go to Mod 库存 › 全部.

const (
	chHead  = iota + 1 // a heading
	chDim              // detail text
	chMod              // a mod's row
	chGroup            // new and gone (expands)
	chDay              // an older day (expands)
)

type chRow struct {
	text string
	kind int
	mod  *modchanges.Row
	key  string // what enter expands
}

type chState struct {
	rows        []chRow
	cursor, off int
	page        *modchanges.Page
	open        map[string]bool     // expanded rows, days and groups
	raw         map[string][]string // raw file lists shown (i)
	library     bool                // the whole library (a)
	of, ofName  string              // only this mod's Lineage (C); "" every mod
}

func rowKey(r *modchanges.Row) string { return fmt.Sprintf("%s|%d|%d", r.ID, r.Before, r.After) }

func (m *model) loadChanges() {
	c := &m.chg
	if c.open == nil {
		c.open, c.raw = map[string]bool{}, map[string][]string{}
	}
	c.rows, c.page = nil, nil
	if m.st == nil {
		return
	}
	o := modchanges.Opts{Library: c.library}
	if c.of != "" {
		o.Of, o.OfName = view.LineageIDs(m.st.Ix, c.of), c.ofName
	}
	p, err := view.BuildChanges(m.app, m.st, o)
	if err != nil {
		c.rows = append(c.rows, chRow{text: err.Error(), kind: chDim})
		return
	}
	c.page = p
	c.relayout()
}

// build lays the page out as rows.
func (c *chState) build() {
	p := c.page
	add := func(text string, kind int) { c.rows = append(c.rows, chRow{text: text, kind: kind}) }
	switch {
	case c.of != "":
		add(Tf("only %s", c.ofName)+"  "+markDot+"  "+T("esc shows every mod"), chHead)
	case c.library:
		add(T("the whole library")+"  "+markDot+"  "+p.Since+"  "+markDot+"  "+T("a: only the current plan"), chHead)
	default:
		add(Tf("plan %s", p.Plan)+"  "+markDot+"  "+p.Since+"  "+markDot+"  "+T("a: the whole library"), chHead)
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
		c.rows = append(c.rows, chRow{text: "  " + mark + d.Day + "  " + markDot + "  " + Tf("%d mods", n), kind: chDay, key: k})
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
	c.rows = append(c.rows, chRow{text: ind + r.Mod + "  " + markDot + "  " + r.Text, kind: chMod, mod: r, key: k})
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

// selectable rows are the mods, the groups and the days.
func (r chRow) selectable() bool { return r.kind == chMod || r.kind == chGroup || r.kind == chDay }

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

// relayout rebuilds the rows and keeps the cursor on the same row.
func (c *chState) relayout() {
	key := ""
	if c.cursor < len(c.rows) {
		key = c.rows[c.cursor].key
	}
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
	if r != nil {
		mod = r.mod
	}
	switch s {
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
			m.loadChanges()
			c.moveCursor(1)
		}
	case "tab", "shift+tab":
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
