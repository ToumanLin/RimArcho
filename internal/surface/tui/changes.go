package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Changes (06 §4.1): after a scan the status line says what changed in one sentence, and c
// lists the changes of the last scans in words, newest first; enter shows the files an
// update changed. Questions that need the player's answer and an outside change of the
// list are items of 需要处理 (06 §7), not rows here.

type chRow struct {
	text  string
	style int // 1 header, 2 dim
	ev    *store.Event
	child bool
}

type chState struct {
	rows        []chRow
	cursor, off int
	expanded    map[int64][]string // event id → file lines
}

func (m *model) loadChanges() {
	c := &m.chg
	if c.expanded == nil {
		c.expanded = map[int64][]string{}
	}
	c.rows = nil
	if m.st == nil {
		return
	}
	scans, err := view.Changes(m.app, m.st)
	if err != nil {
		return
	}
	if len(scans) == 0 {
		c.rows = append(c.rows, chRow{text: T("no scan yet (R scans)"), style: 2})
		return
	}
	for _, s := range scans {
		c.rows = append(c.rows, chRow{text: Tf("scan %s", s.Time) + "  " + markDot + "  " + s.Summary, style: 1})
		for _, e := range s.Events {
			c.rows = append(c.rows, chRow{ev: e.Event, text: "  " + e.Mod + "  " + markDot + "  " + e.Text})
			for _, f := range c.expanded[e.ID] {
				c.rows = append(c.rows, chRow{text: "      " + f, style: 2, child: true})
			}
		}
	}
	if c.cursor >= len(c.rows) {
		c.cursor = max(len(c.rows)-1, 0)
	}
	if c.cursor < len(c.rows) && c.rows[c.cursor].style != 0 {
		c.moveCursor(1)
	}
}

// selectable rows are the events.
func (r chRow) selectable() bool { return r.ev != nil }

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

// fileDiff lists the semantic files an event changed (enter).
func (m *model) fileDiff(e *store.Event) []string { return view.FileDiff(m.app, e) }

func (m *model) changesKey(s string) tea.Cmd {
	c := &m.chg
	page := max(m.h-4, 1)
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
		if c.cursor < len(c.rows) && c.rows[c.cursor].ev != nil {
			ev := c.rows[c.cursor].ev
			if _, ok := c.expanded[ev.ID]; ok {
				delete(c.expanded, ev.ID)
			} else {
				c.expanded[ev.ID] = m.fileDiff(ev)
			}
			m.loadChanges()
		}
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
		case r.style == 1:
			l = sDimS.Render(l)
		case r.style == 2:
			l = sDimS.Render(l)
		}
		out = append(out, l)
	}
	return out
}
