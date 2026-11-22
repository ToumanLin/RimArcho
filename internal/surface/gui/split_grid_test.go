package gui

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	cssGridRule = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	cssGridCols = regexp.MustCompile(`grid-template-columns\s*:\s*([^;}]+)`)
	cssCompound = regexp.MustCompile(`[.#][\w-]+`)
)

// TestUISplitGrids: a grid that puts cards side by side with a card-gap column between them
// (16 §7 "Card spacing") is a multi-pane page, and its gap is a splitter (16 §3.1 rule 10,
// 16 §7.3 item 12): the page script that builds the grid wires its gap with wireSeparator
// (TestUIRules then holds every `split …` it creates to that). A plain spacer in that column (Feedback's `.fb-gap` until 2026-11-22)
// fails here. Whether the panes' first cards start on one line is checked per page by the
// node suites (testdata/feedback.mjs, exchange.mjs), since it needs the page's structure.
func TestUISplitGrids(t *testing.T) {
	t.Parallel()
	css, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{}
	entries, err := fs.ReadDir(assets, "assets/js")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			b, err := fs.ReadFile(assets, "assets/js/"+e.Name())
			if err != nil {
				t.Fatal(err)
			}
			scripts[e.Name()] = string(b)
		}
	}
	for _, p := range splitGridProblems(string(css), scripts) {
		t.Error(p)
	}
}

// splitGrids returns, for each grid with a card-gap column, the classes and ids of its
// selector's last compound (".pm-page > .pm" gives [".pm"]).
func splitGrids(css string) map[string][]string {
	css = cssComments.ReplaceAllString(css, "")
	out := map[string][]string{}
	for _, m := range cssGridRule.FindAllStringSubmatch(css, -1) {
		cols := cssGridCols.FindStringSubmatch(m[2])
		if cols == nil || !strings.Contains(cols[1], "var(--card-gap)") {
			continue
		}
		for _, sel := range strings.Split(m[1], ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" || strings.HasPrefix(sel, "@") {
				continue
			}
			parts := strings.FieldsFunc(sel, func(r rune) bool { return r == ' ' || r == '>' || r == '+' || r == '~' })
			last := parts[len(parts)-1]
			names := cssCompound.FindAllString(regexp.MustCompile(`:[\w-]+(\([^)]*\))?`).ReplaceAllString(last, ""), -1)
			key := strings.Join(names, "")
			// .page is added by page() (screens.js) to every page's own classes
			names = slicesDelete(names, ".page")
			if len(names) > 0 {
				out[key] = names
			}
		}
	}
	return out
}

func slicesDelete(names []string, drop string) []string {
	out := names[:0:0]
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

func splitGridProblems(css string, scripts map[string]string) []string {
	var problems []string
	grids := splitGrids(css)
	keys := make([]string, 0, len(grids))
	for k := range grids {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		names := grids[key]
		var owners []string
		for file, src := range scripts {
			if mentionsAll(src, names) {
				owners = append(owners, file)
			}
		}
		sort.Strings(owners)
		if len(owners) == 0 {
			problems = append(problems, fmt.Sprintf("assets/style.css: %s has a card-gap column, but no page script builds it; remove the rule or the gap column", key))
			continue
		}
		wired := false
		for _, file := range owners {
			if strings.Contains(scripts[file], "wireSeparator(") {
				wired = true
			}
		}
		if !wired {
			problems = append(problems, fmt.Sprintf("assets/style.css: %s puts cards side by side with a card-gap column, but %s creates no splitter there; make the gap el(\"div\", \"split …\") and wire it with wireSeparator (16 §3.1 rule 10, 16 §7.3 item 12)", key, strings.Join(owners, ", ")))
		}
	}
	return problems
}

// mentionsAll: every class appears as a word in one of the script's string literals; an id
// appears as `#id` or `id = "id"`.
func mentionsAll(src string, names []string) bool {
	for _, n := range names {
		word := regexp.QuoteMeta(n[1:])
		var re *regexp.Regexp
		if n[0] == '.' {
			re = regexp.MustCompile(`["'` + "`" + `][^"'` + "`" + `\n]*\b` + word + `(?:\s[^"'` + "`" + `\n]*)?["'` + "`" + `]`)
		} else {
			re = regexp.MustCompile(`#` + word + `\b|\bid\s*=\s*["']` + word + `["']`)
		}
		if !re.MatchString(src) {
			return false
		}
	}
	return true
}

// The check itself: a spacer fails, a wired splitter passes, a grid no script builds is named.
func TestUISplitGridsDetection(t *testing.T) {
	css := `.a-lay { display: grid; grid-template-columns: 1fr var(--card-gap) 2fr; }
/* .x { grid-template-columns: 1fr var(--card-gap) 1fr } */
.pg > .b-lay:not(.n) { grid-template-columns: 1fr var(--card-gap) 1fr; }
.c-lay { grid-template-columns: 1fr var(--card-gap) 1fr; }
.d { gap: var(--card-gap); grid-template-columns: 1fr 1fr; }`
	scripts := map[string]string{
		"a.js": `const g = el("div", "a-lay"); g.append(l, el("div", "a-gap"), r);`,
		"b.js": "const g = el(\"div\", \"b-lay\"); const gap = el(\"div\", \"split b-split\");\nwireSeparator(gap, {});",
	}
	got := splitGridProblems(css, scripts)
	if len(got) != 2 || !strings.Contains(got[0], ".a-lay") || !strings.Contains(got[0], "a.js") || !strings.Contains(got[1], ".c-lay") {
		t.Fatalf("problems = %q", got)
	}
}
