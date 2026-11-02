package view

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Column search (06 §3.7): words, field:value terms, - negation, "quoted words"; all
// terms must hold.

type term struct {
	field string // "" (free text), name, author, tag, id, type, has, version; tagname (a bar filter: one whole tag)
	value string // lower case
	neg   bool
}

var queryFields = map[string]bool{"name": true, "author": true, "tag": true, "id": true, "type": true, "has": true, "version": true, "location": true, "tags": true, "source": true, "content": true, "lists": true}

// Query is a parsed column search.
type Query struct{ terms []term }

// ParseQuery splits a query into terms; an unknown field: prefix is free text.
func ParseQuery(q string) Query {
	var out []term
	for _, tok := range queryTokens(q) {
		t := term{value: strings.ToLower(tok)}
		if strings.HasPrefix(t.value, "-") && len(t.value) > 1 {
			t.neg, t.value = true, t.value[1:]
		}
		if f, v, ok := strings.Cut(t.value, ":"); ok && queryFields[f] {
			t.field, t.value = f, v
			if t.field == "tags" {
				t.field = "tagname"
			}
		}
		if t.value == "" && t.field == "" {
			continue
		}
		out = append(out, t)
	}
	return Query{out}
}

// queryTokens keeps quoted filter values together, including paths containing spaces.
// Backslashes are literal, so Windows paths need no escaping.
func queryTokens(q string) []string {
	var out []string
	rs := []rune(q)
	for i := 0; i < len(rs); {
		for i < len(rs) && unicode.IsSpace(rs[i]) {
			i++
		}
		if i == len(rs) {
			break
		}
		inQuote := false
		var tok []rune
		for i < len(rs) && (inQuote || !unicode.IsSpace(rs[i])) {
			if rs[i] == '"' {
				inQuote = !inQuote
			} else {
				tok = append(tok, rs[i])
			}
			i++
		}
		out = append(out, string(tok))
	}
	return out
}

// Empty: no terms (everything matches).
func (q Query) Empty() bool { return len(q.terms) == 0 }

// contentTypes are the type: values answered from the file lists.
var contentTypes = map[string]bool{"assembly": true, "xml": true, "xml-only": true, "language": true, "language-pack": true, "lang": true}

// NeedsContent reports whether the query uses the content kinds (one query over the file
// lists, made only then).
func (q Query) NeedsContent() bool {
	for _, t := range q.terms {
		if (t.field == "type" || t.field == "content") && contentTypes[t.value] {
			return true
		}
	}
	return false
}

// MatchFacts are what matching needs beyond the row: content kinds by instance (Contents) and
// the row status (06 §3.7 has:problem).
type MatchFacts struct {
	Content map[string]int
	Status  map[string]int
}

// Match reports whether a row satisfies every term.
func (q Query) Match(it *Item, f MatchFacts) bool {
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
func (q Query) Filter(items []Item, f MatchFacts) []int {
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

func termMatch(t term, it *Item, f MatchFacts) bool {
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
			if TagMatch(tg, v) {
				return true
			}
		}
		return false
	case "tagname":
		return slices.ContainsFunc(it.Tags, func(tg string) bool { return strings.EqualFold(tg, v) })
	case "version":
		return slices.Contains(it.Versions, v)
	case "lists":
		return slices.ContainsFunc(it.Lists, func(name string) bool { return strings.EqualFold(name, v) })
	case "location":
		return v != "" && strings.EqualFold(it.Location, v)
	case "id":
		return v != "" && (strings.Contains(it.Package, v) || it.Workshop == v || strings.EqualFold(it.Ref, v) || strings.EqualFold(strings.TrimPrefix(it.Ref, "#"), v))
	case "source":
		switch v {
		case "steamworkshop":
			v = "workshop"
		case "localmods":
			v = "local"
		case "archolocked", "copy":
			v = "locked"
		case "extra":
			v = "other"
		}
		if len(it.Sources) > 0 {
			return slices.ContainsFunc(it.Sources, func(s string) bool { return app.LocalMatches(v, s) })
		}
		return it.Kind == v || v == "steamcmd" && it.SteamCMD
	case "content":
		return termMatch(term{field: "type", value: v}, it, f)
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
			return s != StatusNone && s != StatusUnloadable && s != StatusHint
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
	key := a.Store.IndexKey()
	if st != nil {
		key += "|" + st.Ix.Env.ID
	}
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

// TagMatch: v names tag when, ignoring case, it starts at the beginning of one of the tag's
// words (06 §3.7): "facial" finds "Facial Hair" and "Facial Animation", "animation" and
// "anim" find "Facial Animation", "facial hair" finds "Facial Hair"; "air" does not.
// The rule is app.TagMatch, so Mod library's tag: filter (app.Library) matches the same way.
func TagMatch(tag, v string) bool { return app.TagMatch(tag, v) }

func isTagSep(c byte) bool { return app.IsTagSep(c) }

// queryFieldOrder is the order completion offers fields in when several share a prefix.
var queryFieldOrder = []string{"lists", "name", "author", "tag", "id", "type", "has", "version", "location"}

// queryValues are the fixed values of type: and has: (06 §3.7), in the order offered.
var queryValues = map[string][]string{
	"type": {"workshop", "local", "official", "copy", "git", "steamcmd", "assembly", "xml", "language"},
	"has":  {"problem", "error", "tag", "note", "alias"},
}

// CompleteQuery is the grey text to show after the search box's last word, which tab
// accepts (06 §3.7), from the second letter on: "au" → "thor:", "-ta" → "g:", "type:wo" → "rkshop", "tag:fac" →
// "ial" (from the tags in use, up to the end of the word). "" when nothing fits or the
// last word is inside quotes.
func CompleteQuery(q string, tags []string) string { return CompleteQueryWith(q, tags, nil) }

// CompleteQueryWith is CompleteQuery with the game versions the rows support (version:).
func CompleteQueryWith(q string, tags, versions []string) string {
	return completeWith(q, queryFieldOrder, func(f string) []string {
		switch f {
		case "tag":
			return tagWords(tags)
		case "version":
			return versions
		}
		return queryValues[f]
	})
}

// completeWith is CompleteQuery for a set of fields (in the order offered) and their
// values; Mod library's search box uses it with its own keys (CompleteLibrary).
func completeWith(q string, fields []string, values func(field string) []string) string {
	if q == "" || strings.HasSuffix(q, " ") || strings.Count(q, "\"")%2 == 1 {
		return ""
	}
	tok := q[strings.LastIndexByte(q, ' ')+1:]
	tok = strings.TrimPrefix(tok, "-")
	low := strings.ToLower(tok)
	if f, v, ok := strings.Cut(low, ":"); ok {
		if !slices.Contains(fields, f) || v == "" {
			return ""
		}
		for _, c := range values(f) {
			if len(c) > len(v) && strings.HasPrefix(strings.ToLower(c), v) {
				return c[len(v):]
			}
		}
		return ""
	}
	// two letters at least: a single letter is usually the start of a name, and tab after
	// it should still go to the next list
	if len(low) < 2 {
		return ""
	}
	for _, f := range fields {
		if strings.HasPrefix(f, low) {
			return f[len(low):] + ":"
		}
	}
	return ""
}

// completeLists retains quotes and completes whole list names, including spaces.
func completeLists(q string, names []string, fallback, omit string) string {
	inQuote, start := false, 0
	for i, r := range q {
		if r == '"' {
			inQuote = !inQuote
		} else if unicode.IsSpace(r) && !inQuote {
			start = i + len(string(r))
		}
	}
	token := strings.TrimPrefix(q[start:], "-")
	field, value, ok := strings.Cut(token, ":")
	if !ok || !strings.EqualFold(field, "lists") {
		return fallback
	}
	quoted := strings.HasPrefix(value, "\"")
	if quoted && strings.HasSuffix(value[1:], "\"") {
		return ""
	}
	prefix := strings.TrimPrefix(value, "\"")
	for _, name := range names {
		if omit != "" && strings.EqualFold(name, omit) || len(prefix) > len(name) || !strings.EqualFold(name[:len(prefix)], prefix) {
			continue
		}
		if !quoted && strings.ContainsAny(name, " \t\r\n") {
			continue // a suffix cannot insert an opening quote
		}
		suffix := name[len(prefix):]
		if quoted {
			suffix += "\""
		}
		return suffix
	}
	return ""
}

// tagWords: every word of the tags in use, most used first, then by name; "Facial Hair"
// gives "Facial" and "Hair".
func tagWords(tags []string) []string {
	n := map[string]int{}
	for _, t := range tags {
		for _, w := range strings.FieldsFunc(t, func(r rune) bool { return r < 128 && isTagSep(byte(r)) }) {
			n[w]++
		}
	}
	out := make([]string, 0, len(n))
	for w := range n {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}
