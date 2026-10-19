package tui

import (
	"strings"

	"github.com/rimarcho/rimarcho/internal/store"
)

// Column search (06 §5.5): words, field:value terms, - negation, "quoted words"; all
// terms must hold.

type term struct {
	field string // "" (free text), name, author, tag, id, type, has
	value string // lower case
	neg   bool
}

var queryFields = map[string]bool{"name": true, "author": true, "tag": true, "id": true, "type": true, "has": true}

// parseQuery splits a query into terms; an unknown field: prefix is free text.
func parseQuery(q string) []term {
	var out []term
	rs := []rune(q)
	for i := 0; i < len(rs); {
		for i < len(rs) && rs[i] == ' ' {
			i++
		}
		if i == len(rs) {
			break
		}
		start := i
		inQuote := false
		var tok []rune
		for i < len(rs) && (inQuote || rs[i] != ' ') {
			if rs[i] == '"' {
				inQuote = !inQuote
			} else {
				tok = append(tok, rs[i])
			}
			i++
		}
		_ = start
		t := term{value: strings.ToLower(string(tok))}
		if strings.HasPrefix(t.value, "-") && len(t.value) > 1 {
			t.neg, t.value = true, t.value[1:]
		}
		if f, v, ok := strings.Cut(t.value, ":"); ok && queryFields[f] {
			t.field, t.value = f, v
		}
		if t.value == "" && t.field == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// needsContent reports whether a query uses the content kinds (one query over the file
// lists, made only then).
func needsContent(ts []term) bool {
	for _, t := range ts {
		if t.field == "type" && (t.value == "assembly" || t.value == "xml" || t.value == "language") {
			return true
		}
	}
	return false
}

// matches reports whether an item satisfies one term.
func (m *model) termMatch(t term, it *item) bool {
	v := t.value
	switch t.field {
	case "":
		return strings.Contains(it.text, v)
	case "name":
		return strings.Contains(it.names, v)
	case "author":
		return strings.Contains(it.author, v)
	case "tag":
		for _, tg := range it.tags {
			if strings.EqualFold(tg, v) {
				return true
			}
		}
		return false
	case "id":
		return v != "" && (strings.Contains(it.pkg, v) || it.ws == v || strings.EqualFold(it.ref, v) || strings.EqualFold(strings.TrimPrefix(it.ref, "#"), v))
	case "type":
		switch v { // the long forms are the names in the request (xml-only, language-pack)
		case "xml-only":
			v = "xml"
		case "language-pack", "lang":
			v = "language"
		}
		switch v {
		case "official", "workshop", "local", "copy", "git":
			return it.kind == v
		case "steamcmd":
			return it.steamcmd
		case "assembly", "xml", "language":
			f, ok := m.content[it.id]
			if !ok {
				return false
			}
			switch v {
			case "assembly":
				return f&store.HasDLL != 0
			case "xml":
				return f&store.HasDLL == 0
			default:
				return f&store.HasDLL == 0 && f&store.HasDefsPatch == 0 && f&store.HasLanguages != 0
			}
		}
		return false
	case "has":
		switch v {
		case "problem", "problems":
			s := m.rowStatus(it)
			return s != statusNone && s != statusUnloadable
		case "error", "errors":
			s := m.rowStatus(it)
			return s == statusErr || s == statusLog
		case "tag", "tags":
			return len(it.tags) > 0
		case "note":
			return it.note != ""
		case "alias":
			return it.alias
		}
		return false
	}
	return false
}

// matcher builds the column predicate. The second result is the fallback for a single
// free word that matched nothing: its letters in order in the name.
func (m *model) matcher(q string) (func(*item) bool, func(*item) bool) {
	ts := parseQuery(q)
	if len(ts) == 0 {
		return nil, nil
	}
	if needsContent(ts) {
		m.loadContent()
	}
	all := func(it *item) bool {
		for _, t := range ts {
			if m.termMatch(t, it) == t.neg {
				return false
			}
		}
		return true
	}
	var fb func(*item) bool
	if len(ts) == 1 && ts[0].field == "" && !ts[0].neg {
		w := ts[0].value
		fb = func(it *item) bool { return subsequence(w, it.names) }
	}
	return all, fb
}

// loadContent fills the content kinds once per database state.
func (m *model) loadContent() {
	key := m.app.Store.ChangeKey()
	if m.content != nil && m.contentKey == key {
		return
	}
	snaps := map[string]int64{}
	if m.st != nil {
		for _, mod := range m.st.Ix.Mods {
			snaps[mod.ID] = mod.SnapID
		}
	}
	c, err := m.app.Store.ContentKinds(snaps)
	if err != nil {
		m.say(sayFail, err.Error())
		c = map[string]int{}
	}
	m.content, m.contentKey = c, key
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
