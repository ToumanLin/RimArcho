//go:build windows || cgo

package gui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Contracts between the page scripts, the Go bridge and the specification (16 §3, §7, §8)
// that reading the source can check. Like ui_rules_test.go, each failure names its section,
// each exception has its reason, and an exception nothing needs any more fails. Run them:
//
//	go test -count=1 -run 'TestUI' ./internal/surface/gui/

// ---- 16 §3.5: every command panel command works in the window ----

var (
	cmdPages = regexp.MustCompile(`const pages = \{([^}]*)\}`)
	cmdCase  = regexp.MustCompile(`case "([\w-]+)":`)
	jsKeyRe  = regexp.MustCompile(`(?:^|[{,])\s*(?:"([\w-]+)"|([\w$]+))\s*:`)
)

func commandProblems(main string, ids []string) []string {
	body, ok := jsFunction(jsBlank(main), "cmd")
	if !ok {
		return []string{"js/main.js: no cmd() for the command panel (16 §3.5)"}
	}
	handled := map[string]bool{}
	if m := cmdPages.FindStringSubmatch(body); m != nil {
		for _, k := range jsKeyRe.FindAllStringSubmatch(m[1], -1) {
			handled[k[1]+k[2]] = true
		}
	}
	for _, m := range cmdCase.FindAllStringSubmatch(body, -1) {
		handled[m[1]] = true
	}
	known := map[string]bool{}
	var problems []string
	for _, id := range ids {
		known[id] = true
		if !handled[id] {
			problems = append(problems, fmt.Sprintf("16 §3.5: command %q (view.Commands) does nothing in the window; handle it in main.js cmd() (the pages map or a case)", id))
		}
	}
	for id := range handled {
		if !known[id] {
			problems = append(problems, fmt.Sprintf("16 §3.5: main.js cmd() handles %q, which is no command in view.Commands; the command panel never sends it", id))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestUICommandsWork(t *testing.T) {
	t.Parallel()
	var ids []string
	for _, c := range view.Commands {
		ids = append(ids, c.ID)
	}
	for _, p := range commandProblems(pageScripts(t)["js/main.js"], ids) {
		t.Error(p)
	}
}

// ---- 16 §8.2: the page calls only methods the Go service has ----

// Bridge methods no page script calls. Each is kept for a reason or waits for a decision.
var bridgeUncalled = map[string]string{
	"Close":                      "gui.go closes the page reader when the window ends (defer api.Close()); spikes/guidev too",
	"Clipboard":                  "unused since 2026-11-20: the Import page's paste field moved to All lists' new / import; kept while the page's layout is under review (16 §2.18)",
	"PickAnyImport":              "unused since 2026-11-20: the Import page's list file moved to All lists' new / import; kept while the page's layout is under review (16 §2.18)",
	"ListProblems":               "unused; open question: 16 §8.7 says the editor reads its findings with it, but the page uses Board.problems and the list-problems event",
	"AccountStatus":              "unused; open question",
	"Changes":                    "unused; open question (the updates page reads SubscriptionsPage)",
	"Board":                      "unused; open question (the page gets the board from Workspace and edit results)",
	"History":                    "unused; open question",
	"Logs":                       "unused; open question",
	"ModCandidates":              "unused; open question",
	"NewPlan":                    "unused; open question (All lists creates with CreatePlan)",
	"PlanFroms":                  "unused; open question",
	"ImportPreview":              "unused; open question",
	"PackImport":                 "unused since the first-run guide hands over to Import (16 §2.15), which adds packs with StartPackImport; subscriptions_test still calls it",
	"ImportApply":                "unused; open question",
	"ExportSave":                 "unused; open question",
	"ExportCopy":                 "unused; open question",
	"DiscoverManagerImports":     "unused; open question",
	"PreviewManagerImport":       "unused; open question",
	"ApplyManagerImport":         "unused; open question (api.js still lists it in writesGame)",
	"ApplyManagerImportProfiles": "unused; open question (api.js still lists it in writesGame)",
	"LookStyleAdd":               "looks view, add row follows",
	"LookStyleAddPreview":        "looks view, add row follows",
	"LookStyleRemove":            "looks view, add row follows",
	"LookPreview":                "one prediction on its own: the looks editor shows the ones LookDetail carries per target (16 §2.5 Looks view)",
}

// Go text that reads the same in every language.
var goTextFixed = map[string]string{
	"Core":         "the game's own name for its content, as RimWorld shows it",
	"v%d":          "a list version number",
	"v%d  %s  %s":  "a list version number with its date and note",
	"todds %s  %s": "the converter's name and its arguments",
	"u %s":         "a key and its label",
}

var (
	jsAPICall  = regexp.MustCompile(`\bapi\s*\.\s*([A-Z]\w*)\s*\(`)
	jsWrites   = regexp.MustCompile(`(?s)const writesGame = new Set\(\[(.*?)\]\);`)
	jsEnvCalls = regexp.MustCompile(`if \(!\[([^\]]*)\]\.includes\(method\)\)`)
	jsQuoted   = regexp.MustCompile(`"(\w+)"`)
)

func TestUIBridgeMethods(t *testing.T) {
	t.Parallel()
	methods := map[string]bool{}
	typ := reflect.TypeOf(&API{})
	for i := 0; i < typ.NumMethod(); i++ {
		methods[typ.Method(i).Name] = true
	}
	scripts := pageScripts(t)
	called := map[string]bool{}
	for name, src := range scripts {
		for _, m := range jsAPICall.FindAllStringSubmatchIndex(jsBlank(src), -1) {
			method := src[m[2]:m[3]]
			called[method] = true
			if !methods[method] {
				t.Errorf("assets/%s:%d: api.%s: 16 §8.2: the Go service (gui.API) has no method %s", name, 1+strings.Count(src[:m[0]], "\n"), method, method)
			}
		}
	}
	apiJS := scripts["js/api.js"]
	for _, list := range []*regexp.Regexp{jsWrites, jsEnvCalls} {
		m := list.FindStringSubmatch(jsBlank(apiJS))
		if m == nil {
			t.Fatalf("api.js: %s not found", list)
		}
		seen := map[string]bool{}
		for _, q := range jsQuoted.FindAllStringSubmatch(m[1], -1) {
			if !methods[q[1]] {
				t.Errorf("assets/js/api.js: %q: 16 §8.2: the Go service (gui.API) has no such method", q[1])
			}
			if seen[q[1]] {
				t.Errorf("assets/js/api.js: %q is listed twice", q[1])
			}
			seen[q[1]] = true
		}
	}
	for name := range methods {
		_, listed := bridgeUncalled[name]
		switch {
		case called[name] && listed:
			t.Errorf("%s: listed in bridgeUncalled but a page script calls it; remove it", name)
		case !called[name] && !listed:
			t.Errorf("gui.API.%s: 16 §8.2: no page script calls it; call it, remove it, or list it in bridgeUncalled with a reason", name)
		}
	}
	for name := range bridgeUncalled {
		if !methods[name] {
			t.Errorf("%s: listed in bridgeUncalled but gui.API has no such method; remove it", name)
		}
	}
}

// ---- 16 §7: Go text the interface shows has its translation ----

// goTextDirs hold the Go code whose T / Tf text reaches the window or the terminal.
var goTextDirs = []string{"../../view", "../tui", "."}

// goTextCalls lists the literal first arguments of T, Tf, view.T and view.Tf calls.
type goTextCall struct {
	pos    string
	text   string
	fn     string
	args   int
	spread bool
}

func goTextCalls(t *testing.T) []goTextCall {
	t.Helper()
	var out []goTextCall
	for _, dir := range goTextDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				var name string
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					if x, ok := fn.X.(*ast.Ident); ok && x.Name == "view" {
						name = fn.Sel.Name
					}
				}
				if name != "T" && name != "Tf" {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, goTextCall{pos: fmt.Sprint(fset.Position(lit.Pos())), text: s, fn: name, args: len(call.Args) - 1, spread: call.Ellipsis.IsValid()})
				return true
			})
		}
	}
	if len(out) < 100 {
		t.Fatalf("found only %d T / Tf calls in Go", len(out))
	}
	return out
}

var goVerb = regexp.MustCompile(`%(?:\[(\d+)\])?[-+# 0]*\d*(?:\.\d+)?[a-zA-Z%]`)
var hasLetter = regexp.MustCompile(`\pL`)

func TestUIGoTextTranslated(t *testing.T) {
	t.Parallel()
	zh := l10ndata.Locales["zh"]()
	var missing []string
	seen, used := map[string]bool{}, map[string]bool{}
	for _, c := range goTextCalls(t) {
		if !hasLetter.MatchString(goVerb.ReplaceAllString(c.text, "")) || seen[c.text] {
			continue // a pure format, such as "%s  %s"
		}
		seen[c.text] = true
		if _, ok := goTextFixed[c.text]; ok {
			used[c.text] = true
			continue
		}
		if _, ok := zh.Text[c.text]; ok {
			continue
		}
		if _, ok := zh.ExtraText[c.text]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%s: %q", c.pos, c.text))
	}
	for k := range goTextFixed {
		if !used[k] {
			t.Errorf("%q: listed in goTextFixed but no Go code translates it any more; remove it", k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("16 §7: %d Go interface strings have no Chinese text (add zh and ja, l10ndata/locale):\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

// ---- 16 §7: a Tf format takes as many values as it has verbs ----

var jsVerb = regexp.MustCompile(`%(?:\[(\d+)\])?[sdv]`)

// verbsWanted is the number of values a format takes: its highest %[n], or its verbs.
func verbsWanted(format string, re *regexp.Regexp) int {
	n, indexed := 0, 0
	for _, m := range re.FindAllStringSubmatch(format, -1) {
		if m[0] == "%%" {
			continue
		}
		n++
		if m[1] != "" {
			i, _ := strconv.Atoi(m[1])
			indexed = max(indexed, i)
		}
	}
	if indexed > 0 {
		return indexed
	}
	return n
}

var jsTfCall = regexp.MustCompile(`\bTf\(`)

func jsFormatProblems(name, src string) []string {
	blank := jsBlank(src)
	var problems []string
	for _, m := range jsTfCall.FindAllStringIndex(blank, -1) {
		if m[0] >= 9 && strings.HasSuffix(blank[:m[0]], "function ") {
			continue
		}
		args := jsCallArgs(blank, m[1]-1)
		if len(args) == 0 || !strings.HasPrefix(args[0], `"`) || strings.Contains(strings.Join(args, ","), "...") {
			continue
		}
		f, err := strconv.Unquote(args[0])
		if err != nil {
			continue // a concatenation; not checked
		}
		if want := verbsWanted(f, jsVerb); want != len(args)-1 {
			problems = append(problems, fmt.Sprintf("assets/%s:%d: Tf(%q) gets %d values for %d placeholders (16 §7)", name, 1+strings.Count(src[:m[0]], "\n"), f, len(args)-1, want))
		}
	}
	return problems
}

func TestUITfArguments(t *testing.T) {
	t.Parallel()
	for name, src := range pageScripts(t) {
		for _, p := range jsFormatProblems(name, src) {
			t.Error(p)
		}
	}
	for _, c := range goTextCalls(t) {
		if c.fn != "Tf" || c.spread {
			continue
		}
		if want := verbsWanted(c.text, goVerb); want != c.args {
			t.Errorf("%s: Tf(%q) gets %d values for %d placeholders (16 §7)", c.pos, c.text, c.args, want)
		}
	}
}

// ---- 16 §3.5 ctrl+\ and §8.2: every page has a reference prefix and enters through beginPageRead ----

// Pages drawn without beginPageRead, with the reason.
var pageWithoutRead = map[string]string{
	"uninstall-progress": "uninstalling shows its progress in place; there is nothing to read first (uninstall.js)",
}

var (
	jsOpenScreen = regexp.MustCompile(`\bopenScreen\("([\w-]+)"`)
	jsPageRead   = regexp.MustCompile(`\bbeginPageRead\("([\w-]+)"`)
	jsPrefix     = regexp.MustCompile(`(?s)const PREFIX = \{(.*?)\};`)
)

func TestUIPagesHaveRefPrefixes(t *testing.T) {
	t.Parallel()
	scripts := pageScripts(t)
	opened, read := map[string]string{}, map[string]bool{}
	for name, src := range scripts {
		blank := jsBlank(src)
		for _, m := range jsOpenScreen.FindAllStringSubmatch(blank, -1) {
			opened[m[1]] = name
		}
		for _, m := range jsPageRead.FindAllStringSubmatch(blank, -1) {
			read[m[1]] = true
		}
	}
	m := jsPrefix.FindStringSubmatch(jsBlank(scripts["js/refnums.js"]))
	if m == nil {
		t.Fatal("refnums.js: no PREFIX table")
	}
	prefix, values := map[string]bool{}, map[string]string{}
	for _, kv := range jsSplitTop(m[1], ',') {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		k, v = strings.Trim(strings.TrimSpace(k), `"`), strings.Trim(strings.TrimSpace(v), `"`)
		prefix[k] = true
		if other, dup := values[v]; dup {
			t.Errorf("refnums.js: %s and %s share the prefix %q (16 §3.5 ctrl+\\)", other, k, v)
		}
		values[v] = k
	}
	pages := map[string]bool{}
	for p := range opened {
		pages[p] = true
	}
	for p := range read {
		pages[p] = true
	}
	for p := range pages {
		if !prefix[p] {
			t.Errorf("16 §3.5 ctrl+\\: page %q has no reference-number prefix in refnums.js PREFIX; add one (prefixes are durable: never change an existing one)", p)
		}
	}
	for p := range prefix {
		if !pages[p] {
			t.Errorf("refnums.js PREFIX has %q, which no openScreen or beginPageRead opens", p)
		}
	}
	for p, file := range opened {
		_, allowed := pageWithoutRead[p]
		switch {
		case !read[p] && !allowed:
			t.Errorf("assets/%s: 16 §8.2: page %q is opened without beginPageRead (enter first, then read)", file, p)
		case read[p] && allowed:
			t.Errorf("%s: listed in pageWithoutRead but it has a beginPageRead; remove it", p)
		}
	}
	for p := range pageWithoutRead {
		if opened[p] == "" {
			t.Errorf("%s: listed in pageWithoutRead but no openScreen opens it; remove it", p)
		}
	}
}

// ---- 16 §3.9: data tables ----

// Plain tables marked up as tables but not data tables (§3.9 excludes fixed item → value
// tables and short summaries).
var plainTables = map[string]string{
	"js/detail.js":     "a Mod description's own tables (Steam BBCode [table])",
	"js/planmanage.js": "the list's memory and load-time cost summary, a few fixed rows",
	"js/popups.js":     "the identity question's side-by-side comparison of two copies",
}

var (
	jsCreateTable = regexp.MustCompile(`createTable\(\{\s*key:\s*("[^"]+"|\w+)`)
	jsTableRole   = regexp.MustCompile(`role["']?\s*[,:=]\s*"(?:table|grid)"|el\(\s*"table"`)
	specTables    = regexp.MustCompile("These rules are implemented for (.*)")
	specTableKey  = regexp.MustCompile("\\(`?([a-z][\\w.]*)`?[,)]")
)

func TestUIDataTables(t *testing.T) {
	t.Parallel()
	spec, err := os.ReadFile("../../../docs/specs/16-interface/3-interaction-rules.md")
	if err != nil {
		t.Fatal(err)
	}
	line := specTables.FindStringSubmatch(string(spec))
	if line == nil {
		t.Fatal("3-interaction-rules.md §3.9 no longer lists its tables (These rules are implemented for …)")
	}
	want := map[string]bool{}
	for _, m := range specTableKey.FindAllStringSubmatch(line[1], -1) {
		want[m[1]] = true
	}
	got, tableFiles := map[string]string{}, map[string]bool{"js/datatable.js": true}
	scripts := pageScripts(t)
	for name, src := range scripts {
		blank := jsBlank(src)
		for _, m := range jsCreateTable.FindAllStringSubmatch(blank, -1) {
			key := m[1]
			if !strings.HasPrefix(key, `"`) {
				c := regexp.MustCompile(`const ` + key + `\s*=\s*("[^"]+")`).FindStringSubmatch(blank)
				if c == nil {
					t.Errorf("assets/%s: createTable key %s: cannot read its value", name, key)
					continue
				}
				key = c[1]
			}
			key = strings.TrimSuffix(strings.TrimPrefix(strings.Trim(key, `"`), "ui.gui."), ".columns")
			got[key] = name
			tableFiles[name] = true
		}
	}
	for k, file := range got {
		if !want[k] {
			t.Errorf("assets/%s: data table %q is not in 16 §3.9's list of tables; add it there", file, k)
		}
	}
	for k := range want {
		if got[k] == "" {
			t.Errorf("16 §3.9 lists the table %q but no createTable uses ui.gui.%s.columns", k, k)
		}
	}
	used := map[string]bool{}
	for name, src := range scripts {
		if tableFiles[name] {
			continue
		}
		for _, m := range jsTableRole.FindAllStringIndex(jsBlank(src), -1) {
			if _, ok := plainTables[name]; ok {
				used[name] = true
				continue
			}
			t.Errorf("assets/%s:%d: 16 §3.9: a table here is no data table (createTable); make it one or list it in plainTables with why it is excluded", name, 1+strings.Count(src[:m[0]], "\n"))
		}
	}
	for k := range plainTables {
		if !used[k] {
			t.Errorf("%s: listed in plainTables but has no table any more; remove it", k)
		}
	}
}

// ---- 16 §7.1: scrollers keep their scrollbar's room ----

// What 16 §7.1 "Nothing moves after it is drawn" names, and the scroller that is each.
var stableScrollers = map[string][]string{
	"the editor lists": {".rows"},
	"details":          {".detail .scroll", ".lib-detail .scroll"},
	"Mod library":      {".lib-rows"},
	"pages":            {"#screen", ".page-scroll"},
	"Can it run":       {".needcol > .scroll"},
	"tasks":            {".tk-rows"},
	"updates":          {".subs-list > .scroll", ".subs-detail > .scroll"},
	"saves":            {".sv-body"},
	"Mod options":      {".ms-body"},
	"All lists":        {".pm-list .scroll"},
}

var specScrollers = regexp.MustCompile("`scrollbar-gutter: stable`: ([^)]*)\\)")

func TestUIScrollersStable(t *testing.T) {
	t.Parallel()
	spec, err := os.ReadFile("../../../docs/specs/16-interface/7-visual-design.md")
	if err != nil {
		t.Fatal(err)
	}
	m := specScrollers.FindStringSubmatch(string(spec))
	if m == nil {
		t.Fatal("7-visual-design.md §7.1 no longer lists the scrollers with scrollbar-gutter: stable")
	}
	named := map[string]bool{}
	for _, n := range strings.Split(m[1], ",") {
		n = strings.TrimSpace(n)
		named[n] = true
		if _, ok := stableScrollers[n]; !ok {
			t.Errorf("16 §7.1 names %q as a scroller that keeps its scrollbar's room; map it to its selector in stableScrollers", n)
		}
	}
	for n := range stableScrollers {
		if !named[n] {
			t.Errorf("stableScrollers has %q, which 16 §7.1 no longer names; remove it", n)
		}
	}
	b, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	stable := map[string]bool{}
	for _, d := range cssDeclList(string(b)) {
		if d.name != "scrollbar-gutter" || d.value != "stable" {
			continue
		}
		for _, s := range cssSelectors(d.sel) {
			stable[s] = true
		}
	}
	for n, sels := range stableScrollers {
		for _, s := range sels {
			if !stable[s] {
				t.Errorf("16 §7.1: %s (%s) has no scrollbar-gutter: stable in style.css, so a scrollbar coming or going moves its content", n, s)
			}
		}
	}
}

// cssSelectors expands a selector list, including one :is(…) list, into single selectors.
func cssSelectors(sel string) []string {
	var out []string
	for _, part := range splitStyleShadows(sel) {
		part = strings.TrimSpace(part)
		if i := strings.Index(part, ":is("); i >= 0 {
			end := strings.LastIndexByte(part, ')')
			for _, inner := range splitStyleShadows(part[i+4 : end]) {
				out = append(out, strings.TrimSpace(part[:i]+strings.TrimSpace(inner)+part[end+1:]))
			}
			continue
		}
		out = append(out, part)
	}
	return out
}

// ---- 16 §8.2: every event the page listens to has a Go sender ----

func TestUIListenedEventsAreSent(t *testing.T) {
	t.Parallel()
	sent := map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(tree, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "send" {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					name, _ := strconv.Unquote(lit.Value)
					sent[name] = true
				}
			}
			return true
		})
	}
	listen := regexp.MustCompile(`\bon\("([^"]+)"`)
	for name, src := range pageScripts(t) {
		for _, m := range listen.FindAllStringSubmatchIndex(jsBlank(src), -1) {
			if event := src[m[2]:m[3]]; !sent[event] {
				t.Errorf("assets/%s:%d: 16 §8.2: the page listens to %q, which no Go code sends (API.send)", name, 1+strings.Count(src[:m[0]], "\n"), event)
			}
		}
	}
}

// These examples guard the contract readers themselves.
func TestUIContractsDetection(t *testing.T) {
	t.Parallel()
	main := `async function cmd(id) {
  const pages = { home: "home", "env-page": "env", library: "library" };
  if (pages[id]) return go(pages[id]);
  switch (id) {
    case "sort": case "save": return run(id);
    case "quit": return window.close();
  }
}`
	got := commandProblems(main, []string{"library", "sort", "save", "quit", "update"})
	if len(got) != 3 || !strings.Contains(strings.Join(got, "\n"), `"update"`) || !strings.Contains(strings.Join(got, "\n"), `"env-page"`) {
		t.Errorf("commands: %v", got)
	}
	for _, tc := range []struct {
		js   string
		want int
	}{
		{`say("ok", Tf("%d of %d", a, b)); Tf("%[2]s by %[1]s", x, y); Tf(key, n); Tf("%s", ...xs);`, 0},
		{`Tf("%d mods", a, b);`, 1},
		{"// Tf(\"%d\")\nTf(\"%s and %s\", f(a, b));", 1},
		{`export function Tf(f, ...a) {}`, 0},
	} {
		if got := jsFormatProblems("js/x.js", tc.js); len(got) != tc.want {
			t.Errorf("%s: got %v; want %d", tc.js, got, tc.want)
		}
	}
	if got := verbsWanted("%d%% of %s, %v", goVerb); got != 3 {
		t.Errorf("verbsWanted: %d", got)
	}
	if got := cssSelectors(".a, :is(#b, .c > .d) .e"); strings.Join(got, "|") != ".a|#b .e|.c > .d .e" {
		t.Errorf("cssSelectors: %q", got)
	}
}

// The saves page's verdict (16 §2.11): its button wraps inside the card instead of running
// past a narrow middle card's edge ("make a list from this save to analyse its assemblies").
func TestUISaveVerdictButtonWraps(t *testing.T) {
	b, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range cssDeclList(string(b)) {
		if d.sel == ".sv-fixbar > .btn" {
			got[d.name] = d.value
		}
	}
	for name, want := range map[string]string{"white-space": "normal", "max-width": "100%", "height": "auto", "min-width": "0"} {
		if got[name] != want {
			t.Errorf("16 §2.11: .sv-fixbar > .btn %s is %q, want %q, so a long verdict button stays in the card", name, got[name], want)
		}
	}
}
