// Package cmdline shares interactive command parsing, completion and history between surfaces.
package cmdline

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type Candidate struct {
	Insert string `json:"insert"`
	Label  string `json:"label"`
	Note   string `json:"note"`
	Open   bool   `json:"open"`
}
type Mod struct{ ID, Ref, Name, Package string }
type Context struct {
	App      *app.App
	Mods     []Mod
	Current  *Mod
	Selected []Mod
}
type cmdCand struct {
	insert, label, note string
	open                bool
}
type cmdWord struct {
	text     string
	from, to int
}
type cmdLine struct {
	text []rune
	cur  int
}
type item struct{ id, ref, name, pkg string }
type column struct{ items []item }
type model struct {
	app      *app.App
	line     cmdLine
	cols     []column
	cur      *item
	selected []*item
}

func (m *model) current() *item     { return m.cur }
func (m *model) selection() []*item { return m.selected }
func toItem(x Mod) item             { return item{x.ID, x.Ref, x.Name, x.Package} }
func Complete(c Context, line string, cursor int) ([]Candidate, int, int) {
	r := []rune(line)
	m := model{app: c.App, line: cmdLine{r, max(0, min(cursor, len(r)))}, cols: []column{{}}}
	for _, x := range c.Mods {
		m.cols[0].items = append(m.cols[0].items, toItem(x))
	}
	if c.Current != nil {
		x := toItem(*c.Current)
		m.cur = &x
	}
	for _, x := range c.Selected {
		it := toItem(x)
		m.selected = append(m.selected, &it)
	}
	cs, from, to := m.cmdCands()
	out := make([]Candidate, 0, len(cs))
	for _, x := range cs {
		out = append(out, Candidate{x.insert, x.label, x.note, x.open})
	}
	return out, from, to
}
func Argv(line string) []string                    { return cmdArgv([]rune(line)) }
func Env(argv []string) (string, []string, string) { return cmdEnv(argv) }
func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, "\"") {
		return "\"" + s + "\""
	}
	return s
}

const HistoryLimit = 50

func DecodeHistory(raw string) []string {
	var h []string
	_ = json.Unmarshal([]byte(raw), &h)
	return h
}
func Remember(h []string, line string) []string {
	h = slices.DeleteFunc(h, func(s string) bool { return s == line })
	h = append(h, line)
	if len(h) > HistoryLimit {
		h = h[len(h)-HistoryLimit:]
	}
	return h
}

// Step preserves the draft when moving back from the newest history position.
func Step(h []string, at, delta int, text, draft string) (int, string, string) {
	n := at + delta
	if n < 0 || n > len(h) {
		return at, text, draft
	}
	if at == len(h) {
		draft = text
	}
	text = draft
	if n < len(h) {
		text = h[n]
	}
	return n, text, draft
}
func cmdWords(line []rune) []cmdWord {
	var out []cmdWord
	for i := 0; i < len(line); {
		if unicode.IsSpace(line[i]) {
			i++
			continue
		}
		start := i
		var b strings.Builder
		for i < len(line) && !unicode.IsSpace(line[i]) {
			if q := line[i]; q == '"' || q == '\'' {
				for i++; i < len(line) && line[i] != q; i++ {
					b.WriteRune(line[i])
				}
				if i < len(line) {
					i++
				}
				continue
			}
			b.WriteRune(line[i])
			i++
		}
		out = append(out, cmdWord{b.String(), start, i})
	}
	return out
}

// cmdArgv is the line's arguments without a leading "archo".
func cmdArgv(line []rune) []string {
	var argv []string
	for _, w := range cmdWords(line) {
		argv = append(argv, w.text)
	}
	if len(argv) > 0 && argv[0] == "archo" {
		argv = argv[1:]
	}
	return argv
}

// cmdEnv takes --env <name> / --env=<name> out of argv: like the CLI's global flag it
// applies to this command only.
func cmdEnv(argv []string) (env string, rest []string, err string) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return env, append(rest, argv[i:]...), ""
		case a == "--env":
			if i+1 >= len(argv) {
				return "", nil, "--env needs a name"
			}
			env = argv[i+1]
			i++
		case strings.HasPrefix(a, "--env="):
			env = strings.TrimPrefix(a, "--env=")
		default:
			rest = append(rest, a)
		}
	}
	return env, rest, ""
}

// cmdCands are the completions of the word under the cursor and its rune span.
func (m *model) cmdCands() ([]cmdCand, int, int) {
	l := m.line
	words := cmdWords(l.text)
	from, to, word := l.cur, l.cur, ""
	var prior []string
	for _, w := range words {
		if w.from > l.cur {
			break // the words after the cursor do not matter
		}
		if w.to < l.cur {
			prior = append(prior, w.text)
		} else {
			from, to, word = w.from, w.to, w.text
		}
	}
	if len(prior) > 0 && prior[0] == "archo" {
		prior = prior[1:]
	}
	var out []cmdCand
	c, rest := cmds.Find(prior)
	if c != nil {
		out = append(out, m.argCands(c, rest, word)...)
	}
	out = append(out, nameCands(prior, word)...)
	return out, from, to
}

// nameCands: the commands that go on from the words before (and, at the first word, those
// with the word anywhere in their name).
func nameCands(prior []string, word string) []cmdCand {
	var pre, sub []cmdCand
	lw := strings.ToLower(word)
	for _, c := range cmds.Commands() {
		parts := strings.Fields(c.Name)
		if len(parts) <= len(prior) || !slices.Equal(parts[:len(prior)], prior) {
			continue
		}
		next := parts[len(prior)]
		cand := cmdCand{insert: strings.Join(parts[len(prior):], " "), label: strings.TrimSpace(c.Name + " " + c.Args), note: c.Summary}
		switch {
		case strings.HasPrefix(next, lw):
			pre = append(pre, cand)
		case lw != "" && len(prior) == 0 && strings.Contains(c.Name, lw):
			sub = append(sub, cand)
		}
	}
	return append(pre, sub...)
}

// argCands: the flags of c (a word starting with -) or the values of the positional
// argument the word is, from the placeholder in c's usage (16 §9.4).
func (m *model) argCands(c *cmds.Command, rest []string, word string) []cmdCand {
	if strings.HasPrefix(word, "-") {
		var out []cmdCand
		for _, f := range append(slices.Clone(c.Flags), cmds.Flag{Name: "env", Value: "<name>", Help: "use another game for this command"}) {
			if !strings.HasPrefix("--"+f.Name, word) {
				continue
			}
			label := "--" + f.Name
			if f.Value != "" {
				label += " " + f.Value
			}
			out = append(out, cmdCand{insert: "--" + f.Name, label: label, note: f.Help})
		}
		return out
	}
	// the positional arguments before the word; a value flag takes the next word
	n := 0
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "--") || a == "--" {
			n++
			continue
		}
		if strings.Contains(a, "=") {
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if name == "env" {
			if i+1 == len(rest) {
				return m.envCands(word) // the word is --env's value
			}
			i++
			continue
		}
		for _, f := range c.Flags {
			if f.Name == name && f.Value != "" {
				if i+1 == len(rest) {
					return nil // the word is this flag's value: not completed
				}
				i++
			}
		}
	}
	tok := argToken(c.Args, n)
	switch {
	case tok == "":
		return nil
	case !strings.Contains(tok, "<") && strings.Contains(tok, "|"):
		return literalCands(strings.Split(tok, "|"), word)
	case isModToken(tok):
		return m.modCands(word, strings.Contains(tok, "…") || strings.Contains(tok, "..."))
	}
	switch tok {
	case "<key>":
		if strings.HasPrefix(c.Name, "config") {
			return m.settingCands(word)
		}
	case "<value>":
		if c.Name == "config set" && n == 1 {
			var out []cmdCand
			for i, v := range cmds.SettingChoices(positional(rest, c)[0]) {
				if strings.HasPrefix(v, word) {
					cand := cmdCand{insert: v, label: v}
					if i == 0 {
						cand.note = view.T("default")
					}
					out = append(out, cand)
				}
			}
			return out
		}
	case "<kind>":
		if c.Name == "cache clear" {
			return literalCands([]string{"previews", "workshop", "images", "source", "logs"}, word)
		}
	case "<name>", "<old>", "<plan>", "<name|file>":
		switch {
		case c.Name == "env add", c.Name == "plan new":
			return nil // a new name
		case strings.HasPrefix(c.Name, "env "):
			return m.envCands(word)
		case strings.HasPrefix(c.Name, "plan "), c.Name == "test run":
			return m.planCands(word)
		case strings.HasPrefix(c.Name, "pack "):
			return m.packCands(word)
		case strings.HasPrefix(c.Name, "deps "):
			return literalCands([]string{"todds", "steamcmd"}, word)
		}
	}
	return nil
}

// literalCands: the values that start with the word.
func literalCands(values []string, word string) []cmdCand {
	var out []cmdCand
	for _, v := range values {
		if strings.HasPrefix(v, word) {
			out = append(out, cmdCand{insert: v, label: v})
		}
	}
	return out
}

// positional is rest without flags and their values.
func positional(rest []string, c *cmds.Command) []string {
	var out []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
			continue
		}
		if strings.Contains(a, "=") {
			continue
		}
		for _, f := range c.Flags {
			if f.Name == strings.TrimPrefix(a, "--") && f.Value != "" {
				i++
			}
		}
	}
	return out
}

// argToken is the usage placeholder of the n-th positional argument: the first form of
// a usage with alternatives ("a | b"), brackets removed; a repeating last one (<ref…>)
// covers every later position.
func argToken(usage string, n int) string {
	form, _, _ := strings.Cut(usage, " | ")
	toks := strings.Fields(strings.NewReplacer("[", "", "]", "").Replace(form))
	if len(toks) == 0 {
		return ""
	}
	if n < len(toks) {
		return toks[n]
	}
	if last := toks[len(toks)-1]; strings.Contains(last, "…") || strings.Contains(last, "...") {
		return last
	}
	return ""
}

// isModToken: a placeholder for a mod reference (<ref>, <ref…>, <mod>, <A>, <B>), not
// <p:ref>, <src:label> and the like.
func isModToken(tok string) bool {
	if strings.Contains(tok, ":") {
		return false
	}
	return strings.Contains(tok, "ref") || strings.Contains(tok, "<mod>") || tok == "<A>" || tok == "<B>"
}

// modCands: the mods of the current game whose name, package id or reference matches the
// word, inserted as a reference without # (13 §3.1 rule 11). An empty word starts with the
// mod under the cursor and, for a repeating argument, the selection.
func (m *model) modCands(word string, many bool) []cmdCand {
	lw := strings.ToLower(strings.TrimPrefix(word, "#"))
	ref := func(it *item) string { return strings.TrimPrefix(it.ref, "#") }
	cand := func(it *item) cmdCand {
		note := ref(it)
		if it.pkg != "" {
			note += "  " + it.pkg
		}
		return cmdCand{insert: ref(it), label: it.name, note: note}
	}
	var out []cmdCand
	seen := map[string]bool{}
	if lw == "" {
		if sel := m.selection(); many && len(sel) > 1 {
			var refs []string
			for _, it := range sel {
				refs = append(refs, ref(it))
			}
			out = append(out, cmdCand{insert: strings.Join(refs, " "), label: view.Tf("the %d selected mods", len(sel))})
		}
		if it := m.current(); it != nil && it.id != "" {
			out = append(out, cand(it))
			seen[it.id] = true
		}
	}
	var pre, sub, other []cmdCand
	for k := range m.cols {
		for i := range m.cols[k].items {
			it := &m.cols[k].items[i]
			if it.id == "" || seen[it.id] {
				continue
			}
			seen[it.id] = true
			name := strings.ToLower(it.name)
			switch {
			case strings.HasPrefix(name, lw):
				pre = append(pre, cand(it))
			case strings.Contains(name, lw):
				sub = append(sub, cand(it))
			case strings.Contains(strings.ToLower(it.pkg), lw) || strings.HasPrefix(ref(it), lw):
				other = append(other, cand(it))
			}
		}
	}
	return slices.Concat(out, pre, sub, other)
}

func (m *model) envCands(word string) []cmdCand {
	envs, err := m.app.Store.Envs()
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, e := range envs {
		if strings.HasPrefix(strings.ToLower(e.Name), strings.ToLower(word)) {
			out = append(out, cmdCand{insert: quoteArg(e.Name), label: e.Name})
		}
	}
	return out
}

func (m *model) planCands(word string) []cmdCand {
	rows, err := view.PlanRows(m.app)
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, r := range rows {
		if strings.HasPrefix(strings.ToLower(r.Name), strings.ToLower(strings.Trim(word, "\"'"))) {
			out = append(out, cmdCand{insert: quoteArg(r.Name), label: r.Name})
		}
	}
	return out
}

func (m *model) packCands(word string) []cmdCand {
	rows, _, err := view.PackRows(m.app)
	if err != nil {
		return nil
	}
	var out []cmdCand
	for _, r := range rows {
		if strings.HasPrefix(strings.ToLower(r.Name), strings.ToLower(strings.Trim(word, "\"'"))) {
			out = append(out, cmdCand{insert: quoteArg(r.Name), label: r.Name})
		}
	}
	return out
}

func (m *model) settingCands(word string) []cmdCand {
	var out []cmdCand
	for _, k := range cmds.SettingKeys() {
		if strings.HasPrefix(k, word) {
			out = append(out, cmdCand{insert: k, label: k, note: cmds.Setting(m.app, k)})
		}
	}
	return out
}
