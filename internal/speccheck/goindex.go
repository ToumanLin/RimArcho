package speccheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Package is what one Go package (its non-test files) declares.
type Package struct {
	Dir   string // slash path relative to the root, e.g. "internal/app"
	Name  string
	Files map[string]*ast.File // by slash path relative to the root
	Fset  *token.FileSet

	Funcs   map[string]bool   // top-level functions
	Types   map[string]bool   // named types
	Values  map[string]bool   // top-level consts and vars
	Strings map[string]string // string constants with a literal or resolvable value
	Methods map[string]bool   // "Type.Method", including interface methods
	Fields  map[string]bool   // "Type.Field", including embedded type names
	Embeds  map[string][]string
	// Defined maps a file to the names it declares: functions, "Type.Method" and the bare
	// method name, types, consts and vars.
	Defined map[string]map[string]bool
}

var (
	pkgOnce sync.Once
	pkgAll  []*Package
	pkgErr  error
)

// Packages parses every package under internal/ (non-test files only), once.
func Packages() ([]*Package, error) {
	pkgOnce.Do(func() {
		dirs := map[string][]string{}
		err := filepath.WalkDir(filepath.Join(Root(), "internal"), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if d.IsDir() || filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			dirs[filepath.Dir(p)] = append(dirs[filepath.Dir(p)], p)
			return nil
		})
		if err != nil {
			pkgErr = err
			return
		}
		var keys []string
		for k := range dirs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, dir := range keys {
			p, err := parsePackage(dir, dirs[dir])
			if err != nil {
				pkgErr = err
				return
			}
			pkgAll = append(pkgAll, p)
		}
	})
	return pkgAll, pkgErr
}

// PackagesNamed returns the packages whose package clause is name.
func PackagesNamed(name string) []*Package {
	all, _ := Packages()
	var out []*Package
	for _, p := range all {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// PackageAt returns the package in dir ("internal/app"), or nil.
func PackageAt(dir string) *Package {
	all, _ := Packages()
	for _, p := range all {
		if p.Dir == dir {
			return p
		}
	}
	return nil
}

func rel(p string) string {
	r, _ := filepath.Rel(Root(), p)
	return filepath.ToSlash(r)
}

func parsePackage(dir string, files []string) (*Package, error) {
	p := &Package{Dir: rel(dir), Files: map[string]*ast.File{}, Fset: token.NewFileSet(),
		Funcs: map[string]bool{}, Types: map[string]bool{}, Values: map[string]bool{}, Strings: map[string]string{},
		Methods: map[string]bool{}, Fields: map[string]bool{}, Embeds: map[string][]string{}, Defined: map[string]map[string]bool{}}
	sort.Strings(files)
	consts := map[string]ast.Expr{}
	for _, f := range files {
		af, err := parser.ParseFile(p.Fset, f, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		r := rel(f)
		p.Files[r] = af
		p.Name = af.Name.Name
		def := map[string]bool{}
		p.Defined[r] = def
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					p.Funcs[d.Name.Name] = true
					def[d.Name.Name] = true
					continue
				}
				recv := recvName(d.Recv.List[0].Type)
				p.Methods[recv+"."+d.Name.Name] = true
				def[recv+"."+d.Name.Name] = true
				def[d.Name.Name] = true
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						p.Types[s.Name.Name] = true
						def[s.Name.Name] = true
						p.addTypeMembers(s.Name.Name, s.Type, def)
					case *ast.ValueSpec:
						for i, n := range s.Names {
							p.Values[n.Name] = true
							def[n.Name] = true
							if d.Tok == token.CONST && i < len(s.Values) {
								consts[n.Name] = s.Values[i]
							}
						}
					}
				}
			}
		}
	}
	for n, e := range consts {
		if v, ok := StringValue(e, func(id string) (ast.Expr, bool) { x, ok := consts[id]; return x, ok }); ok {
			p.Strings[n] = v
		}
	}
	return p, nil
}

func (p *Package) addTypeMembers(typ string, t ast.Expr, def map[string]bool) {
	switch t := t.(type) {
	case *ast.StructType:
		for _, f := range t.Fields.List {
			if len(f.Names) == 0 {
				n := recvName(f.Type)
				p.Fields[typ+"."+n] = true
				p.Embeds[typ] = append(p.Embeds[typ], n)
				continue
			}
			for _, n := range f.Names {
				p.Fields[typ+"."+n.Name] = true
				def[typ+"."+n.Name] = true
			}
		}
	case *ast.InterfaceType:
		for _, m := range t.Methods.List {
			for _, n := range m.Names {
				p.Methods[typ+"."+n.Name] = true
				def[typ+"."+n.Name] = true
			}
		}
	}
}

// recvName is the bare type name of a receiver or embedded field type.
func recvName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return recvName(e.X)
	case *ast.IndexExpr:
		return recvName(e.X)
	case *ast.IndexListExpr:
		return recvName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// HasMember reports whether Type has a method or field m, also through embedded types of
// the same package.
func (p *Package) HasMember(typ, m string) bool {
	seen := map[string]bool{}
	var walk func(t string) bool
	walk = func(t string) bool {
		if seen[t] {
			return false
		}
		seen[t] = true
		if p.Methods[t+"."+m] || p.Fields[t+"."+m] {
			return true
		}
		for _, e := range p.Embeds[t] {
			if walk(e) {
				return true
			}
		}
		return false
	}
	return walk(typ)
}

// Has reports whether the package declares name at the top level.
func (p *Package) Has(name string) bool {
	return p.Funcs[name] || p.Types[name] || p.Values[name]
}

// StringValue evaluates a constant string expression: literals, + and identifiers that
// lookup resolves.
func StringValue(e ast.Expr, lookup func(string) (ast.Expr, bool)) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return StringValue(e.X, lookup)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok1 := StringValue(e.X, lookup)
		b, ok2 := StringValue(e.Y, lookup)
		return a + b, ok1 && ok2
	case *ast.Ident:
		if lookup == nil {
			return "", false
		}
		x, ok := lookup(e.Name)
		if !ok {
			return "", false
		}
		return StringValue(x, lookup)
	}
	return "", false
}

// TestFunc is one func TestXxx in a _test.go file.
type TestFunc struct {
	Name string
	Pkg  string // the package clause without _test
	File string // slash path relative to the root
}

var testFuncRe = regexp.MustCompile(`(?m)^func (Test\w*)\(`)
var pkgClauseRe = regexp.MustCompile(`(?m)^package (\w+)`)

// TestFuncs lists every Go test function in the repository (worktrees under .claude and
// vendored or generated folders skipped).
func TestFuncs() ([]TestFunc, error) {
	var out []TestFunc
	root := Root()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		pkg := ""
		if m := pkgClauseRe.FindSubmatch(b); m != nil {
			pkg = strings.TrimSuffix(string(m[1]), "_test")
		}
		for _, m := range testFuncRe.FindAllSubmatch(b, -1) {
			out = append(out, TestFunc{Name: string(m[1]), Pkg: pkg, File: rel(p)})
		}
		return nil
	})
	return out, err
}
