package archlint

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// Function size (17 §4 "Function size"): an agent fixes a function by reading all of it, so
// a function stays small enough to read whole. Limits for production code under internal/
// and cmd/: cyclomatic complexity (1 + each if, for, range, case, comm clause, && and ||, as
// gocyclo counts, function literals included) and lines from `func` to the closing brace.
const (
	maxComplexity = 40
	maxFuncLines  = 150
)

// funcSize is one function's measure.
type funcSize struct{ complexity, lines int }

// funcSizes measures every function declaration of the production files, keyed
// "internal/app/uninstall.go (*App).planUninstall".
func funcSizes(files []*goFile) map[string]funcSize {
	out := map[string]funcSize{}
	for _, g := range files {
		if g.test {
			continue
		}
		for _, d := range g.f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			out[g.path+" "+sizeKey(fd)] = funcSize{
				complexity: complexity(fd),
				lines:      g.fset.Position(fd.End()).Line - g.fset.Position(fd.Pos()).Line + 1,
			}
		}
	}
	return out
}

// sizeKey is "Name", "(T).Name" or "(*T).Name".
func sizeKey(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	star := ""
	if s, ok := t.(*ast.StarExpr); ok {
		star, t = "*", s.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	name := "?"
	if id, ok := t.(*ast.Ident); ok {
		name = id.Name
	}
	return "(" + star + name + ")." + fd.Name.Name
}

func complexity(fd *ast.FuncDecl) int {
	n := 1
	ast.Inspect(fd.Body, func(x ast.Node) bool {
		switch x := x.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			n++
		case *ast.CaseClause:
			if x.List != nil { // not default
				n++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				n++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

func (s funcSize) over() bool { return s.complexity > maxComplexity || s.lines > maxFuncLines }

// TestArchFunctionSize: no production function is over the limits. RIMARCHO_FUNCSIZE_PRINT=1
// prints the largest functions, to see how close they are.
func TestArchFunctionSize(t *testing.T) {
	t.Parallel()
	sizes := funcSizes(repoSource(t))
	if os.Getenv("RIMARCHO_FUNCSIZE_PRINT") != "" {
		keys := make([]string, 0, len(sizes))
		for k := range sizes {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return sizes[keys[i]].complexity > sizes[keys[j]].complexity })
		var b strings.Builder
		for _, k := range keys[:min(30, len(keys))] {
			fmt.Fprintf(&b, "%s %d %d\n", k, sizes[k].complexity, sizes[k].lines)
		}
		t.Logf("largest by complexity:\n%s", b.String())
	}
	var problems []string
	for k, s := range sizes {
		if s.over() {
			problems = append(problems, fmt.Sprintf("%s: complexity %d, %d lines; the limits are %d and %d. Split it into functions that each do one step (17 §4 \"Function size\")", k, s.complexity, s.lines, maxComplexity, maxFuncLines))
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}
