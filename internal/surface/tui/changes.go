package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Changes view (06 §5): open identity questions pinned on top (s same · c copy · u
// unrelated), an external ModsConfig.xml change (L load into the workspace · I ignore),
// then the change events of the last scans, newest first, grouped by event type; enter
// shows the semantic files an event changed.

const changeScans = 10 // scans shown

type chRow struct {
	text  string
	style int // 1 header, 2 dim
	q     *store.Question
	ev    *store.Event
	conf  int // ModsConfig.xml conflict version
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
	ix := m.st.Ix
	label := func(id string) string {
		if x := ix.ByID[id]; x != nil {
			return ix.Ref(id) + " " + x.DisplayName()
		}
		return ix.Ref(id)
	}
	if qs, err := m.app.Store.Questions(ix.Env.ID, false); err == nil && len(qs) > 0 {
		c.rows = append(c.rows, chRow{text: Tf("identity questions %d — s same · c copy · u unrelated", len(qs)), style: 1})
		for _, q := range qs {
			c.rows = append(c.rows, chRow{q: q, text: fmt.Sprintf("q:%d  %s  ↔  %s  (%s %.2f)", q.ID, label(q.InstanceID), label(q.CandidateID), T("score"), q.Score)})
		}
	}
	if n := m.st.WS.Conflict; n != 0 {
		c.rows = append(c.rows, chRow{text: T("ModsConfig.xml"), style: 1},
			chRow{conf: n, text: Tf("changed outside RimArcho, kept as v%d — L load into the workspace · I ignore", n)})
	}
	scans, err := m.app.Store.Scans(ix.Env.ID, changeScans)
	if err != nil || len(scans) == 0 {
		c.rows = append(c.rows, chRow{text: T("no scan yet (R scans)"), style: 2})
		return
	}
	evs, err := m.app.Store.Events(store.EventFilter{EnvID: ix.Env.ID, MinScan: scans[len(scans)-1].ID})
	if err != nil {
		return
	}
	by := map[int64][]*store.Event{}
	for _, e := range evs {
		by[e.ScanID] = append(by[e.ScanID], e)
	}
	for _, s := range scans {
		es := by[s.ID]
		counts := map[string]int{}
		for _, e := range es {
			counts[e.Kind]++
		}
		var parts []string
		for k, n := range counts {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
		sort.Strings(parts)
		sum := strings.Join(parts, ", ")
		if sum == "" {
			sum = T("no changes")
		}
		c.rows = append(c.rows, chRow{text: fmt.Sprintf("scan:%d  %s  %s", s.ID, m.app.FormatTime(s.Started), sum), style: 1})
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Kind != es[j].Kind {
				return es[i].Kind < es[j].Kind
			}
			return strings.ToLower(label(es[i].InstanceID)) < strings.ToLower(label(es[j].InstanceID))
		})
		for _, e := range es {
			text := fmt.Sprintf("  %-14s %s", e.Kind, label(e.InstanceID))
			if e.Detail != "" {
				text += "  " + e.Detail
			}
			c.rows = append(c.rows, chRow{ev: e, text: text})
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

// selectable rows are questions, the conflict and events.
func (r chRow) selectable() bool { return r.q != nil || r.ev != nil || r.conf != 0 }

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

// fileDiff lists the semantic files an event changed (06 §5: enter).
func (m *model) fileDiff(e *store.Event) []string {
	if e.Before == 0 || e.After == 0 || e.Before == e.After {
		return []string{T("(no file-level difference recorded for this event)")}
	}
	a, err1 := m.app.Store.SemanticSet(e.Before)
	b, err2 := m.app.Store.SemanticSet(e.After)
	if err1 != nil || err2 != nil {
		return []string{T("(snapshot no longer stored)")}
	}
	var out []string
	for p, h := range b {
		if old, ok := a[p]; !ok {
			out = append(out, "+ "+p)
		} else if old != h {
			out = append(out, "~ "+p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			out = append(out, "- "+p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][2:] < out[j][2:] })
	if len(out) == 0 {
		return []string{T("(semantic files unchanged: only other files changed)")}
	}
	if len(out) > 20 {
		out = append(out[:20], Tf("… %d more", len(out)-20))
	}
	return out
}

func (m *model) changesKey(s string) tea.Cmd {
	c := &m.chg
	page := max(m.h-4, 1)
	var cur chRow
	if c.cursor < len(c.rows) {
		cur = c.rows[c.cursor]
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
		if cur.ev != nil {
			if _, ok := c.expanded[cur.ev.ID]; ok {
				delete(c.expanded, cur.ev.ID)
			} else {
				c.expanded[cur.ev.ID] = m.fileDiff(cur.ev)
			}
			m.loadChanges()
		}
	case "s", "c", "u":
		if cur.q != nil {
			ans := map[string]string{"s": "same", "c": "copy", "u": "unrelated"}[s]
			m.confirmChange([]string{"identity", "resolve", fmt.Sprintf("q:%d", cur.q.ID), ans})
		}
	case "L":
		if cur.conf != 0 {
			m.confirmChange([]string{"loadout", "checkout", fmt.Sprintf("v%d", cur.conf)})
		}
	case "I":
		if cur.conf != 0 {
			if err := m.app.DismissConflict(); err != nil {
				m.status = T("error: ") + err.Error()
			} else {
				m.status = T("external ModsConfig.xml version ignored (it stays in loadout log)")
				m.reload()
				m.loadChanges()
			}
		}
	}
	return nil
}

// confirmChange previews a command and runs it on y; the view reloads afterwards.
func (m *model) confirmChange(argv []string) {
	m.openPanel("archo "+strings.Join(argv, " "), T("run this command?"), argv)
	m.afterPanel = func(m *model) { m.loadChanges() }
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
			l = stSel.Render(l)
		case r.style == 1:
			l = stHeader.Render(l)
		case r.style == 2:
			l = stDim.Render(l)
		}
		out = append(out, l)
	}
	return out
}

var _ = cmds.ExitOK
