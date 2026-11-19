package cmds

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/speccheck"
)

// Spec checks for the CLI (13 §3.1, §4): the command tables document every command of
// Commands() and its flags, the settings keys match configKeys and freeKeys with their
// values, and rule 1 of 13 §3.1 names exactly the commands that print JSON.

// Documented command spans that name no registered command, and registered commands the
// tables leave out.
var commandExceptions = map[string]string{
	"tui":      "archo tui is handled by cmd/archo before the command table (13 §4.1)",
	"help":     "archo help is handled by cmd/archo before the command table (13 §4.1)",
	"unplayed": "not implemented (12 §3); the row documents the planned command",
}

// Flags that differ between a command and its rows: "<command> --<flag>".
var flagExceptions = map[string]string{}

// Flags every command takes (13 §4.1).
var globalFlags = map[string]bool{"env": true, "open-plan": true, "help": true, "version": true}

var (
	plainWordsRe = regexp.MustCompile(`^[a-z][a-z0-9-]*(\|[a-z][a-z0-9-]*)*$`)
	optWordRe    = regexp.MustCompile(`^\[([a-z][a-z0-9-]*)\]$`)
	flagRe       = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)
)

// docCommand is one command span of a table row, resolved against the registry.
type docCommand struct {
	span  string
	cmds  []*Command
	flags []string
	row   speccheck.Row
}

// resolveSpan finds the commands a usage span names: its leading plain words, with a|b
// alternatives and [word] optional words expanded, each matched to the longest registered
// command name that prefixes it.
func resolveSpan(span string, reg []*Command) ([]*Command, bool) {
	s := strings.TrimSpace(span)
	if s == "archo" {
		return nil, true
	}
	s = strings.TrimPrefix(s, "archo ")
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "<") {
		return nil, true // a global flag or a placeholder, not a command
	}
	seqs := [][]string{nil}
	for _, tok := range strings.Fields(s) {
		if m := optWordRe.FindStringSubmatch(tok); m != nil {
			var next [][]string
			for _, q := range seqs {
				next = append(next, q, append(slices.Clone(q), m[1]))
			}
			seqs = next
			continue
		}
		if !plainWordsRe.MatchString(tok) {
			break
		}
		var next [][]string
		for _, q := range seqs {
			for _, alt := range strings.Split(tok, "|") {
				next = append(next, append(slices.Clone(q), alt))
			}
		}
		seqs = next
	}
	var out []*Command
	for _, q := range seqs {
		var best *Command
		for _, c := range reg {
			w := strings.Fields(c.Name)
			if len(w) <= len(q) && slices.Equal(w, q[:len(w)]) && (best == nil || len(w) > len(strings.Fields(best.Name))) {
				best = c
			}
		}
		if best == nil {
			return nil, false
		}
		if !slices.Contains(out, best) {
			out = append(out, best)
		}
	}
	return out, true
}

func commandTableRows(t *testing.T) []speccheck.Row {
	t.Helper()
	var rows []speccheck.Row
	for _, sec := range []struct{ num, col string }{{"4.1", "Syntax"}, {"4.2", "Command"}, {"4.3", "Command"}} {
		lines, err := speccheck.Section("13", sec.num)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tab := range speccheck.Tables(lines) {
			col := speccheck.Column(tab[0], sec.col)
			if col < 0 {
				continue
			}
			found = true
			for _, r := range tab[1:] {
				// The usage cell first, then the rest of the row.
				cells := append([]string{r.Cells[col]}, slices.Delete(slices.Clone(r.Cells), col, col+1)...)
				rows = append(rows, speccheck.Row{Cells: cells, Line: r.Line})
			}
		}
		if !found {
			t.Fatalf("13 §%s: no table with a %s column", sec.num, sec.col)
		}
	}
	return rows
}

func documentedCommands(rows []speccheck.Row, reg []*Command, ex *speccheck.Exceptions) ([]docCommand, []string) {
	var out []docCommand
	var problems []string
	for _, r := range rows {
		for _, s := range speccheck.Spans(r.Cells[0]) {
			cs, ok := resolveSpan(s, reg)
			if !ok {
				w := strings.Fields(strings.TrimPrefix(s, "archo "))[0]
				if !ex.Has(w) {
					problems = append(problems, fmt.Sprintf("%s: 13 §4.2: `%s` names no command of cmds.Commands() (fix the row, or register the command)", r.Line.Pos(), s))
				}
				continue
			}
			if len(cs) == 0 {
				continue
			}
			var flags []string
			for _, m := range flagRe.FindAllStringSubmatch(s, -1) {
				flags = append(flags, m[1])
			}
			out = append(out, docCommand{span: s, cmds: cs, flags: flags, row: r})
		}
	}
	return out, problems
}

func TestSpecCommandTable(t *testing.T) {
	t.Parallel()
	reg := allCommands()
	ex := speccheck.NewExceptions("commandExceptions (internal/cmds/spec_test.go)", commandExceptions)
	fex := speccheck.NewExceptions("flagExceptions (internal/cmds/spec_test.go)", flagExceptions)
	rows := commandTableRows(t)
	docs, problems := documentedCommands(rows, reg, ex)
	for _, p := range problems {
		t.Error(p)
	}
	analysis := analysisParameterCommands(t)
	for _, p := range commandTableProblems(reg, docs, analysis, ex, fex) {
		t.Error(p)
	}
	for _, p := range append(ex.Stale(), fex.Stale()...) {
		t.Error(p)
	}
}

// analysisParameterCommands are the commands the **Analysis parameter** paragraph of 13
// §4.2 gives --game-version.
func analysisParameterCommands(t *testing.T) map[string]bool {
	t.Helper()
	lines, err := speccheck.Section("13", "4.2")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, l := range lines {
		if strings.HasPrefix(l.Text, "**Analysis parameter**") {
			before, _, _ := strings.Cut(l.Text, " analyzes ")
			for _, s := range speccheck.Spans(before) {
				out[s] = true
			}
		}
	}
	if len(out) < 5 {
		t.Fatalf("13 §4.2: the **Analysis parameter** paragraph names only %v", out)
	}
	return out
}

// commandTableProblems: every command is documented (13 §4.2); every flag a usage span
// gives a command is one of its flags; every flag of a command appears in a row that
// documents it (--game-version may come from the Analysis parameter paragraph instead).
func commandTableProblems(reg []*Command, docs []docCommand, analysis map[string]bool, ex, fex *speccheck.Exceptions) []string {
	var out []string
	rowsOf := map[*Command][]speccheck.Row{}
	for _, d := range docs {
		for _, c := range d.cmds {
			rowsOf[c] = append(rowsOf[c], d.row)
			for _, f := range d.flags {
				if globalFlags[f] || slices.ContainsFunc(c.Flags, func(x Flag) bool { return x.Name == f }) || fex.Has(c.Name+" --"+f) {
					continue
				}
				out = append(out, fmt.Sprintf("%s: 13 §4.2: `%s` documents --%s, which `%s` does not take (cmds.allCommands())", d.row.Line.Pos(), d.span, f, c.Name))
			}
		}
	}
	for _, c := range reg {
		rows, ok := rowsOf[c]
		if !ok {
			if !ex.Has(c.Name) {
				out = append(out, fmt.Sprintf("13 §4.2: command `%s` (%s) is missing from the command table; add a row from its Summary and flags", c.Name, c.Summary))
			}
			continue
		}
		for _, f := range c.Flags {
			found := false
			for _, r := range rows {
				for _, cell := range r.Cells {
					if regexp.MustCompile(`--` + regexp.QuoteMeta(f.Name) + `\b`).MatchString(cell) {
						found = true
					}
				}
			}
			if !found && f.Name == "game-version" && analysis[strings.Fields(c.Name)[0]] {
				found = true
			}
			if !found && !fex.Has(c.Name+" --"+f.Name) {
				out = append(out, fmt.Sprintf("%s: 13 §4.2: `%s` takes --%s (%s), which its row does not document", rows[0].Line.Pos(), c.Name, f.Name, f.Help))
			}
		}
	}
	return out
}

// 13 §3.1 rule 1: the commands that print JSON are exactly the commands with a --json flag.
func TestSpecJSONCommands(t *testing.T) {
	t.Parallel()
	lines, err := speccheck.Section("13", "3.1")
	if err != nil {
		t.Fatal(err)
	}
	var rule speccheck.Line
	for _, l := range lines {
		if strings.HasPrefix(l.Text, "1. ") {
			rule = l
			break
		}
	}
	documented := map[string]bool{}
	for _, s := range speccheck.Spans(rule.Text) {
		if !strings.Contains(s, "--json") {
			continue
		}
		cs, ok := resolveSpan(s, allCommands())
		if !ok || len(cs) == 0 {
			t.Errorf("%s: 13 §3.1 rule 1: `%s` names no command", rule.Pos(), s)
		}
		for _, c := range cs {
			documented[c.Name] = true
		}
	}
	for _, c := range allCommands() {
		has := slices.ContainsFunc(c.Flags, func(f Flag) bool { return f.Name == "json" })
		if has && !documented[c.Name] {
			t.Errorf("%s: 13 §3.1 rule 1: `%s --json` prints JSON but the rule does not name it", rule.Pos(), c.Name)
		}
		if !has && documented[c.Name] {
			t.Errorf("%s: 13 §3.1 rule 1 names `%s --json`, but the command has no --json flag", rule.Pos(), c.Name)
		}
	}
}

// Settings keys documented somewhere other than the 13 §4.2 list, or documented values that
// are not the key's choices.
var settingExceptions = map[string]string{}

var (
	settingKeyRe = regexp.MustCompile(`^([a-z][a-z0-9_-]*(?:\.[a-z0-9_-]+)+)(?:\s+(.*))?$`)
	fileNameRe   = regexp.MustCompile(`\.(toml|json|xml|txt|md|db)$`)
)

// 13 §4.2 **Settings keys** documents every key of configKeys and freeKeys, and for a key
// with named choices exactly those choices (`key a|b|c`; a <placeholder> is free form).
func TestSpecSettingsKeys(t *testing.T) {
	t.Parallel()
	lines, err := speccheck.Section("13", "4.2")
	if err != nil {
		t.Fatal(err)
	}
	var para speccheck.Line
	for _, l := range lines {
		if strings.HasPrefix(l.Text, "**Settings keys**") {
			para = l
		}
	}
	if para.Text == "" {
		t.Fatal("13 §4.2: no **Settings keys** paragraph")
	}
	ex := speccheck.NewExceptions("settingExceptions (internal/cmds/spec_test.go)", settingExceptions)
	for _, p := range settingsProblems(para, configKeys, freeKeysNames(), ex) {
		t.Error(p)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

func freeKeysNames() map[string]bool {
	out := map[string]bool{}
	for k := range freeKeys {
		out[k] = true
	}
	return out
}

func settingsProblems(para speccheck.Line, choices map[string][]string, free map[string]bool, ex *speccheck.Exceptions) []string {
	var out []string
	documented := map[string]bool{}
	for _, s := range speccheck.Spans(para.Text) {
		m := settingKeyRe.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		key, values := m[1], strings.TrimSpace(m[2])
		if fileNameRe.MatchString(key) {
			continue // `config.toml`, a file
		}
		documented[key] = true
		code, isChoice := choices[key]
		if !isChoice && !free[key] {
			if !ex.Has(key) {
				out = append(out, fmt.Sprintf("%s: 13 §4.2 documents settings key %s, which configKeys and freeKeys (internal/cmds/config.go) do not have", para.Pos(), key))
			}
			continue
		}
		if !isChoice || values == "" || strings.HasPrefix(values, "<") {
			continue
		}
		doc := strings.Split(values, "|")
		a, b := slices.Clone(doc), slices.Clone(code)
		sort.Strings(a)
		sort.Strings(b)
		if !slices.Equal(a, b) && !ex.Has(key+" "+values) {
			out = append(out, fmt.Sprintf("%s: 13 §4.2 gives %s the values %s, but configKeys has %s", para.Pos(), key, strings.Join(doc, "|"), strings.Join(code, "|")))
		}
	}
	var keys []string
	for k := range choices {
		keys = append(keys, k)
	}
	for k := range free {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !documented[k] && !ex.Has(k) {
			out = append(out, fmt.Sprintf("%s: 13 §4.2: settings key %s (internal/cmds/config.go) is missing from **Settings keys**; add it with its values and the section that owns it", para.Pos(), k))
		}
	}
	return out
}

func TestSpecCLIDetection(t *testing.T) {
	t.Parallel()
	reg := []*Command{
		{Name: "identity", Flags: []Flag{{Name: "all"}}}, {Name: "identity list", Flags: []Flag{{Name: "all"}}},
		{Name: "problems"}, {Name: "problems show"}, {Name: "problems ignore"}, {Name: "rule add", Flags: []Flag{{Name: "note"}}},
		{Name: "def", Flags: []Flag{{Name: "game-version"}}}, {Name: "gone"},
	}
	for _, tc := range []struct {
		span string
		want []string
		ok   bool
	}{
		{"identity [list] [--all]", []string{"identity", "identity list"}, true},
		{"problems show|ignore <p:ref>", []string{"problems show", "problems ignore"}, true},
		{"rule add after|before <A> <B> [--note <text>]", []string{"rule add"}, true},
		{"archo problems --all", []string{"problems"}, true},
		{"--env <name>", nil, true},
		{"unplayed [--group x]", nil, false},
	} {
		cs, ok := resolveSpan(tc.span, reg)
		var names []string
		for _, c := range cs {
			names = append(names, c.Name)
		}
		if ok != tc.ok || !slices.Equal(names, tc.want) {
			t.Errorf("%q: got %v %v; want %v %v", tc.span, names, ok, tc.want, tc.ok)
		}
	}
	row := func(cells ...string) speccheck.Row {
		return speccheck.Row{Cells: cells, Line: speccheck.Line{Doc: "x.md", N: 1}}
	}
	rows := []speccheck.Row{row("`identity [list] [--all]` / `rule add top <A> [--nope]`", "x"), row("`def [<Type/defName>]`", "y"), row("`unplayed`", "z")}
	ex := speccheck.NewExceptions("x", map[string]string{"unplayed": "x"})
	docs, problems := documentedCommands(rows, reg, ex)
	if len(problems) != 0 {
		t.Errorf("%v", problems)
	}
	got := commandTableProblems(reg, docs, map[string]bool{"def": true}, ex, speccheck.NewExceptions("x", map[string]string{}))
	// rule add --nope unknown; rule add's --note undocumented; problems, problems show, problems ignore, gone missing
	if len(got) != 6 {
		t.Errorf("command table problems: %d\n%s", len(got), strings.Join(got, "\n"))
	}
	para := speccheck.Line{Text: "**Settings keys**: `ui.a on|off`; `ui.b <x>`; `ui.c x|y`; `gone.key x`; `config.toml`."}
	got = settingsProblems(para, map[string][]string{"ui.a": {"off", "on"}, "ui.c": {"x", "z"}, "ui.d": {"p"}}, map[string]bool{"ui.b": true}, speccheck.NewExceptions("x", map[string]string{}))
	if len(got) != 3 { // ui.c values, gone.key unknown, ui.d undocumented
		t.Errorf("settings problems: %v", got)
	}
}
