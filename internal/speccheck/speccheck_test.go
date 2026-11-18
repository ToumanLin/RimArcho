package speccheck

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Spec checks that need no package's internals: cited tests and Go identifiers exist, the
// finding catalog (05 §4.3) matches the emitted codes, acceptance IDs are unique, and the
// scheduled-behaviour table (16 §2.16.1) matches the code. Each failure names the section
// to read. Exceptions carry a reason; one that no longer matches fails as stale.

// ---- 1. Cited tests exist ----

// Cited test names that are not Go test functions.
var citedTestExceptions = map[string]string{}

var (
	testCiteRe = regexp.MustCompile(`\b(?:([a-z][a-z0-9]*)\.)?(Test[A-Z][A-Za-z0-9_]*)(\*)?`)
	runArgRe   = regexp.MustCompile(`-run[= ]+['"]?([^'"\s]+)`)
)

func TestSpecCitedTests(t *testing.T) {
	t.Parallel()
	docs, err := Docs()
	if err != nil {
		t.Fatal(err)
	}
	tests, err := TestFuncs()
	if err != nil {
		t.Fatal(err)
	}
	ex := NewExceptions("citedTestExceptions (internal/speccheck/speccheck_test.go)", citedTestExceptions)
	for _, p := range citedTestProblems(docs, tests, ex) {
		t.Error(p)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

func citedTestProblems(docs []*Doc, tests []TestFunc, ex *Exceptions) []string {
	var out []string
	for _, d := range docs {
		for _, l := range d.Lines {
			prefix := map[string]bool{}
			for _, m := range runArgRe.FindAllStringSubmatch(l.Text, -1) {
				for _, n := range strings.Split(m[1], "|") {
					prefix[strings.Trim(n, "^$()")] = true
				}
			}
			for _, m := range testCiteRe.FindAllStringSubmatch(l.Text, -1) {
				pkg, name := m[1], m[2]
				isPrefix := m[3] == "*" || prefix[name]
				found := false
				for _, tf := range tests {
					if pkg != "" && tf.Pkg != pkg && path.Base(path.Dir(tf.File)) != pkg {
						continue
					}
					if tf.Name == name || isPrefix && strings.HasPrefix(tf.Name, name) {
						found = true
						break
					}
				}
				cite := name
				if pkg != "" {
					cite = pkg + "." + name
				}
				if !found && !ex.Has(cite) {
					where := "any _test.go"
					if pkg != "" {
						where = "package " + pkg + "'s tests"
					}
					out = append(out, fmt.Sprintf("%s: %s cites test %s, but %s has no func %s (rename the citation, or add an exception with the reason if it is not a Go test)",
						l.Pos(), sectionOf(d, l.N), cite, where, name))
				}
			}
		}
	}
	return out
}

// sectionOf names the section a line is in, "NN §x.y", for failure messages.
func sectionOf(d *Doc, n int) string {
	nn := specNumber(d.Path)
	for i := n - 1; i >= 0; i-- {
		if m := headingRe.FindStringSubmatch(d.Lines[i].Text); m != nil {
			if nn == "" {
				return d.Path + " §" + m[2]
			}
			return nn + " §" + m[2]
		}
	}
	if nn == "" {
		return d.Path
	}
	return nn
}

var specFileRe = regexp.MustCompile(`^docs/specs/(\d\d)-`)

func specNumber(path string) string {
	if m := specFileRe.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	if path == "docs/00a-architecture.md" {
		return "00a"
	}
	return ""
}

// ---- 2. Cited Go identifiers exist ----

// Backticked identifiers of the form pkg.Ident that are not Go declarations.
var identExceptions = map[string]string{
	"tui.cmdline.history": "a database meta key (16 §9.4), not Go",
}

var identRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)\.([A-Za-z_][A-Za-z0-9_]*)(?:\.([A-Za-z_][A-Za-z0-9_]*))?(?:\(\))?$`)

func TestSpecGoIdentifiers(t *testing.T) {
	t.Parallel()
	docs, err := Docs()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Packages(); err != nil {
		t.Fatal(err)
	}
	ex := NewExceptions("identExceptions (internal/speccheck/speccheck_test.go)", identExceptions)
	for _, p := range identProblems(docs, ex) {
		t.Error(p)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

// identProblems checks backticked pkg.Ident, pkg.Type.Member, App.X and Store.X. To stay
// conservative, pkg must be the name of a package under internal/, and a two-part name
// whose identifier is one lower-case word (`order.violated`, a finding code; `known.json`,
// a file) is not taken for Go.
func identProblems(docs []*Doc, ex *Exceptions) []string {
	var out []string
	for _, d := range docs {
		for _, l := range d.Lines {
			for _, s := range Spans(l.Text) {
				m := identRe.FindStringSubmatch(s)
				if m == nil {
					continue
				}
				first, second, third := m[1], m[2], m[3]
				var pkgs []*Package
				typ, member := "", ""
				switch {
				case first == "App" || first == "Store":
					if third != "" {
						continue
					}
					pkgs = PackagesNamed(strings.ToLower(first))
					typ, member = first, second
				case first == strings.ToLower(first):
					pkgs = PackagesNamed(first)
					if len(pkgs) == 0 {
						continue
					}
					if third == "" && strings.ToLower(second) == second {
						continue
					}
					if third != "" {
						typ, member = second, third
					}
				default:
					continue
				}
				if strings.HasPrefix(second, "Test") || strings.HasPrefix(third, "Test") {
					continue // a test: TestSpecCitedTests
				}
				ok := false
				for _, p := range pkgs {
					if typ == "" && p.Has(second) || typ != "" && p.Types[typ] && p.HasMember(typ, member) {
						ok = true
					}
					// The specifications write App's and Store's methods as app.X and store.X.
					if typ == "" && (p.Dir == "internal/app" && p.HasMember("App", second) || p.Dir == "internal/store" && p.HasMember("Store", second)) {
						ok = true
					}
				}
				name := strings.TrimSuffix(s, "()")
				if !ok && !ex.Has(name) {
					out = append(out, fmt.Sprintf("%s: %s names `%s`, which internal/ does not declare (follow the code: rename or remove the reference)", l.Pos(), sectionOf(d, l.N), name))
				}
			}
		}
	}
	return out
}

// ---- 3. Finding codes and the catalog (05 §4.3) ----

// Codes in Finding literals or the catalog that are not one-for-one.
var findingExceptions = map[string]string{}

var findingCodeRe = regexp.MustCompile(`^[a-z]+\.[a-z0-9-]+$`)

type emitted struct {
	sev map[string]bool // empty when the severity is not a constant
	pos string
}

func TestSpecFindingCatalog(t *testing.T) {
	t.Parallel()
	sec, err := Section("05", "4.3")
	if err != nil {
		t.Fatal(err)
	}
	catalog, catPos := findingCatalog(sec)
	if len(catalog) < 30 {
		t.Fatalf("05 §4.3: only %d codes read from the catalog tables", len(catalog))
	}
	codes, err := emittedCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) < 30 {
		t.Fatalf("only %d emitted finding codes found", len(codes))
	}
	ex := NewExceptions("findingExceptions (internal/speccheck/speccheck_test.go)", findingExceptions)
	for _, p := range findingProblems(catalog, catPos, codes, ex) {
		t.Error(p)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

// findingCatalog reads every table of 05 §4.3 whose first column is `code`: code → the
// severities its Severity cell names.
func findingCatalog(sec []Line) (map[string]map[string]bool, map[string]string) {
	cat, pos := map[string]map[string]bool{}, map[string]string{}
	for _, tab := range Tables(sec) {
		if len(tab) < 2 || tab[0].Cells[0] != "`code`" {
			continue
		}
		sevCol := Column(tab[0], "Severity")
		for _, r := range tab[1:] {
			sevs := map[string]bool{}
			if sevCol >= 0 && sevCol < len(r.Cells) {
				for _, w := range regexp.MustCompile(`[a-z]+`).FindAllString(r.Cells[sevCol], -1) {
					if w == "error" || w == "warning" || w == "info" {
						sevs[w] = true
					}
				}
			}
			// A code may have several rows (the 4.3.1 severity basis splits version.unsupported).
			for _, c := range Spans(r.Cells[0]) {
				if cat[c] == nil {
					cat[c], pos[c] = map[string]bool{}, r.Line.Pos()
				}
				for s := range sevs {
					cat[c][s] = true
				}
			}
		}
	}
	return cat, pos
}

func findingProblems(catalog map[string]map[string]bool, catPos map[string]string, codes map[string]*emitted, ex *Exceptions) []string {
	var out []string
	for _, c := range sortedKeys(codes) {
		e := codes[c]
		doc, ok := catalog[c]
		if !ok {
			if !ex.Has(c) {
				out = append(out, fmt.Sprintf("05 §4.3: finding code %s is emitted (%s) but missing from the catalog", c, e.pos))
			}
			continue
		}
		for _, s := range sortedKeys(e.sev) {
			if !doc[s] && !ex.Has(c+" "+s) {
				out = append(out, fmt.Sprintf("05 §4.3: finding code %s is emitted as %s (%s) but its catalog row (%s) says %s", c, s, e.pos, catPos[c], strings.Join(sortedKeys(doc), " / ")))
			}
		}
	}
	for _, c := range sortedKeys(catalog) {
		if _, ok := codes[c]; !ok && !ex.Has(c) {
			out = append(out, fmt.Sprintf("05 §4.3: finding code %s is in the catalog (%s) but no Finding emits it", c, catPos[c]))
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// emittedCodes collects every finding code: the Code of finding.Finding literals (string
// literals and constants), the string constants named Code… that hold a finding code (they
// reach a literal through a field, as GameLink.Code does), and "log."+class for every class
// of logSeverity (internal/app/logs.go), with their severities.
func emittedCodes() (map[string]*emitted, error) {
	pkgs, err := Packages()
	if err != nil {
		return nil, err
	}
	fin := PackageAt("internal/finding")
	logsPkg := PackageAt("internal/logs")
	if fin == nil || logsPkg == nil {
		return nil, fmt.Errorf("internal/finding or internal/logs not found")
	}
	out := map[string]*emitted{}
	add := func(code, sev, pos string) {
		e := out[code]
		if e == nil {
			e = &emitted{sev: map[string]bool{}, pos: pos}
			out[code] = e
		}
		if sev != "" {
			e.sev[sev] = true
		}
	}
	sevOf := func(x ast.Expr) string {
		if s, ok := x.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && id.Name == "finding" {
				return fin.Strings[s.Sel.Name]
			}
		}
		if id, ok := x.(*ast.Ident); ok && fin.Strings[id.Name] != "" {
			return fin.Strings[id.Name] // inside package finding
		}
		return ""
	}
	for _, p := range pkgs {
		for name, v := range p.Strings {
			if strings.HasPrefix(name, "Code") && findingCodeRe.MatchString(v) {
				add(v, "", p.Dir+": const "+name)
			}
		}
		for file, f := range p.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.GenDecl:
					if file == "internal/app/logs.go" {
						logSeverities(d, logsPkg, sevOf, add, p.Fset)
					}
				case *ast.FuncDecl:
					if d.Body == nil {
						continue
					}
					ast.Inspect(d.Body, func(n ast.Node) bool {
						cl, ok := n.(*ast.CompositeLit)
						if !ok || !isFindingType(cl.Type) {
							return true
						}
						var codeX, sevX ast.Expr
						for _, el := range cl.Elts {
							if kv, ok := el.(*ast.KeyValueExpr); ok {
								switch kv.Key.(*ast.Ident).Name {
								case "Code":
									codeX = kv.Value
								case "Severity":
									sevX = kv.Value
								}
							}
						}
						if codeX == nil {
							return true
						}
						code, ok := StringValue(codeX, func(id string) (ast.Expr, bool) {
							if v, ok := p.Strings[id]; ok {
								return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", v)}, true
							}
							return nil, false
						})
						if !ok {
							return true
						}
						pos := p.Fset.Position(cl.Pos())
						where := fmt.Sprintf("%s:%d", file, pos.Line)
						sevs := literalSeverities(d.Body, cl, sevX, sevOf)
						add(code, "", where)
						for _, s := range sevs {
							add(code, s, where)
						}
						return true
					})
				}
			}
		}
	}
	return out, nil
}

func isFindingType(x ast.Expr) bool {
	switch x := x.(type) {
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		return ok && id.Name == "finding" && x.Sel.Name == "Finding"
	case *ast.Ident:
		return x.Name == "Finding"
	}
	return false
}

// literalSeverities resolves a literal's Severity: a constant, or a local variable and a
// later f.Severity = … on the variable holding the literal, from assignments in body.
func literalSeverities(body *ast.BlockStmt, cl *ast.CompositeLit, sevX ast.Expr, sevOf func(ast.Expr) string) []string {
	var out []string
	if sevX != nil {
		if s := sevOf(sevX); s != "" {
			return []string{s}
		}
	}
	sevVar := ""
	if id, ok := sevX.(*ast.Ident); ok {
		sevVar = id.Name
	}
	holder := ""
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for i, r := range as.Rhs {
			x := r
			if u, ok := x.(*ast.UnaryExpr); ok && u.Op == token.AND {
				x = u.X
			}
			if x == cl {
				if id, ok := as.Lhs[i].(*ast.Ident); ok {
					holder = id.Name
				}
			}
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for i, l := range as.Lhs {
			s := sevOf(as.Rhs[i])
			if s == "" {
				continue
			}
			if id, ok := l.(*ast.Ident); ok && sevVar != "" && id.Name == sevVar {
				out = append(out, s)
			}
			if se, ok := l.(*ast.SelectorExpr); ok && se.Sel.Name == "Severity" && holder != "" {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == holder {
					out = append(out, s)
				}
			}
		}
		return true
	})
	return out
}

func logSeverities(d *ast.GenDecl, logsPkg *Package, sevOf func(ast.Expr) string, add func(code, sev, pos string), fset *token.FileSet) {
	for _, s := range d.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "logSeverity" || len(vs.Values) != 1 {
			continue
		}
		cl, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, el := range cl.Elts {
			kv := el.(*ast.KeyValueExpr)
			sel, ok := kv.Key.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			class, ok := logsPkg.Strings[sel.Sel.Name]
			if !ok {
				continue
			}
			add("log."+class, sevOf(kv.Value), fmt.Sprintf("internal/app/logs.go:%d logSeverity", fset.Position(kv.Pos()).Line))
		}
	}
}

// ---- 5. Acceptance IDs are unique within each file ----

var acceptIDRe = regexp.MustCompile(`^[A-Z]{1,3}\d+[a-z]?$`)

func TestSpecAcceptanceIDs(t *testing.T) {
	t.Parallel()
	docs, err := Docs()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range acceptanceProblems(docs) {
		t.Error(p)
	}
}

// acceptanceProblems finds an ID used by two rows of the Acceptance tables of one file.
func acceptanceProblems(docs []*Doc) []string {
	var out []string
	for _, d := range docs {
		seen := map[string]Line{}
		in := false
		for _, l := range d.Lines {
			if m := regexp.MustCompile(`^(#{2,6})\s`).FindString(l.Text); m != "" {
				in = strings.Contains(l.Text, "Acceptance")
				continue
			}
			if !in || !strings.HasPrefix(strings.TrimSpace(l.Text), "|") {
				continue
			}
			cells := SplitRow(l.Text)
			id := strings.TrimSpace(cells[0])
			if !acceptIDRe.MatchString(id) {
				continue
			}
			if first, dup := seen[id]; dup {
				out = append(out, fmt.Sprintf("%s: %s: acceptance ID %s is already used at line %d; give the new row the next free number", l.Pos(), sectionOf(d, l.N), id, first.N))
				continue
			}
			seen[id] = l
		}
	}
	return out
}

// ---- 6. Scheduled-behaviour table (16 §2.16.1) ----

// Timers in files the table does not list that carry no `// not scheduled:` comment; for
// files this lint may not edit. Key: "<file relative to internal/>:<line text trimmed>".
var timerExceptions = map[string]string{
	"surface/gui/command.go:slow := time.AfterFunc(time.Second, func() {":     "not scheduled: a command panel line still running after a second gets a Background tasks row (UI); internal/surface/gui is edited by the GUI owner, who may move this into a comment",
	"surface/gui/selfupdate.go:time.AfterFunc(1500*time.Millisecond, s.quit)": "not scheduled: the window quits shortly after starting the updated program (17 §5.1); internal/surface/gui is edited by the GUI owner, who may move this into a comment",
}

var (
	rowRefRe   = regexp.MustCompile(`2\.16\.1 ([A-Z]\d\d)\b`)
	rowIDRe    = regexp.MustCompile(`^([A-Z]\d\d) `)
	timerRe    = regexp.MustCompile(`\btime\.(NewTicker|Tick|AfterFunc|NewTimer)\(`)
	notSchedRe = regexp.MustCompile(`//\s*not scheduled:\s*\S`)
)

type schedRow struct {
	id   string
	code []string // "file:func/func" spans
	line Line
}

func TestSpecScheduledTable(t *testing.T) {
	t.Parallel()
	sec, err := Section("16", "2.16.1")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := scheduleRows(sec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 40 {
		t.Fatalf("16 §2.16.1: only %d rows read", len(rows))
	}
	docs, err := Docs()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range scheduleCodeProblems(rows, readSource) {
		t.Error(p)
	}
	for _, p := range scheduleRefProblems(rows, docs) {
		t.Error(p)
	}
	ex := NewExceptions("timerExceptions (internal/speccheck/speccheck_test.go)", timerExceptions)
	listed := map[string]bool{}
	for _, r := range rows {
		for _, c := range r.code {
			f, _, _ := strings.Cut(c, ":")
			listed[schedFile(f)] = true
		}
	}
	err = filepath.WalkDir(filepath.Join(Root(), "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, pr := range timerProblems(rel(p), string(b), listed, ex) {
			t.Error(pr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ex.Stale() {
		t.Error(p)
	}
}

func scheduleRows(sec []Line) ([]schedRow, error) {
	var out []schedRow
	for _, tab := range Tables(sec) {
		col := Column(tab[0], "Code (file:function)")
		if col < 0 {
			continue
		}
		for _, r := range tab[1:] {
			m := rowIDRe.FindStringSubmatch(r.Cells[0])
			if m == nil || col >= len(r.Cells) {
				return nil, fmt.Errorf("%s: 16 §2.16.1: row without an ID or code cell", r.Line.Pos())
			}
			out = append(out, schedRow{id: m[1], code: Spans(r.Cells[col]), line: r.Line})
		}
	}
	return out, nil
}

// schedFile maps a table path (relative to internal/, JS/ for the GUI scripts) to a slash
// path relative to the root.
func schedFile(f string) string {
	if rest, ok := strings.CutPrefix(f, "JS/"); ok {
		return "internal/surface/gui/assets/js/" + rest
	}
	return "internal/" + f
}

func readSource(path string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(Root(), filepath.FromSlash(path)))
	return string(b), err == nil
}

// scheduleCodeProblems checks each "file:function/function" of the Code column: the file
// exists and defines each function (Go: a function, a method, Type.method, a var or const;
// JS: a function, method or arrow function).
func scheduleCodeProblems(rows []schedRow, read func(string) (string, bool)) []string {
	var out []string
	for _, r := range rows {
		if len(r.code) == 0 {
			out = append(out, fmt.Sprintf("%s: 16 §2.16.1 %s: the Code column names no file", r.line.Pos(), r.id))
		}
		for _, c := range r.code {
			f, funcs, _ := strings.Cut(c, ":")
			path := schedFile(f)
			src, ok := read(path)
			if !ok {
				out = append(out, fmt.Sprintf("%s: 16 §2.16.1 %s: file %s does not exist", r.line.Pos(), r.id, path))
				continue
			}
			if funcs == "" {
				continue
			}
			for _, fn := range strings.Split(funcs, "/") {
				if !definesFunc(path, src, fn) {
					out = append(out, fmt.Sprintf("%s: 16 §2.16.1 %s: %s does not define %s", r.line.Pos(), r.id, path, fn))
				}
			}
		}
	}
	return out
}

func definesFunc(path, src, fn string) bool {
	if strings.HasSuffix(path, ".go") {
		p := PackageAt(filepath.ToSlash(filepath.Dir(path)))
		if p != nil && p.Defined[path] != nil {
			return p.Defined[path][fn]
		}
		// a file outside the parsed packages: fall back to text
		return regexp.MustCompile(`func (\([^)]*\) )?` + regexp.QuoteMeta(fn[strings.LastIndex(fn, ".")+1:]) + `\b`).MatchString(src)
	}
	name := regexp.QuoteMeta(fn[strings.LastIndex(fn, ".")+1:])
	re := regexp.MustCompile(`(?m)(\bfunction\*?\s+` + name + `\s*\(|\b` + name + `\s*[:=]\s*(async\s+)?(function\b|\([^)]*\)\s*=>|\w+\s*=>)|^\s*(async\s+)?` + name + `\s*\([^)]*\)\s*\{)`)
	return re.MatchString(src)
}

// scheduleRefProblems checks that every "2.16.1 X00" reference in the specifications names
// a row of the table.
func scheduleRefProblems(rows []schedRow, docs []*Doc) []string {
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.id] = true
	}
	var out []string
	for _, d := range docs {
		for _, l := range d.Lines {
			for _, m := range rowRefRe.FindAllStringSubmatch(l.Text, -1) {
				if !ids[m[1]] {
					out = append(out, fmt.Sprintf("%s: %s refers to 16 §2.16.1 %s, which is not a row of the table", l.Pos(), sectionOf(d, l.N), m[1]))
				}
			}
		}
	}
	return out
}

// timerProblems: a timer in a file the table does not list needs a `// not scheduled:
// <reason>` comment on its line or the line above.
func timerProblems(path, src string, listed map[string]bool, ex *Exceptions) []string {
	if listed[path] {
		return nil
	}
	var out []string
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i, l := range lines {
		if !timerRe.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		if notSchedRe.MatchString(l) || i > 0 && notSchedRe.MatchString(lines[i-1]) {
			continue
		}
		short := strings.TrimPrefix(path, "internal/")
		if ex.Has(short + ":" + strings.TrimSpace(l)) {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d: 16 §2.16.1: a timer in a file the scheduled-behaviour table does not list; add a row for it, or a `// not scheduled: <reason>` comment if it is not scheduled behaviour (a UI throttle, a process poll)", path, i+1))
	}
	return out
}

// ---- The lint guards itself ----

func TestSpecDetection(t *testing.T) {
	t.Parallel()
	doc := func(path string, lines ...string) *Doc {
		d := &Doc{Path: path}
		for i, l := range lines {
			d.Lines = append(d.Lines, Line{Doc: path, N: i + 1, Text: l})
		}
		return d
	}
	tests := []TestFunc{{Name: "TestFooBar", Pkg: "tui"}, {Name: "TestEndlessAnimationsOnlyWhileWorking", Pkg: "gui"}}
	for _, tc := range []struct {
		line string
		want int
	}{
		{"`TestFooBar` and `tui.TestFooBar`", 0},
		{"`TestFoo*` and go test -run 'TestNone|TestEndlessAnimations' ./x", 1},
		{"`gui.TestFooBar`, `TestMissing`", 2},
		{"go test -run TestFoo ./x; TestFoo alone", 0},
	} {
		ex := NewExceptions("x", map[string]string{})
		if got := citedTestProblems([]*Doc{doc("docs/specs/99-x.md", "## 1. X", tc.line)}, tests, ex); len(got) != tc.want {
			t.Errorf("cited tests %q: got %v; want %d", tc.line, got, tc.want)
		}
	}
	ex := NewExceptions("x", map[string]string{"TestGone": "a Node test"})
	if got := citedTestProblems([]*Doc{doc("docs/specs/99-x.md", "TestGone")}, tests, ex); len(got) != 0 || len(ex.Stale()) != 0 {
		t.Errorf("an exception was not honoured: %v %v", got, ex.Stale())
	}
	if ex := NewExceptions("x", map[string]string{"TestGone": "x"}); len(ex.Stale()) != 1 {
		t.Error("an unused exception is not stale")
	}

	for _, tc := range []struct {
		line string
		want int
	}{
		{"`app.App`, `App.Store`, `store.SchemaVersion`, `finding.Finding.Code`, `cmds.Commands()`", 0},
		{"`order.violated`, `known.json`, `ui.gui.x`, `harmony.manual`, `lipgloss.HasDarkBackground`", 0},
		{"`view.NoSuchThing`, `App.NoSuchMethod`, `store.Store.NoField`, `finding.Nope()`", 4},
	} {
		if got := identProblems([]*Doc{doc("docs/specs/99-x.md", tc.line)}, NewExceptions("x", map[string]string{})); len(got) != tc.want {
			t.Errorf("identifiers %q: got %v; want %d", tc.line, got, tc.want)
		}
	}

	cat, pos := findingCatalog([]Line{
		{Text: "| `code` | Severity | Detection |"}, {Text: "| --- | --- | --- |"},
		{Text: "| `a.one` | warning / info | x |"}, {Text: "| `a.two`, `a.three` | info, may become warning | x |"},
	})
	if len(cat) != 3 || !cat["a.three"]["warning"] || !cat["a.one"]["info"] || cat["a.one"]["error"] {
		t.Fatalf("catalog %v", cat)
	}
	got := findingProblems(cat, pos, map[string]*emitted{
		"a.one": {sev: map[string]bool{"error": true}}, "a.two": {sev: map[string]bool{}}, "a.new": {sev: map[string]bool{}},
	}, NewExceptions("x", map[string]string{}))
	if len(got) != 3 { // a.one as error, a.new uncatalogued, a.three never emitted
		t.Errorf("finding problems: %v", got)
	}

	if got := acceptanceProblems([]*Doc{doc("docs/specs/99-x.md", "## 9. Acceptance", "| # | x |", "| --- | --- |", "| B7 | a |", "| B8 | b |", "| B7 | c |", "## 10. Other", "| B8 | d |")}); len(got) != 1 {
		t.Errorf("acceptance: %v", got)
	}

	rows := []schedRow{{id: "A01", code: []string{"x.go:Run/Type.stop", "JS/a.js:poll/read", "gone.go"}}, {id: "B02"}}
	src := map[string]string{
		"internal/x.go":                       "package x\nfunc Run() {}\nfunc (t *Type) stop() {}\n",
		"internal/surface/gui/assets/js/a.js": "function poll() {}\nconst other = () => 1;\n",
	}
	read := func(p string) (string, bool) { s, ok := src[p]; return s, ok }
	if got := scheduleCodeProblems(rows, read); len(got) != 3 { // read undefined, gone.go missing, B02 has no code
		t.Errorf("schedule code: %v", got)
	}
	if got := scheduleRefProblems(rows, []*Doc{doc("docs/specs/99-x.md", "16 §2.16.1 A01 and 2.16.1 T04")}); len(got) != 1 {
		t.Errorf("schedule refs: %v", got)
	}
	timers := "package x\nfunc f() {\n\tt := time.NewTicker(d)\n\t// not scheduled: a UI throttle\n\tu := time.AfterFunc(d, g)\n\tv := time.NewTimer(d) // not scheduled: a process poll\n\t// time.Tick(d) in a comment\n}\n"
	if got := timerProblems("internal/x/y.go", timers, map[string]bool{}, NewExceptions("x", map[string]string{})); len(got) != 1 || !strings.HasPrefix(got[0], "internal/x/y.go:3:") {
		t.Errorf("timers: %v", got)
	}
	if got := timerProblems("internal/x/y.go", timers, map[string]bool{"internal/x/y.go": true}, NewExceptions("x", map[string]string{})); len(got) != 0 {
		t.Errorf("timers in a listed file: %v", got)
	}
}
