package view

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The filter bar of the plan editor (16 §2.5, §3.7): one search box and one set of filters
// for Not enabled and Enabled together. The bar's groups and values are the same JSON as Mod library's
// (LibGroup, LibChoice, LibCond), so both pages draw them with one component.
//
// A filter is "group:value" ("-group:value" leaves out). Within a group the values are
// "or", between groups "and", as in Mod library (16 §2.9). The search box is the column search
// of 16 §3.7 and may hold tag: type: has: version: words of its own.

// BarKeys are the editor bar's groups in the order shown.
var BarKeys = []string{"lists", "tag", "source", "location", "version", "category", "content", "has", "added"}

// barValues are the fixed values of a group, in the order shown (tags and versions come
// from the rows).
var barValues = map[string][]string{
	"source":   {"workshop", "steamcmd", "local", "locked", "official", "other"},
	"content":  {"assembly", "xml", "language"},
	"category": app.CategoryKeys,
	"has":      {"problem", "error", "note", "alias", "notag"},
	"added":    app.LibAdded,
}

// BarFilter is the bar's result: the matching rows of Not enabled and Enabled (indexes into the
// columns), and the groups when asked for (the bar is open).
type BarFilter struct {
	Rows     [2][]int   `json:"rows"`
	Groups   []LibGroup `json:"groups,omitempty"`
	Conds    []LibCond  `json:"conds"`
	Complete string     `json:"complete,omitempty"`
}

// barTerm turns a bar filter into the query term it means.
func barTerm(key, v string) term {
	switch key {
	case "content":
		return term{field: "type", value: v}
	case "tag":
		return term{field: "tagname", value: v}
	}
	return term{field: key, value: v}
}

// barCond is the filters grouped: group → included values, and every excluded term.
type barCond struct {
	inc map[string][]term
	exc []term
}

func parseBar(filters []string) barCond {
	c := barCond{inc: map[string][]term{}}
	for _, f := range filters {
		ex := strings.HasPrefix(f, "-")
		k, v, ok := strings.Cut(strings.TrimPrefix(f, "-"), ":")
		if !ok || v == "" || !slices.Contains(BarKeys, k) {
			continue
		}
		t := barTerm(k, strings.ToLower(v))
		if ex {
			c.exc = append(c.exc, t)
		} else {
			c.inc[k] = append(c.inc[k], t)
		}
	}
	return c
}

// passes: the row holds one included value of every group but skip, and no excluded one.
func (c barCond) passes(it *Item, f MatchFacts, skip string) bool {
	for k, ts := range c.inc {
		if k == skip {
			continue
		}
		if !slices.ContainsFunc(ts, func(t term) bool { return termMatch(t, it, f) }) {
			return false
		}
	}
	for _, t := range c.exc {
		if termMatch(t, it, f) {
			return false
		}
	}
	return true
}

// barRowValues are a row's values for a group.
func barRowValues(it *Item, key string, f MatchFacts) []string {
	switch key {
	case "lists":
		return it.Lists
	case "tag":
		return it.Tags
	case "source":
		if len(it.Sources) > 0 {
			return it.Sources
		}
	case "version":
		return it.Versions
	case "location":
		if it.Location != "" {
			return []string{it.Location}
		}
		return nil
	}
	var out []string
	for _, v := range barValues[key] {
		if termMatch(barTerm(key, v), it, f) {
			out = append(out, v)
		}
	}
	return out
}

// BarNeedsContent: the content group or a type: word of the search needs the file lists.
func BarNeedsContent(search string, filters []string, groups bool) bool {
	if groups || ParseQuery(search).NeedsContent() {
		return true
	}
	for _, f := range filters {
		if strings.HasPrefix(strings.TrimPrefix(f, "-"), "content:") {
			return true
		}
	}
	return false
}

// FilterBoard applies the bar to Not enabled and Enabled. groups adds the groups with their counts:
// each group's counts ignore that group's own choices, so its other values say what adding
// them would bring (as Mod library's). game is the environment's version ("1.6"), listed first
// and marked.
func FilterBoard(b *Board, search string, filters []string, f MatchFacts, groups bool, game string) *BarFilter {
	out := &BarFilter{Conds: []LibCond{}}
	words, typed := splitBoardQuery(search)
	filters = append(append([]string(nil), filters...), typed...)
	words, filters = currentVersion(words, game), slices.Clone(filters)
	for i, f := range filters {
		filters[i] = currentVersion(f, game)
	}
	q := ParseQuery(words)
	c := parseBar(filters)
	out.Complete = boardComplete(b, search)
	t := barTally{counts: map[string]map[string]int{}, shown: map[string]map[string]string{}}
	for _, k := range BarKeys {
		t.counts[k], t.shown[k] = map[string]int{}, map[string]string{}
	}
	if b.Lists != nil {
		for _, name := range b.Lists.Names {
			t.shown["lists"][strings.ToLower(name)] = name
		}
	}
	for col := 0; col < 2; col++ {
		out.Rows[col] = t.filterColumn(b.Cols[col], q, c, f, groups)
	}
	on := t.chosen(out, filters, groups)
	if !groups {
		return out
	}
	for _, k := range BarKeys {
		g := LibGroup{Key: k, Title: barKeyTitle(k)}
		for lv, v := range t.shown[k] {
			if k == "lists" && b.Lists != nil && strings.EqualFold(v, b.Lists.Edited) {
				continue // the list being edited is not an available-list choice
			}
			cur := k == "version" && v == game || k == "lists" && b.Lists != nil && strings.EqualFold(v, b.Lists.Current)
			g.Values = append(g.Values, LibChoice{Filter: k + ":" + v, Label: boardValueText(b, k, v), Count: t.counts[k][lv], On: on[k+":"+lv], Cur: cur})
		}
		sortBar(k, g.Values)
		out.Groups = append(out.Groups, g)
	}
	out.Groups = append(moveNoTag(out.Groups), boardTextGroups(b)...)
	return out
}

// moveNoTag puts has:notag ("no tag", the Mods without a tag) last in the tags group, where a
// player checking their tags looks for it (16 §3.7), rather than among has:'s values. Its
// filter stays has:notag, so it counts and toggles as a has: value.
func moveNoTag(groups []LibGroup) []LibGroup {
	var none *LibChoice
	for gi := range groups {
		if groups[gi].Key != "has" {
			continue
		}
		vs := groups[gi].Values
		for i := range vs {
			if vs[i].Filter == "has:notag" {
				c := vs[i]
				none = &c
				groups[gi].Values = append(vs[:i:i], vs[i+1:]...)
				break
			}
		}
	}
	if none == nil {
		return groups
	}
	for gi := range groups {
		if groups[gi].Key == "tag" {
			if len(groups[gi].Values) == 0 && none.On == 0 {
				return groups // nothing is tagged: no tag would be every Mod
			}
			groups[gi].Values = append(groups[gi].Values, *none)
			return groups
		}
	}
	return groups
}

// barTally is each bar group's values as shown and their counts of rows in the search.
type barTally struct {
	counts map[string]map[string]int
	shown  map[string]map[string]string // group → lower value → as shown
}

// boardComplete is the search's completions: the board's tags, versions and lists.
func boardComplete(b *Board, search string) string {
	var tags, vers []string
	for k := range b.Cols {
		for i := range b.Cols[k] {
			tags = append(tags, b.Cols[k][i].Tags...)
			for _, v := range b.Cols[k][i].Versions {
				if !slices.Contains(vers, v) {
					vers = append(vers, v)
				}
			}
		}
	}
	sort.Slice(vers, func(i, j int) bool { return versionNewer(vers[i], vers[j]) })
	complete := CompleteQueryWith(search, tags, append([]string{"current"}, vers...))
	if b.Lists != nil {
		complete = completeLists(search, b.Lists.Names, complete, b.Lists.Edited)
	}
	return complete
}

// filterColumn is the rows of one column that pass the search and the bar; groups also
// counts each group's values ignoring that group's own choices.
func (t barTally) filterColumn(items []Item, q Query, c barCond, f MatchFacts, groups bool) []int {
	found := q.Filter(items, f) // the search's rows (with its fallback)
	inSearch := make([]bool, len(items))
	for _, i := range found {
		inSearch[i] = true
	}
	rows := []int{}
	for i := range items {
		it := &items[i]
		if groups {
			for _, k := range BarKeys {
				for _, v := range barRowValues(it, k, f) {
					lv := strings.ToLower(v)
					if _, ok := t.shown[k][lv]; !ok {
						t.shown[k][lv] = v
					}
					if inSearch[i] && c.passes(it, f, k) {
						t.counts[k][lv]++
					}
				}
			}
		}
		if inSearch[i] && c.passes(it, f, "") {
			rows = append(rows, i)
		}
	}
	return rows
}

// chosen adds the bar's chosen filters to out's conditions and returns each one's state
// by "group:value": 1 chosen, -1 excluded.
func (t barTally) chosen(out *BarFilter, filters []string, groups bool) map[string]int {
	on := map[string]int{}
	for _, x := range filters {
		ex := strings.HasPrefix(x, "-")
		k, v, ok := strings.Cut(strings.TrimPrefix(x, "-"), ":")
		if !ok || !slices.Contains(BarKeys, k) {
			continue
		}
		key := k + ":" + strings.ToLower(v)
		if ex {
			on[key] = -1
		} else {
			on[key] = 1
		}
		out.Conds = append(out.Conds, LibCond{Filter: x, Label: barKeyTitle(k) + " " + barValueText(k, v), Exclude: ex})
		if groups {
			if _, ok := t.shown[k][strings.ToLower(v)]; !ok {
				t.shown[k][strings.ToLower(v)] = v // a chosen value no row has any more still shows
			}
		}
	}
	return on
}

// boardTextGroups are the name, author and id groups: every value the board has.
func boardTextGroups(b *Board) []LibGroup {
	var out []LibGroup
	for _, key := range []string{"name", "author", "id"} {
		g := LibGroup{Key: key, Title: libKeyTitle(key)}
		seen := map[string]bool{}
		for _, items := range b.Cols {
			for _, it := range items {
				value := map[string]string{"name": it.Name, "author": it.Author, "id": it.Package}[key]
				if value != "" && !seen[value] {
					seen[value] = true
					g.Values = append(g.Values, LibChoice{Filter: key + ":" + value, Label: value})
				}
			}
		}
		out = append(out, g)
	}
	return out
}

// sortBar: fixed values in their order; tags most used first; versions newest first with
// the game's own version leading.
func sortBar(k string, vs []LibChoice) {
	val := func(c LibChoice) string { return c.Filter[len(k)+1:] }
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.Cur != b.Cur {
			return a.Cur
		}
		switch k {
		case "version":
			return versionNewer(val(a), val(b))
		case "lists", "tag", "location":
			if a.Count != b.Count {
				return a.Count > b.Count
			}
			return strings.ToLower(val(a)) < strings.ToLower(val(b))
		}
		return slices.Index(barValues[k], val(a)) < slices.Index(barValues[k], val(b))
	})
}

func barKeyTitle(k string) string {
	return map[string]string{"lists": T("lists"), "tag": T("tags"), "source": T("library source"), "location": T("location"), "version": T("supported versions"), "content": T("content"), "category": T("category"), "has": T("has"), "added": T("added")}[k]
}

func barValueText(k, v string) string {
	switch k {
	case "source":
		return LibSourceText(v)
	case "legacy-source":
		return T(map[string]string{"workshop": "workshop", "local": "local", "official": "official", "copy": "local copy", "git": "git", "steamcmd": "SteamCMD"}[v])
	case "content":
		return T(map[string]string{"assembly": "with a DLL", "xml": "XML only", "language": "language pack"}[v])
	case "category":
		return CategoryText(v)
	case "has":
		return T(map[string]string{"problem": "a problem", "error": "an error", "note": "a note", "alias": "an alias", "notag": "no tag"}[v])
	case "added":
		return addedText(v)
	}
	return v
}

// ---- moves while the bar filters (16 §3.5) ----

// StepAt is where J / K put the mods while the list shows only some rows: past the next
// shown row (d rows for alt+J / alt+K), so the moving rows trade places with the rows the
// player sees. visible are the shown rows of Enabled (by load position, 1-based, in order);
// moving the positions of the mods that move. The answer is a 1-based place in the list
// without the moving mods (Place, MoveBlock); 0 when there is nowhere to go.
// With every row shown and a block of neighbouring rows it is MoveBy's place.
func StepAt(visible []int, moving []int, d int) int {
	if len(moving) == 0 || d == 0 {
		return 0
	}
	mv := map[int]bool{}
	for _, p := range moving {
		mv[p] = true
	}
	first, last := slices.Min(moving), slices.Max(moving)
	// the shown rows that do not move, in order
	var others []int
	for _, p := range visible {
		if !mv[p] {
			others = append(others, p)
		}
	}
	before := func(p int) int { // moving mods ahead of position p
		n := 0
		for _, q := range moving {
			if q < p {
				n++
			}
		}
		return n
	}
	if d > 0 {
		// the d-th shown row after the block's last row
		var after []int
		for _, p := range others {
			if p > last {
				after = append(after, p)
			}
		}
		if len(after) == 0 {
			return 0
		}
		p := after[min(d, len(after))-1]
		return p - before(p) + 1 // just after it
	}
	var ahead []int
	for _, p := range others {
		if p < first {
			ahead = append(ahead, p)
		}
	}
	if len(ahead) == 0 {
		return 0
	}
	p := ahead[max(len(ahead)+d, 0)]
	return p - before(p) // just before it
}

// splitBoardQuery shares the bar's group semantics with typed conditions.
func splitBoardQuery(text string) (string, []string) {
	var words, filters []string
	for _, token := range queryTokens(text) {
		raw := strings.TrimPrefix(token, "-")
		k, v, ok := strings.Cut(raw, ":")
		if k == "tags" {
			k = "tag"
		}
		if ok && slices.Contains(BarKeys, k) && !strings.HasPrefix(raw, "tag:") {
			if v != "" {
				prefix := ""
				if strings.HasPrefix(token, "-") {
					prefix = "-"
				}
				filters = append(filters, prefix+k+":"+v)
			}
			continue
		}
		words = append(words, strconv.Quote(token))
	}
	return strings.Join(words, " "), filters
}
func boardValueText(b *Board, k, v string) string {
	if k == "source" {
		for _, col := range b.Cols {
			for _, it := range col {
				if name := it.SourceNames[v]; name != "" {
					return name
				}
			}
		}
		return LibSourceText(v)
	}
	return barValueText(k, v)
}

// currentVersion reads version:current (also -version:current) as the game's own version
// (16 §3.7), so one filter keeps meaning "runs on my game" after the game updates.
func currentVersion(q, game string) string {
	if game == "" || !strings.Contains(strings.ToLower(q), "version:current") {
		return q
	}
	toks := strings.Fields(q)
	for i, t := range toks {
		neg := strings.HasPrefix(t, "-")
		if strings.EqualFold(strings.TrimPrefix(t, "-"), "version:current") {
			toks[i] = map[bool]string{true: "-", false: ""}[neg] + "version:" + game
		}
	}
	return strings.Join(toks, " ")
}
