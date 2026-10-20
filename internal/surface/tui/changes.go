package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Changes (06 §4.1): after a scan the status line says what changed in one sentence, and c
// lists the changes of the last scans in words, newest first; enter shows the files an
// update changed. Questions that need the player's answer and an outside change of the
// list are items of 需要处理 (06 §7), not rows here.

const changeScans = 10 // scans shown

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
	ix := m.st.Ix
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
	name := func(id string) string {
		if x := ix.ByID[id]; x != nil {
			return x.DisplayName()
		}
		return T("a removed mod")
	}
	for _, s := range scans {
		es := by[s.ID]
		counts := map[string]int{}
		for _, e := range es {
			counts[e.Kind]++
		}
		sum := changeSummary(counts)
		if sum == "" {
			sum = T("nothing changed")
		}
		c.rows = append(c.rows, chRow{text: Tf("scan %s", m.app.FormatTime(s.Started)) + "  " + markDot + "  " + sum, style: 1})
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Kind != es[j].Kind {
				return kindOrder(es[i].Kind) < kindOrder(es[j].Kind)
			}
			return strings.ToLower(name(es[i].InstanceID)) < strings.ToLower(name(es[j].InstanceID))
		})
		for _, e := range es {
			c.rows = append(c.rows, chRow{ev: e, text: "  " + name(e.InstanceID) + "  " + markDot + "  " + eventText(e.Kind)})
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

// kindOrder puts what a player most wants to know first.
func kindOrder(k string) int {
	for i, x := range []string{identity.EvUpdated, identity.EvPackageIDChanged, identity.EvRenamed, identity.EvDiscovered,
		identity.EvDisappeared, identity.EvReappeared, identity.EvDependencies, identity.EvLoadRules, identity.EvVersionSupport,
		identity.EvAuthorChanged, identity.EvMoved, identity.EvCopied} {
		if k == x {
			return i
		}
	}
	return 99
}

// changeSummary: "12 mods updated · 1 changed its package id" (06 §4.1).
func changeSummary(counts map[string]int) string {
	var kinds []string
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kindOrder(kinds[i]) < kindOrder(kinds[j]) })
	var parts []string
	for _, k := range kinds {
		n := counts[k]
		switch k {
		case identity.EvUpdated:
			parts = append(parts, Tf("%d mods updated", n))
		case identity.EvDiscovered:
			parts = append(parts, Tf("%d new mods", n))
		case identity.EvDisappeared:
			parts = append(parts, Tf("%d mods gone", n))
		case identity.EvPackageIDChanged:
			parts = append(parts, Tf("%d changed their package id", n))
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, eventText(k)))
		}
	}
	return strings.Join(parts, " "+markDot+" ")
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
