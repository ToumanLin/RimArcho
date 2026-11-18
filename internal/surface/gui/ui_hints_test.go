package gui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 16 §2.2 and §3.1 rule 4: a hint line lists at most 7 actions, ending with ? all keys.
// TestUIHintLines follows every hint provider renderHints (main.js) dispatches and adds up
// each way through it: a return ends a way, an if without a return may add its items, a
// loop needs a written limit (out.length < N). ctrl+o and ctrl+w fold into one group
// (foldShortcuts). renderHints itself keeps ? when it trims (hintline.js, testdata/hints.mjs).

const hintMax = 7

// Ways through a provider that may break the rule, by "file function: first key", or a
// whole provider by "file function".
var hintAllowed = map[string]string{
	"js/planmanage.js hintList: enter": "the new-list form: its name field has the focus and takes ? as text (16 §2.17)",
	"js/tasks.js hintList: ←→↑↓":       "the open filter bar: its value field has the focus and takes ? as text (16 §2.16)",
	"js/subscriptions.js addHints":     "the address field has the focus and takes ? as text (16 §2.9)",
}

// Pages whose list renderHints extends: the updates page of one Mod puts esc first.
var hintPrefix = map[string]int{"js/subscriptions.js changesHints": 1}

type hintWay struct {
	n            int
	first        string
	last         string
	open         bool // a loop or spread without a written limit
	ctrlO, ctrlW bool
}

func (w hintWay) count() int {
	if w.ctrlO && w.ctrlW {
		return w.n - 1
	}
	return w.n
}

type hintItem struct {
	key  string
	way  *hintWay // a spread of a known list
	open bool
}

type hintScope struct {
	lists map[string]hintWay
	objs  map[string]string
}

func (s hintScope) clone() hintScope {
	c := hintScope{lists: map[string]hintWay{}, objs: map[string]string{}}
	for k, v := range s.lists {
		c.lists[k] = v
	}
	for k, v := range s.objs {
		c.objs[k] = v
	}
	return c
}

var (
	hintKey      = regexp.MustCompile(`^\{\s*key:\s*("(?:[^"\\]|\\.)*")`)
	hintIdent    = regexp.MustCompile(`^[A-Za-z_]\w*$`)
	hintDecl     = regexp.MustCompile(`^(?:const|let|var)\s+(\w+)\s*=\s*`)
	hintPush     = regexp.MustCompile(`^(\w+)\.push\(`)
	hintLimit    = regexp.MustCompile(`\.length\s*<\s*(\d+)`)
	hintCtl      = regexp.MustCompile(`^(if|for|while)\b`)
	hintRenderTo = regexp.MustCompile(`\blist\s*=\s*(?:([\w]+)\.)?(\w+)\(\)|\blist\s*=\s*\[`)
)

func (s hintScope) items(elems []string) []hintItem {
	var out []hintItem
	for _, e := range elems {
		e = strings.TrimSpace(e)
		switch {
		case e == "":
		case strings.HasPrefix(e, "{"):
			key := "(computed)"
			if m := hintKey.FindStringSubmatch(e); m != nil {
				key, _ = strconv.Unquote(m[1])
			}
			out = append(out, hintItem{key: key})
		case strings.HasPrefix(e, "..."):
			name := strings.TrimSpace(e[3:])
			if w, ok := s.lists[name]; ok {
				out = append(out, hintItem{way: &w})
				break
			}
			// ...(cond ? [a, b] : []): the longer list, its end unknown
			best := hintWay{last: "(conditional)"}
			found := false
			for i := 0; i < len(name); i++ {
				if name[i] == '[' {
					end := jsMatch(name, i)
					w := s.way(s.items(jsSplitTop(name[i+1:end], ',')))
					if w.n > best.n {
						best.n, best.ctrlO, best.ctrlW = w.n, w.ctrlO, w.ctrlW
					}
					found = true
					i = end
				}
			}
			if !found {
				out = append(out, hintItem{open: true})
				break
			}
			out = append(out, hintItem{way: &best})
		case hintIdent.MatchString(e):
			if k, ok := s.objs[e]; ok {
				out = append(out, hintItem{key: k})
			} else {
				out = append(out, hintItem{key: "(" + e + ")"})
			}
		default:
			out = append(out, hintItem{key: "(computed)"})
		}
	}
	return out
}

func (s hintScope) way(items []hintItem) hintWay {
	var w hintWay
	return w.add(items)
}

func (w hintWay) add(items []hintItem) hintWay {
	for _, it := range items {
		switch {
		case it.open:
			w.open = true
		case it.way != nil:
			if w.n == 0 {
				w.first = it.way.first
			}
			w.n += it.way.n
			w.open = w.open || it.way.open
			w.ctrlO, w.ctrlW = w.ctrlO || it.way.ctrlO, w.ctrlW || it.way.ctrlW
			if it.way.n > 0 {
				w.last = it.way.last
			}
		default:
			if w.n == 0 {
				w.first = it.key
			}
			w.n++
			w.last = it.key
			w.ctrlO = w.ctrlO || it.key == "ctrl+o"
			w.ctrlW = w.ctrlW || it.key == "ctrl+w"
		}
	}
	return w
}

// run adds up the statements; it returns the ways that ended and whether every way did.
func (s hintScope) run(statements []string) (ended []hintWay, returned bool) {
	for _, st := range statements {
		switch m := hintCtl.FindStringSubmatch(st); {
		case m != nil && m[1] == "if":
			branches := hintBranches(st)
			var open []hintScope
			all := true
			for _, b := range branches {
				c := s.clone()
				e, r := c.run(jsStatements(b))
				ended = append(ended, e...)
				if !r {
					open = append(open, c)
					all = false
				}
			}
			if len(branches) == 1 {
				open = append(open, s.clone())
				all = false
			}
			if all {
				return ended, true
			}
			for name, before := range s.lists {
				merged := before
				same := true
				for _, o := range open {
					w := o.lists[name]
					if w.n > merged.n {
						merged.n = w.n
					}
					merged.open = merged.open || w.open
					merged.ctrlO, merged.ctrlW = merged.ctrlO || w.ctrlO, merged.ctrlW || w.ctrlW
					if w.last != open[0].lists[name].last {
						same = false
					}
					if merged.first == "" {
						merged.first = w.first
					}
				}
				if same {
					merged.last = open[0].lists[name].last
				} else {
					merged.last = "(conditional)"
				}
				s.lists[name] = merged
			}
		case m != nil:
			for name, w := range s.lists {
				if !strings.Contains(st, name+".push(") {
					continue
				}
				if l := hintLimit.FindStringSubmatch(st); l != nil {
					n, _ := strconv.Atoi(l[1])
					if n > w.n {
						w.n = n
					}
				} else {
					w.open = true
				}
				w.last = "(conditional)"
				s.lists[name] = w
			}
		case strings.HasPrefix(st, "return"):
			e := strings.TrimSpace(strings.TrimPrefix(st, "return"))
			e = strings.TrimSuffix(e, ";")
			switch {
			case strings.HasPrefix(e, "["):
				end := jsMatch(e, 0)
				ended = append(ended, s.way(s.items(jsSplitTop(e[1:end], ','))))
			case hintIdent.MatchString(e):
				if w, ok := s.lists[e]; ok {
					ended = append(ended, w)
				}
			}
			return ended, true
		default:
			if d := hintDecl.FindStringSubmatch(st); d != nil {
				rest := strings.TrimSpace(st[len(d[0]):])
				switch {
				case strings.HasPrefix(rest, "["):
					end := jsMatch(rest, 0)
					s.lists[d[1]] = s.way(s.items(jsSplitTop(rest[1:end], ',')))
				case strings.HasPrefix(rest, "{") && hintKey.MatchString(rest):
					k, _ := strconv.Unquote(hintKey.FindStringSubmatch(rest)[1])
					s.objs[d[1]] = k
				}
				continue
			}
			if p := hintPush.FindStringSubmatchIndex(st); p != nil {
				name := st[p[2]:p[3]]
				s.lists[name] = s.lists[name].add(s.items(jsCallArgs(st, p[1]-1)))
			}
		}
	}
	return ended, false
}

// hintBranches returns the statements of each branch of an if / else if / else chain.
func hintBranches(st string) []string {
	var out []string
	for {
		st = strings.TrimSpace(strings.TrimPrefix(st, "if"))
		open := strings.IndexByte(st, '(')
		st = strings.TrimSpace(st[jsMatch(st, open)+1:])
		var branch string
		if strings.HasPrefix(st, "{") {
			end := jsMatch(st, 0)
			branch, st = st[1:end], strings.TrimSpace(st[end+1:])
		} else {
			parts := jsStatements(st)
			branch = parts[0]
			st = strings.TrimSpace(strings.Join(parts[1:], ";\n"))
		}
		out = append(out, branch)
		if !strings.HasPrefix(st, "else") {
			return out
		}
		st = strings.TrimSpace(st[4:])
		if !strings.HasPrefix(st, "if") || !hintCtl.MatchString(st) {
			if strings.HasPrefix(st, "{") {
				st = st[1:jsMatch(st, 0)]
			}
			return append(out, st)
		}
	}
}

// hintProviders resolves what renderHints dispatches to: "file function" → body.
func hintProviders(t *testing.T, scripts map[string]string) map[string]string {
	t.Helper()
	main := jsBlank(scripts["js/main.js"])
	render, ok := jsFunction(main, "renderHints")
	if !ok {
		t.Fatal("main.js has no renderHints")
	}
	out := map[string]string{}
	for _, m := range hintRenderTo.FindAllStringSubmatchIndex(render, -1) {
		if m[4] < 0 {
			at := strings.LastIndexByte(render[:m[1]], '[')
			if strings.Contains(render[at:jsMatch(render, at)], "...list") {
				continue // a prefix to a provider's list (hintPrefix)
			}
			out["js/main.js renderHints"] = "return " + render[at:jsMatch(render, at)+1]
			continue
		}
		ns, name := "", render[m[4]:m[5]]
		if m[2] >= 0 {
			ns = render[m[2]:m[3]]
		}
		file, fn := hintResolve(t, scripts, "js/main.js", ns, name)
		body, ok := jsFunction(jsBlank(scripts[file]), fn)
		if !ok {
			t.Fatalf("%s: no function %s for renderHints", file, fn)
		}
		out[file+" "+fn] = body
	}
	if len(out) < 10 {
		t.Fatalf("found only %d hint providers in renderHints", len(out))
	}
	return out
}

// hintResolve finds where an imported name is defined, following re-exports.
func hintResolve(t *testing.T, scripts map[string]string, file, ns, name string) (string, string) {
	t.Helper()
	src := scripts[file]
	if ns != "" {
		m := regexp.MustCompile(`import \* as ` + ns + ` from "\./([\w-]+\.js)"`).FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("%s: no namespace import %s", file, ns)
		}
		return hintResolve(t, scripts, "js/"+m[1], "", name)
	}
	if _, ok := jsFunction(jsBlank(src), name); ok {
		return file, name
	}
	for _, m := range regexp.MustCompile(`(?:import|export) \{([^}]*)\} from "\./([\w-]+\.js)"`).FindAllStringSubmatch(src, -1) {
		for _, part := range strings.Split(m[1], ",") {
			f := strings.Fields(part)
			if len(f) == 3 && f[1] == "as" && f[2] == name {
				return hintResolve(t, scripts, "js/"+m[2], "", f[0])
			}
			if len(f) == 1 && f[0] == name {
				return hintResolve(t, scripts, "js/"+m[2], "", name)
			}
		}
	}
	t.Fatalf("%s: cannot find %s", file, name)
	return "", ""
}

func hintProblems(provider, body string, used map[string]bool) []string {
	s := hintScope{lists: map[string]hintWay{}, objs: map[string]string{}}
	ways, _ := s.run(jsStatements(body))
	if len(ways) == 0 {
		return []string{provider + ": no hint list found; TestUIHintLines cannot read it"}
	}
	if _, ok := hintAllowed[provider]; ok {
		used[provider] = true
		return nil
	}
	var problems []string
	for _, w := range ways {
		id := provider + ": " + w.first
		if _, ok := hintAllowed[id]; ok {
			used[id] = true
			continue
		}
		n := w.count() + hintPrefix[provider]
		switch {
		case w.open:
			problems = append(problems, fmt.Sprintf("16 §3.1 rule 4: hint provider %s (from %q) adds actions in a loop or spread without a limit; at most %d, ending with ? all keys", provider, w.first, hintMax))
		case n > hintMax:
			problems = append(problems, fmt.Sprintf("16 §3.1 rule 4: hint provider %s (from %q) lists up to %d actions; at most %d, ending with ? all keys", provider, w.first, n, hintMax))
		}
		if w.last != "?" {
			problems = append(problems, fmt.Sprintf("16 §2.2: hint provider %s (from %q) ends with %q; the last action is ? all keys", provider, w.first, w.last))
		}
	}
	return problems
}

func TestUIHintLines(t *testing.T) {
	t.Parallel()
	scripts := pageScripts(t)
	used := map[string]bool{}
	for provider, body := range hintProviders(t, scripts) {
		for _, p := range hintProblems(provider, body, used) {
			t.Error(p)
		}
	}
	for k := range hintAllowed {
		if !used[k] {
			t.Errorf("%s: listed in hintAllowed but nothing needs it any more; remove it", k)
		}
	}
	if !strings.Contains(scripts["js/main.js"], "hintLine(") {
		t.Error("16 §3.1 rule 4: renderHints must trim through hintLine (hintline.js), which keeps ? all keys")
	}
}

// These examples guard the hint reader itself.
func TestUIHintLinesDetection(t *testing.T) {
	t.Parallel()
	q := `{ key: "?", label: T("all keys") }`
	for _, tc := range []struct {
		body string
		want int
	}{
		{`return [{ key: "a" }, ` + q + `]`, 0},
		{`const out = [{ key: "a" }]; if (x) out.push({ key: "b" }); out.push(` + q + `); return out;`, 0},
		{`if (x) return [{ key: "a" }]; return [` + q + `]`, 1},
		{`const out = []; for (const a of acts) out.push({ key: a.key }); out.push(` + q + `); return out;`, 1},
		{`const out = []; for (const a of acts) if (out.length < 6) out.push({ key: a.key }); out.push(` + q + `); return out;`, 0},
		{`const out = [{ key: "1" }, { key: "2" }, { key: "3" }, { key: "4" }]; if (a) { out.push({ key: "5" }, { key: "6" }); } else out.push({ key: "7" }); out.push({ key: "8" }, ` + q + `); return out;`, 1},
		{`const out = [{ key: "1" }, { key: "2" }, { key: "3" }, { key: "4" }, { key: "5" }]; out.push({ key: "ctrl+o" }, { key: "ctrl+w" }, ` + q + `); return out;`, 0},
		{`const tail = [{ key: "p" }, ` + q + `]; if (a) { return [{ key: "x" }, ...tail]; } return [{ key: "y" }, ...(b ? [{ key: "z" }] : []), ...tail];`, 0},
		{`const list = []; if (g) { list.push({ key: "a" }); return list; } list.push({ key: "b" }); if (c) list.push(` + q + `); return list;`, 2},
	} {
		got := hintProblems("js/x.js f", tc.body, map[string]bool{})
		if len(got) != tc.want {
			t.Errorf("%s: got %v; want %d failures", tc.body, got, tc.want)
		}
	}
}
