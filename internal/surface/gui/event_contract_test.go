//go:build windows || cgo

package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Check real event producers against frontend subscriptions independently of test callbacks.
func TestSentEventsHaveFrontendListeners(t *testing.T) {
	listeners := map[string]bool{}
	subscription := regexp.MustCompile(`\bon\("([^"]+)"`)
	js, err := filepath.Glob("assets/js/*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range js {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range subscription.FindAllSubmatch(data, -1) {
			listeners[string(match[1])] = true
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
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
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "send" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			count++
			if !listeners[name] {
				t.Errorf("%s sends %q without a frontend listener", path, name)
			}
			return true
		})
	}
	if count == 0 {
		t.Fatal("no event producers checked")
	}
}
