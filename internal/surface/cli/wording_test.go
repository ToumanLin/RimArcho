package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §7.2: the interface uses the player's words. This package links every surface, so it
// sees every translation table, the same as the program.
func TestWording(t *testing.T) {
	for _, p := range view.WordingProblems() {
		t.Error(p)
	}
}

// Include literal view text outside the tables, such as key names and separators.
// CLI and MCP have their own vocabulary and output contracts.
func TestWordingSurfaceLiterals(t *testing.T) {
	for _, dir := range []string{"../../view", "../gui", "../tui"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Error(err)
					return true
				}
				for _, p := range view.WordingTextProblems(fmt.Sprint(fset.Position(lit.Pos())), text) {
					t.Error(p)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
