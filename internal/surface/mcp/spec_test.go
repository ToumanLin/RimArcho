package mcp

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/speccheck"
)

// Spec checks for MCP (14 §1-§3): the tool table, the group lists, the destructiveHint and
// openWorldHint lists and the tool counts match Tools() and openWorld; every env parameter
// reads as 14 §3 says; and every snake_case tool-like name in 13, 14, 15, 20, the tool
// descriptions, Instructions and the guide is a tool, a parameter or a parameter's value.

// snake_case names that are none of those, each with a reason.
var toolNameExceptions = map[string]string{
	"list_unplayed": "not implemented (13 §1.2, 12 §3)",
}

func TestSpecMCPTools(t *testing.T) {
	t.Parallel()
	sec2, err := speccheck.Section("14", "2")
	if err != nil {
		t.Fatal(err)
	}
	sec1, err := speccheck.Section("14", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range mcpTableProblems(sec1, sec2, Tools(), openWorld) {
		t.Error(p)
	}
}

var sentenceEnd = regexp.MustCompile(`\.(\s|$)`)

type hintList struct {
	label string // what the failure calls it
	start string // the text that introduces the list
	want  func(tool) bool
}

func mcpTableProblems(sec1, sec2 []speccheck.Line, tools []tool, open map[string]bool) []string {
	var out []string
	byName := map[string]tool{}
	for _, t := range tools {
		byName[t.name] = t
	}
	// The tool table.
	seen := map[string]bool{}
	for _, tab := range speccheck.Tables(sec2) {
		ro, de := speccheck.Column(tab[0], "readOnlyHint"), speccheck.Column(tab[0], "destructiveHint")
		if tab[0].Cells[0] != "Tool" || ro < 0 || de < 0 {
			continue
		}
		for _, r := range tab[1:] {
			spans := speccheck.Spans(r.Cells[0])
			if len(spans) != 1 {
				out = append(out, fmt.Sprintf("%s: 14 §2: a tool row names %d tools", r.Line.Pos(), len(spans)))
				continue
			}
			name := spans[0]
			seen[name] = true
			t, ok := byName[name]
			if !ok {
				out = append(out, fmt.Sprintf("%s: 14 §2: tool %s has a row but is not in Tools() (internal/surface/mcp/server.go)", r.Line.Pos(), name))
				continue
			}
			if got := r.Cells[ro]; got != strconv.FormatBool(t.readOnly) {
				out = append(out, fmt.Sprintf("%s: 14 §2: %s readOnlyHint is %s, the row says %s", r.Line.Pos(), name, strconv.FormatBool(t.readOnly), got))
			}
			want := strconv.FormatBool(t.destructive)
			if t.readOnly {
				want = "—" // New sets no destructiveHint on a read-only tool
			}
			if got := r.Cells[de]; got != want {
				out = append(out, fmt.Sprintf("%s: 14 §2: %s destructiveHint is %s, the row says %s", r.Line.Pos(), name, want, got))
			}
		}
	}
	for _, t := range tools {
		if !seen[t.name] {
			out = append(out, fmt.Sprintf("14 §2: tool %s (Tools()) has no row in the tool table", t.name))
		}
	}
	// The lists in the paragraphs.
	all := strings.Join(lineTexts(sec2), "\n")
	for _, l := range []hintList{
		{"the core group", "Core (served by default): ", func(t tool) bool { return t.group == Core }},
		{"the --tools all group", "Only with `--tools all`: ", func(t tool) bool { return t.group == All }},
		{"destructiveHint: true", "`destructiveHint: true` applies to ", func(t tool) bool { return !t.readOnly && t.destructive }},
		{"openWorldHint: true", "`openWorldHint: true` applies to ", func(t tool) bool { return open[t.name] }},
	} {
		i := strings.Index(all, l.start)
		if i < 0 {
			out = append(out, fmt.Sprintf("14 §2: the list of %s (%q) is missing", l.label, l.start))
			continue
		}
		text := all[i+len(l.start):]
		if loc := sentenceEnd.FindStringIndex(text); loc != nil {
			text = text[:loc[0]]
		}
		var got []string
		for _, s := range speccheck.Spans(text) {
			if _, ok := byName[s]; ok {
				got = append(got, s)
			}
		}
		var want []string
		for _, t := range tools {
			if l.want(t) {
				want = append(want, t.name)
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		for _, n := range want {
			if !slices.Contains(got, n) {
				out = append(out, fmt.Sprintf("14 §2: %s is missing from the list of %s", n, l.label))
			}
		}
		for _, n := range got {
			if !slices.Contains(want, n) {
				out = append(out, fmt.Sprintf("14 §2: %s is in the list of %s, but Tools() does not put it there", n, l.label))
			}
		}
	}
	// The counts in 14 §1.
	core := 0
	for _, t := range tools {
		if t.group == Core {
			core++
		}
	}
	s1 := strings.Join(lineTexts(sec1), "\n")
	for _, c := range []struct {
		re   string
		want int
	}{{`There are (\d+) tools`, len(tools)}, {`the (\d+) tools for`, core}, {`serves all (\d+)`, len(tools)}} {
		m := regexp.MustCompile(c.re).FindStringSubmatch(s1)
		if m == nil {
			out = append(out, fmt.Sprintf("14 §1: %q is missing", c.re))
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n != c.want {
			out = append(out, fmt.Sprintf("14 §1: %q says %d, Tools() has %d", c.re, n, c.want))
		}
	}
	return out
}

func lineTexts(ls []speccheck.Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

// 14 §3: every env parameter has the documented description.
func TestSpecMCPEnvParameter(t *testing.T) {
	t.Parallel()
	sec, err := speccheck.Section("14", "3")
	if err != nil {
		t.Fatal(err)
	}
	var want string
	for _, l := range sec {
		if m := regexp.MustCompile("`env` always reads \"([^\"]+)\"").FindStringSubmatch(l.Text); m != nil {
			want = m[1]
		}
	}
	if want == "" {
		t.Fatal("14 §3: no sentence `env` always reads \"…\"")
	}
	ex := speccheck.NewExceptions("envExceptions (internal/surface/mcp/spec_test.go)", envExceptions)
	n := 0
	for _, tl := range Tools() {
		f, ok := tl.in.FieldByName("Env")
		if !ok {
			continue
		}
		n++
		if got := f.Tag.Get("jsonschema"); got != want && !ex.Has(tl.name) {
			t.Errorf("14 §3: %s's env parameter reads %q; the specification says %q", tl.name, got, want)
		}
	}
	if n < 30 {
		t.Fatalf("only %d tools have an env parameter", n)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

// Tools whose env parameter reads otherwise.
var envExceptions = map[string]string{
	"list_saves":   "env also keeps only that Game Config's saves (14 §2 list_saves), and the description says so",
	"get_save":     "open question: reads \"Game Config; default current\"; the documented wording is 2 bytes longer and tools/list (--tools all) is at its 48 KB budget (TestInitialContextBudget)",
	"check_links":  "open question: no description; adding one puts tools/list (--tools all) over its 48 KB budget (TestInitialContextBudget)",
	"repair_links": "open question: no description; adding one puts tools/list (--tools all) over its 48 KB budget (TestInitialContextBudget)",
}

var snakeRe = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

func TestSpecMCPToolNames(t *testing.T) {
	t.Parallel()
	known, verbs := map[string]bool{}, map[string]bool{}
	for _, d := range Catalog() {
		verb, _, _ := strings.Cut(d.Name, "_")
		verbs[verb] = true
		known[d.Name] = true
		for _, p := range d.Params {
			known[p.Name] = true
			for _, w := range snakeRe.FindAllString(p.Help, -1) {
				known[w] = true // an action or value the parameter lists
			}
		}
	}
	ex := speccheck.NewExceptions("toolNameExceptions (internal/surface/mcp/spec_test.go)", toolNameExceptions)
	var out []string
	for _, nn := range []string{"13", "14", "15", "20"} {
		docs, err := speccheck.SpecDocs(nn)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			for _, l := range d.Lines {
				for _, s := range speccheck.Spans(l.Text) {
					// A span that starts with a tool-like word: `name` or `name key=value`.
					w := strings.Fields(s)
					if len(w) == 0 || !snakeRe.MatchString(w[0]) || snakeRe.FindString(w[0]) != w[0] {
						continue
					}
					out = append(out, toolNameProblem(w[0], l.Pos()+": "+nn, known, verbs, ex)...)
				}
			}
		}
	}
	texts := map[string]string{"Instructions": Instructions}
	for _, d := range Catalog() {
		texts["the "+d.Name+" description"] = d.Description
	}
	for _, topic := range cmds.GuideTopics() {
		g, _ := cmds.Guide(topic)
		texts["guide topic "+topic] = g
	}
	for where, text := range texts {
		for _, w := range snakeRe.FindAllString(text, -1) {
			out = append(out, toolNameProblem(w, where, known, verbs, ex)...)
		}
	}
	sort.Strings(out)
	for _, p := range slices.Compact(out) {
		t.Error(p)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

// toolNameProblem: a snake_case word that starts like a tool (its first part is a tool's
// first part: get_, list_, manage_, …) is a tool, a parameter or a listed value. Other
// snake_case words (JSON fields such as load_seconds) are not about tools.
func toolNameProblem(w, where string, known, verbs map[string]bool, ex *speccheck.Exceptions) []string {
	verb, _, _ := strings.Cut(w, "_")
	if !verbs[verb] || known[w] || ex.Has(w) {
		return nil
	}
	return []string{fmt.Sprintf("%s names %s, which is not an MCP tool, parameter or parameter value (14 §2; fix the name, or add an exception with the reason)", where, w)}
}

func TestSpecMCPDetection(t *testing.T) {
	t.Parallel()
	line := func(s string) speccheck.Line { return speccheck.Line{Doc: "x.md", N: 1, Text: s} }
	tools := []tool{{name: "get_a", group: Core, readOnly: true}, {name: "set_b", group: All, destructive: true}, {name: "set_c", group: All}}
	sec1 := []speccheck.Line{line("There are 3 tools. Serves the 1 tools for diagnosis. `--tools all` serves all 3.")}
	good := []speccheck.Line{
		line("`openWorldHint: true` applies to `set_c` (x). It is false for others."),
		line("| Tool | Command | readOnlyHint | destructiveHint |"), line("| --- | --- | --- | --- |"),
		line("| `get_a` | a | true | — |"), line("| `set_b` | b | false | true |"), line("| `set_c` | c | false | false |"),
		line(""),
		line("**Groups.** Core (served by default): `get_a`. Only with `--tools all`: `set_b` and `set_c`."),
		line("`destructiveHint: true` applies to `set_b`. More."),
	}
	if got := mcpTableProblems(sec1, good, tools, map[string]bool{"set_c": true}); len(got) != 0 {
		t.Errorf("good: %v", got)
	}
	bad := []speccheck.Line{
		line("`openWorldHint: true` applies to `get_a`. It is false for others."),
		line("| Tool | Command | readOnlyHint | destructiveHint |"), line("| --- | --- | --- | --- |"),
		line("| `get_a` | a | true | false |"), line("| `set_b` | b | false | false |"), line("| `gone` | c | false | false |"),
		line(""),
		line("**Groups.** Core (served by default): `get_a`, `set_c`. Only with `--tools all`: `set_b`."),
		line("`destructiveHint: true` applies to `set_b`, `set_c`. More."),
	}
	// get_a's destructive column; set_b's destructive column; gone not a tool; set_c no row;
	// openWorld get_a extra and set_c missing; core set_c extra; all set_c missing; destructive set_c extra;
	// and the counts: "the 1 tools" against 1 core is fine, so none.
	if got := mcpTableProblems(sec1, bad, tools, map[string]bool{"set_c": true}); len(got) != 9 {
		t.Errorf("bad: %d\n%s", len(got), strings.Join(got, "\n"))
	}
	known, verbs := map[string]bool{"get_a": true, "dry_run": true}, map[string]bool{"get": true, "list": true}
	ex := speccheck.NewExceptions("x", map[string]string{"list_gone": "x"})
	var got []string
	for _, w := range []string{"get_a", "dry_run", "load_seconds", "list_gone", "get_gone"} {
		got = append(got, toolNameProblem(w, "x", known, verbs, ex)...)
	}
	if len(got) != 1 || len(ex.Stale()) != 0 {
		t.Errorf("tool names: %v %v", got, ex.Stale())
	}
}
