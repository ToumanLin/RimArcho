// Package archlint holds static tests (test files only) that enforce the architecture and
// forbidden-API rules of the specifications: import layering (17 §2), the file write
// boundary (02 §1), the recycle bin entry point (02 §3), hidden child windows (17 §5),
// network listeners and transports (17 §2.2), pinned dependencies (17 §1, §7.1), process exit
// and terminal output (13 §3), the CHANGELOG layout and the MCP stdout rule (14 §1).
//
// Each failure names the specification section, so an agent who skipped the docs learns
// what to read. Exceptions live in Go maps beside each rule, each with its reason; an
// exception that no longer matches fails as stale, so the lists can shrink but never grow
// silently. Reasons starting "existing; open question:" mark violations that wait for a
// design decision. Every rule has a detection test that feeds it synthetic code, so the lint
// itself cannot silently stop matching. Run them all with:
//
//	go test -count=1 ./internal/archlint/
package archlint

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// goFile is one parsed Go file of the repository.
type goFile struct {
	path string // slash path from the repository root: internal/app/trash.go
	dir  string // its package directory: internal/app
	test bool   // a _test.go file
	f    *ast.File
	fset *token.FileSet
}

var (
	rootOnce sync.Once
	rootDir  string
	module   string
	rootErr  error
)

// repoRoot finds the directory holding go.mod and reads the module path.
func repoRoot(t testing.TB) (string, string) {
	t.Helper()
	rootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			rootErr = err
			return
		}
		for {
			b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
			if err == nil {
				rootDir = dir
				sc := bufio.NewScanner(strings.NewReader(string(b)))
				for sc.Scan() {
					if m, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
						module = strings.TrimSpace(m)
						break
					}
				}
				if module == "" {
					rootErr = fmt.Errorf("%s/go.mod has no module line", dir)
				}
				return
			}
			up := filepath.Dir(dir)
			if up == dir {
				rootErr = fmt.Errorf("no go.mod above %s", dir)
				return
			}
			dir = up
		}
	})
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	return rootDir, module
}

// sourceFiles lists the repository's .go files under internal/ and cmd/, skipping testdata.
func sourceFiles(root string) ([]string, error) {
	var out []string
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "testdata" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".go") {
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				out = append(out, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

type parsed struct {
	once  sync.Once
	files []*goFile
	err   error
}

var (
	importsOnly parsed // every file, imports only (layering, cgo)
	fullSource  parsed // non-test files, whole syntax
)

// parseRepo parses the repository's files in parallel; withTests includes _test.go files.
func parseRepo(t testing.TB, p *parsed, mode parser.Mode, withTests bool) []*goFile {
	t.Helper()
	root, _ := repoRoot(t)
	p.once.Do(func() {
		names, err := sourceFiles(root)
		if err != nil {
			p.err = err
			return
		}
		if !withTests {
			keep := names[:0]
			for _, n := range names {
				if !strings.HasSuffix(n, "_test.go") {
					keep = append(keep, n)
				}
			}
			names = keep
		}
		fset := token.NewFileSet()
		files := make([]*goFile, len(names))
		errs := make([]error, len(names))
		work := make(chan int)
		var wg sync.WaitGroup
		for range runtime.GOMAXPROCS(0) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range work {
					f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(names[i])), nil, mode|parser.SkipObjectResolution)
					errs[i] = err
					files[i] = &goFile{path: names[i], dir: path.Dir(names[i]), test: strings.HasSuffix(names[i], "_test.go"), f: f, fset: fset}
				}
			}()
		}
		for i := range names {
			work <- i
		}
		close(work)
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				p.err = err
				return
			}
		}
		p.files = files
	})
	if p.err != nil {
		t.Fatal(p.err)
	}
	return p.files
}

func repoImports(t testing.TB) []*goFile { return parseRepo(t, &importsOnly, parser.ImportsOnly, true) }
func repoSource(t testing.TB) []*goFile  { return parseRepo(t, &fullSource, 0, false) }

// synthetic parses detection-test sources: name is the repository path the code pretends to have.
func synthetic(t testing.TB, srcs map[string]string) []*goFile {
	t.Helper()
	fset := token.NewFileSet()
	var out []*goFile
	names := make([]string, 0, len(srcs))
	for n := range srcs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f, err := parser.ParseFile(fset, n, srcs[n], parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		out = append(out, &goFile{path: n, dir: path.Dir(n), test: strings.HasSuffix(n, "_test.go"), f: f, fset: fset})
	}
	return out
}

// imports maps the file's local import names to import paths ("_" and "." are skipped).
func (g *goFile) imports() map[string]string {
	m := map[string]string{}
	for _, s := range g.f.Imports {
		p, _ := strconv.Unquote(s.Path.Value)
		name := path.Base(p)
		if strings.HasPrefix(name, "v") && len(name) > 1 && strings.Trim(name[1:], "0123456789") == "" {
			name = path.Base(path.Dir(p)) // github.com/go-git/go-git/v5 is go-git…
		}
		name = strings.ReplaceAll(name, "-", "")
		if s.Name != nil {
			name = s.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		m[name] = p
	}
	return m
}

// ref is one use of a package-level name of an imported package (os.Remove, source.Trash).
type ref struct {
	pkg, name string
	pos       token.Position
	fn        string        // the enclosing function, "(*API).Ready", or "package level"
	call      *ast.CallExpr // the call when the reference is the called function
}

func (r ref) String() string { return r.pkg + "." + r.name }

// refs lists every pkg.Name selector in g whose pkg is an import.
func (g *goFile) refs() []ref {
	imp := g.imports()
	var out []ref
	visit := func(fn string, n ast.Node) {
		calls := map[*ast.SelectorExpr]*ast.CallExpr{}
		ast.Inspect(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if s, ok := n.Fun.(*ast.SelectorExpr); ok {
					calls[s] = n
				}
			case *ast.SelectorExpr:
				id, ok := n.X.(*ast.Ident)
				if !ok {
					return true
				}
				p, ok := imp[id.Name]
				if !ok {
					return true
				}
				out = append(out, ref{pkg: p, name: n.Sel.Name, pos: g.fset.Position(n.Pos()), fn: fn, call: calls[n]})
			}
			return true
		})
	}
	for _, d := range g.f.Decls {
		visit(declName(d), d)
	}
	return out
}

// declName names a top-level declaration: a function as Go prints it, a variable
// declaration "var name" (a function held in a variable is named so), else "package level".
func declName(d ast.Decl) string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return funcName(d)
	case *ast.GenDecl:
		if d.Tok == token.VAR && len(d.Specs) == 1 {
			if vs, ok := d.Specs[0].(*ast.ValueSpec); ok && len(vs.Names) == 1 {
				return "var " + vs.Names[0].Name
			}
		}
	}
	return "package level"
}

// funcName names a declaration as Go prints it: Foo, (*T).Foo, T.Foo.
func funcName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	star := ""
	if s, ok := t.(*ast.StarExpr); ok {
		star, t = "*", s.X
	}
	if ix, ok := t.(*ast.IndexExpr); ok {
		t = ix.X
	}
	if ix, ok := t.(*ast.IndexListExpr); ok {
		t = ix.X
	}
	name := "?"
	if id, ok := t.(*ast.Ident); ok {
		name = id.Name
	}
	if star != "" {
		return "(*" + name + ")." + fd.Name.Name
	}
	return name + "." + fd.Name.Name
}

// under reports whether dir is pkg or inside it ("internal/surface" holds internal/surface/gui).
func under(dir, pkg string) bool { return dir == pkg || strings.HasPrefix(dir, pkg+"/") }

// loc is file:line for messages.
func loc(p token.Position, file string) string { return fmt.Sprintf("%s:%d", file, p.Line) }

// staleProblems reports exceptions nothing needs any more.
func staleProblems(list string, allow map[string]string, used map[string]bool) []string {
	var out []string
	for k := range allow {
		if !used[k] {
			out = append(out, fmt.Sprintf("%s: %q is listed as an exception but nothing needs it any more; remove it from internal/archlint", list, k))
		}
	}
	sort.Strings(out)
	return out
}

// report fails t with each problem.
func report(t *testing.T, problems []string) {
	t.Helper()
	for _, p := range problems {
		t.Error(p)
	}
}

// expect checks a detection case: the rule finds exactly want problems.
func expect(t *testing.T, name string, got []string, want int) {
	t.Helper()
	if len(got) != want {
		t.Errorf("%s: got %d problems, want %d:\n  %s", name, len(got), want, strings.Join(got, "\n  "))
	}
}

func TestArchReasons(t *testing.T) {
	t.Parallel()
	for name, list := range allExceptionLists() {
		for k, why := range list {
			if strings.TrimSpace(why) == "" {
				t.Errorf("%s: exception %q has no reason", name, k)
			}
		}
	}
}
