package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// 16 §3.1 rule 14: the GUI never asks the player to type an archo command; where a refusal
// names one, the page offers a button or a page instead. TestUINoCLICommands reads every
// string literal in the page scripts and in this package's Go code, and the text the view
// package translates (T, Tf), and fails on "archo <command>" for a registered command.
// The TUI and CLI keep their commands: they share the view package's text only where it is
// translated for every surface.

// Literals that may name a command, by "file: command": none yet.
var noCLIAllowed = map[string]string{}

var archoWordRe = regexp.MustCompile(`\barcho ([a-z][a-z-]*)`)

func cliRoots() map[string]bool {
	roots := map[string]bool{}
	for _, c := range cmds.Commands() {
		roots[strings.Fields(c.Name)[0]] = true
	}
	return roots
}

func cliCommandIn(s string, roots map[string]bool) string {
	for _, m := range archoWordRe.FindAllStringSubmatch(s, -1) {
		if roots[m[1]] {
			return m[1]
		}
	}
	return ""
}

// jsStringRe finds JavaScript string and template literals; comments are dropped first.
var (
	jsStringRe       = regexp.MustCompile("\"(?:[^\"\\\\\\n]|\\\\.)*\"|'(?:[^'\\\\\\n]|\\\\.)*'|`(?:[^`\\\\]|\\\\.)*`")
	jsLineCommentRe  = regexp.MustCompile(`(?m)(^|[^:\\"'])//.*$`)
	jsBlockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

func TestUINoCLICommands(t *testing.T) {
	roots := cliRoots()
	if !roots["env"] || !roots["monitor"] {
		t.Fatalf("command registry not read: %v", roots)
	}
	seen := map[string]bool{}
	report := func(where, cmd string) {
		key := where + ": " + cmd
		if _, ok := noCLIAllowed[key]; ok {
			seen[key] = true
			return
		}
		t.Errorf("%s names `archo %s`: 16 §3.1 rule 14: GUI text never asks for a command; offer a button or a page", where, cmd)
	}
	err := fs.WalkDir(assets, "assets/js", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return err
		}
		b, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		src := jsBlockCommentRe.ReplaceAllString(string(b), "")
		src = jsLineCommentRe.ReplaceAllString(src, "$1")
		for _, lit := range jsStringRe.FindAllString(src, -1) {
			if c := cliCommandIn(lit, roots); c != "" {
				report(strings.TrimPrefix(path, "assets/"), c)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	goStrings := func(dir string, translatedOnly bool) {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			check := func(lit *ast.BasicLit) {
				if lit == nil || lit.Kind != token.STRING {
					return
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					return
				}
				if c := cliCommandIn(s, roots); c != "" {
					report(filepath.ToSlash(file), c)
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if !translatedOnly {
					if lit, ok := n.(*ast.BasicLit); ok {
						check(lit)
					}
					return true
				}
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "T" || id.Name == "Tf") {
					lit, _ := call.Args[0].(*ast.BasicLit)
					check(lit)
				}
				return true
			})
		}
	}
	goStrings(".", false)
	goStrings(filepath.Join("..", "..", "view"), true)
	for key := range noCLIAllowed {
		if !seen[key] {
			t.Errorf("noCLIAllowed entry %q no longer matches; remove it", key)
		}
	}
}

func TestUINoCLICommandsDetection(t *testing.T) {
	roots := cliRoots()
	for s, want := range map[string]string{
		"no games yet (archo env add)":          "env",
		"run an archo command":                  "",
		"Archo Monitor (archo monitor install)": "monitor",
		"archo ":                                "",
	} {
		if got := cliCommandIn(s, roots); got != want {
			t.Errorf("cliCommandIn(%q) = %q, want %q", s, got, want)
		}
	}
}
