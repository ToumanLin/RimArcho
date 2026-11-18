package archlint

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// exitAllowed: internal code that ends the process. Key: "file function pkg.Name".
var exitAllowed = map[string]string{}

var exitFuncs = map[string]map[string]bool{"os": {"Exit": true}, "log": {"Fatal": true, "Fatalf": true, "Fatalln": true}, "syscall": {"Exit": true}}

// exitProblems: only cmd/ ends the process; internal code returns the exit code (13 §3.3)
// so that MCP, the TUI and tests keep running and deferred cleanup runs.
func exitProblems(files []*goFile, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test || !under(g.dir, "internal") {
			continue
		}
		for _, r := range g.refs() {
			if !exitFuncs[r.pkg][r.name] || r.call == nil {
				continue
			}
			if k := writeKey(g, r); allow[k] != "" {
				used[k] = true
				continue
			}
			out = append(out, fmt.Sprintf("13 §3.3: %s in %s calls %s; internal code returns an exit code and only cmd/ exits (MCP and the TUI share the process)", loc(r.pos, g.path), r.fn, r))
		}
	}
	sort.Strings(out)
	return out
}

func TestArchExit(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	report(t, exitProblems(repoSource(t), exitAllowed, used))
	report(t, staleProblems("exitAllowed", exitAllowed, used))
}

// plainTextDirs print the CLI and MCP text, which has no ANSI escapes (13 §3.1 rule 8).
var plainTextDirs = map[string]bool{"internal/cmds": true, "internal/render": true, "internal/surface/mcp": true, "internal/surface/cli": true}

// ansiAllowed: escape sequences in plain-text packages. Key: "file function".
var ansiAllowed = map[string]string{
	"internal/surface/cli/upgrade.go (*upgradeReport).finish": "clears the database-upgrade progress line on stderr, only when stderr is a terminal (u.tty); never in command output",
}

// funcAt returns the declaration enclosing pos.
func funcAt(f *ast.File, pos token.Pos) string {
	for _, d := range f.Decls {
		if d.Pos() <= pos && pos < d.End() {
			return declName(d)
		}
	}
	return "package level"
}

func ansiProblems(files []*goFile, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test || !plainTextDirs[g.dir] {
			continue
		}
		ast.Inspect(g.f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(s, "\x1b[") {
				return true
			}
			fn := funcAt(g.f, lit.Pos())
			if k := g.path + " " + fn; allow[k] != "" {
				used[k] = true
				return true
			}
			out = append(out, fmt.Sprintf("13 §3.1 rule 8: %s in %s has an ANSI escape; CLI and MCP text has no colours or cursor codes, in a terminal too", loc(g.fset.Position(lit.Pos()), g.path), fn))
			return true
		})
	}
	sort.Strings(out)
	return out
}

func TestArchNoANSI(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	report(t, ansiProblems(repoSource(t), ansiAllowed, used))
	report(t, staleProblems("ansiAllowed", ansiAllowed, used))
}

func TestArchOutputDetection(t *testing.T) {
	t.Parallel()
	exit := `package p
import ("log"; "os")
func a() { os.Exit(1) }
func b() { log.Fatalf("x") }
var c = os.Exit
`
	expect(t, "exit internal", exitProblems(synthetic(t, map[string]string{"internal/app/e.go": exit}), nil, map[string]bool{}), 2)
	expect(t, "exit cmd", exitProblems(synthetic(t, map[string]string{"cmd/archo/main.go": exit}), nil, map[string]bool{}), 0)
	allow := map[string]string{"internal/app/e.go a os.Exit": "x", "internal/app/e.go z os.Exit": "y"}
	used := map[string]bool{}
	expect(t, "allowed exit", exitProblems(synthetic(t, map[string]string{"internal/app/e.go": exit}), allow, used), 1)
	expect(t, "stale exit exception", staleProblems("exitAllowed", allow, used), 1)

	ansi := "package p\nconst red = \"\\x1b[31m\"\nfunc f() string { return \"\\033[0m\" + `\x1b[K` + \"\\u001b[1m\" + \"plain [x]\" }\n"
	expect(t, "ansi cmds", ansiProblems(synthetic(t, map[string]string{"internal/cmds/a.go": ansi}), nil, map[string]bool{}), 4)
	expect(t, "ansi tui", ansiProblems(synthetic(t, map[string]string{"internal/surface/tui/a.go": ansi}), nil, map[string]bool{}), 0)
	allow = map[string]string{"internal/render/a.go f": "x"}
	used = map[string]bool{}
	expect(t, "allowed ansi", ansiProblems(synthetic(t, map[string]string{"internal/render/a.go": ansi}), allow, used), 1)
	expect(t, "ansi exception used", staleProblems("ansiAllowed", allow, used), 0)
}
