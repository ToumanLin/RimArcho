package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// Column search (06 §5.5): the syntax and the matching are view.Query (08 §1.1).

// matcher builds the column predicate. The second result is the fallback for a single
// free word that matched nothing: its letters in order in the name.
func (m *model) matcher(q string) (func(*item) bool, func(*item) bool) {
	pq := view.ParseQuery(q)
	if pq.Empty() {
		return nil, nil
	}
	f := view.MatchFacts{Status: m.prob.status}
	if pq.NeedsContent() {
		c, err := m.content.Get(m.app, m.st)
		if err != nil {
			m.say(sayFail, err.Error())
		}
		f.Content = c
	}
	all := func(it *item) bool { return pq.Match(&it.v, f) }
	var fb func(*item) bool
	if g := pq.Fallback(); g != nil {
		fb = func(it *item) bool { return g(&it.v) }
	}
	return all, fb
}

// refilterCol applies the column's query and keeps the cursor on keepID.
func (m *model) refilterCol(k colKind, keepID string) {
	c := &m.cols[k]
	match, fallback := m.matcher(c.filter)
	c.view = c.view[:0]
	for i := range c.items {
		if match == nil || match(&c.items[i]) {
			c.view = append(c.view, i)
		}
	}
	if len(c.view) == 0 && fallback != nil {
		for i := range c.items {
			if fallback(&c.items[i]) {
				c.view = append(c.view, i)
			}
		}
	}
	c.cursor = min(c.cursor, max(len(c.view)-1, 0))
	if keepID != "" {
		for i, vi := range c.view {
			if c.items[vi].id == keepID {
				c.cursor = i
				break
			}
		}
	}
	c.scroll(m.rowsShown() - 1)
}
