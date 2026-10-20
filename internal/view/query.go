package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Column search (06 §5.5): words, field:value terms, - negation, "quoted words"; all
// terms must hold.

type term struct {
	field string // "" (free text), name, author, tag, id, type, has
	value string // lower case
	neg   bool
}

var queryFields = map[string]bool{"name": true, "author": true, "tag": true, "id": true, "type": true, "has": true}

// Query is a parsed column search.
type Query struct{ terms []term }

// ParseQuery splits a query into terms; an unknown field: prefix is free text.
func ParseQuery(q string) Query {
	var out []term
	rs := []rune(q)
	for i := 0; i < len(rs); {
		for i < len(rs) && rs[i] == ' ' {
			i++
		}
		if i == len(rs) {
			break
		}
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
	return Query{out}
}

// Empty: no terms (everything matches).
func (q Query) Empty() bool { return len(q.terms) == 0 }

// contentTypes are the type: values answered from the file lists.
var contentTypes = map[string]bool{"assembly": true, "xml": true, "xml-only": true, "language": true, "language-pack": true, "lang": true}

// NeedsContent reports whether the query uses the content kinds (one query over the file
// lists, made only then).
func (q Query) NeedsContent() bool {
	for _, t := range q.terms {
		if t.field == "type" && contentTypes[t.value] {
			return true
		}
	}
	return false
}

// Facts are what matching needs beyond the row: content kinds by instance (Contents) and
// the row status (06 §5.5 has:problem).
type Facts struct {
	Content map[string]int
	Status  map[string]int
}

// Match reports whether a row satisfies every term.
func (q Query) Match(it *Item, f Facts) bool {
	for _, t := range q.terms {
		if termMatch(t, it, f) == t.neg {
			return false
		}
	}
	return true
}

// Fallback is the match for a single free word that matched nothing: its letters in order
// in the name ("hsl" finds "Hair Style Lib"). Nil when the query is not one free word.
func (q Query) Fallback() func(*Item) bool {
	if len(q.terms) != 1 || q.terms[0].field != "" || q.terms[0].neg {
		return nil
	}
	w := q.terms[0].value
	return func(it *Item) bool { return Subsequence(w, it.names) }
}

// Filter returns the indexes of the rows that match, with the fallback when nothing does.
func (q Query) Filter(items []Item, f Facts) []int {
	out := []int{}
	for i := range items {
		if q.Empty() || q.Match(&items[i], f) {
			out = append(out, i)
		}
	}
	if fb := q.Fallback(); len(out) == 0 && fb != nil {
		for i := range items {
			if fb(&items[i]) {
				out = append(out, i)
			}
		}
	}
	return out
}

func termMatch(t term, it *Item, f Facts) bool {
	v := t.value
	switch t.field {
	case "":
		return strings.Contains(it.text, v)
	case "name":
		return strings.Contains(it.names, v)
	case "author":
		return strings.Contains(it.author, v)
	case "tag":
		for _, tg := range it.Tags {
			if strings.EqualFold(tg, v) {
				return true
			}
		}
		return false
	case "id":
		return v != "" && (strings.Contains(it.Package, v) || it.Workshop == v || strings.EqualFold(it.Ref, v) || strings.EqualFold(strings.TrimPrefix(it.Ref, "#"), v))
	case "type":
		switch v { // the long forms are the names in the request (xml-only, language-pack)
		case "xml-only":
			v = "xml"
		case "language-pack", "lang":
			v = "language"
		}
		switch v {
		case "official", "workshop", "local", "copy", "git":
			return it.Kind == v
		case "steamcmd":
			return it.SteamCMD
		case "assembly", "xml", "language":
			c, ok := f.Content[it.ID]
			if !ok {
				return false
			}
			switch v {
			case "assembly":
				return c&store.HasDLL != 0
			case "xml":
				return c&store.HasDLL == 0
			default:
				return c&store.HasDLL == 0 && c&store.HasDefsPatch == 0 && c&store.HasLanguages != 0
			}
		}
		return false
	case "has":
		switch v {
		case "problem", "problems":
			s := RowStatus(it, f.Status)
			return s != StatusNone && s != StatusUnloadable
		case "error", "errors":
			s := RowStatus(it, f.Status)
			return s == StatusErr || s == StatusLog
		case "tag", "tags":
			return len(it.Tags) > 0
		case "note":
			return it.Note != ""
		case "alias":
			return it.Alias
		}
		return false
	}
	return false
}

// Subsequence: the letters of q appear in s in order.
func Subsequence(q, s string) bool {
	rs := []rune(s)
	i := 0
	for _, r := range q {
		for i < len(rs) && rs[i] != r {
			i++
		}
		if i == len(rs) {
			return false
		}
		i++
	}
	return true
}

// ContentCache keeps the content kinds per database state.
type ContentCache struct {
	key   string
	kinds map[string]int
}

// Get returns the content kinds of the state's instances, loading them once per database
// state.
func (c *ContentCache) Get(a *app.App, st *app.LoadoutState) (map[string]int, error) {
	key := a.Store.ChangeKey()
	if c.kinds != nil && c.key == key {
		return c.kinds, nil
	}
	snaps := map[string]int64{}
	if st != nil {
		for _, mod := range st.Ix.Mods {
			snaps[mod.ID] = mod.SnapID
		}
	}
	k, err := a.Store.ContentKinds(snaps)
	if err != nil {
		return map[string]int{}, err
	}
	c.kinds, c.key = k, key
	return k, nil
}
