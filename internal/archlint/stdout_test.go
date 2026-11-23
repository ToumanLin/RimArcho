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

// stdioAllowed: uses of the process's stdin/stdout in packages `archo mcp` links. Key:
// "file function pkg.Name" or "file function print". Each reason says why it never runs
// inside the MCP server.
var stdioAllowed = map[string]string{
	"internal/cmds/agent.go agentKeyCmd os.Stdin":                 "`archo agent key` reads the API key from stdin; not an MCP tool (15 §7.2)",
	"internal/cmds/agent.go agentRunCmd os.Stdin":                 "`archo agent run` hands the terminal to Pi; not an MCP tool (15 §7.2)",
	"internal/cmds/agent.go agentRunCmd os.Stdout":                "`archo agent run` hands the terminal to Pi; not an MCP tool (15 §7.2)",
	"internal/app/gamedl.go startSteamCMD os.Stdin":               "a CLI game download gives SteamCMD the caller's console for the login (11 §3), only when stdin and stdout are a console (callerConsole)",
	"internal/app/gamedl.go startSteamCMD os.Stdout":              "a CLI game download gives SteamCMD the caller's console for the login (11 §3), only when stdin and stdout are a console (callerConsole)",
	"internal/app/console_other.go startWorkshopCMD os.Stdin":     "an interactive SteamCMD login on Linux uses the caller's terminal only when stdin and stdout are one (callerConsole); under MCP they are pipes and the run is hidden",
	"internal/app/console_other.go startWorkshopCMD os.Stdout":    "an interactive SteamCMD login on Linux uses the caller's terminal only when stdin and stdout are one (callerConsole); under MCP they are pipes and the run is hidden",
	"internal/app/console_other.go callerConsole os.Stdin":        "only checks whether stdin is a terminal",
	"internal/app/console_other.go callerConsole os.Stdout":       "only checks whether stdout is a terminal",
	"internal/app/console_windows.go callerConsole os.Stdin":      "only checks whether stdin is a console",
	"internal/app/console_windows.go callerConsole os.Stdout":     "only checks whether stdout is a console",
	"internal/machine/host_windows.go TerminalDrawsTUI os.Stdout": "only reads and sets the console mode before the TUI starts; MCP never calls it",
	"internal/steamworks/worker_windows.go Worker os.Stdin":       "runs in the `steam-worker` child process, whose stdin is a pipe from its parent, never in the MCP server",
}

// stdioProblems: in `archo mcp`, stdout carries only protocol messages (14 §1) and stdin
// carries the client's requests. Packages the server links never print to stdout, nor
// hand stdin or stdout to a child. reach lists the package directories it links.
func stdioProblems(files []*goFile, reach map[string]bool, allow map[string]string, used map[string]bool) []string {
	var out []string
	flag := func(g *goFile, pos token.Position, fn, what string) {
		if k := g.path + " " + fn + " " + what; allow[k] != "" {
			used[k] = true
			return
		}
		out = append(out, fmt.Sprintf("14 §1: %s in %s uses %s; `archo mcp` links %s and its stdout carries only protocol messages (stdin the client's requests): return text, write to an io.Writer the caller passes, or progress to stderr", loc(pos, g.path), fn, what, g.dir))
	}
	for _, g := range files {
		if g.test || !reach[g.dir] {
			continue
		}
		for _, r := range g.refs() {
			switch {
			case r.pkg == "os" && (r.name == "Stdout" || r.name == "Stdin"):
			case r.pkg == "fmt" && (r.name == "Print" || r.name == "Printf" || r.name == "Println") && r.call != nil:
			default:
				continue
			}
			flag(g, r.pos, r.fn, r.String())
		}
		for _, d := range g.f.Decls {
			ast.Inspect(d, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && (id.Name == "print" || id.Name == "println") {
						flag(g, g.fset.Position(c.Pos()), declName(d), id.Name)
					}
				}
				return true
			})
		}
	}
	sort.Strings(out)
	return out
}

// mcpReach lists the module's package directories that internal/surface/mcp links,
// through non-test imports.
func mcpReach(files []*goFile, mod string) map[string]bool {
	deps := map[string][]string{}
	for _, g := range files {
		if g.test {
			continue
		}
		for _, s := range g.f.Imports {
			p, _ := strconv.Unquote(s.Path.Value)
			if to, ok := strings.CutPrefix(p, mod+"/"); ok {
				deps[g.dir] = append(deps[g.dir], to)
			}
		}
	}
	reach := map[string]bool{}
	var walk func(string)
	walk = func(d string) {
		if reach[d] {
			return
		}
		reach[d] = true
		for _, to := range deps[d] {
			walk(to)
		}
	}
	walk("internal/surface/mcp")
	return reach
}

func TestArchMCPStdout(t *testing.T) {
	t.Parallel()
	_, mod := repoRoot(t)
	reach := mcpReach(repoImports(t), mod)
	for _, must := range []string{"internal/cmds", "internal/app", "internal/render", "internal/store"} {
		if !reach[must] {
			t.Errorf("mcpReach misses %s; the import walk is broken", must)
		}
	}
	used := map[string]bool{}
	report(t, stdioProblems(repoSource(t), reach, stdioAllowed, used))
	report(t, staleProblems("stdioAllowed", stdioAllowed, used))
}

func TestArchMCPStdoutDetection(t *testing.T) {
	t.Parallel()
	const m = "example.com/x"
	files := synthetic(t, map[string]string{
		"internal/surface/mcp/s.go": "package mcp\nimport \"example.com/x/internal/cmds\"\nvar _ = cmds.X\n",
		"internal/cmds/c.go":        "package cmds\nimport (\"fmt\"; \"os\"; \"example.com/x/internal/app\")\nfunc a() { fmt.Println(\"x\"); fmt.Fprintln(os.Stderr, \"ok\"); println(\"y\"); app.Run(os.Stdin, os.Stdout) }\n",
		"internal/app/a.go":         "package app\nimport (\"fmt\"; \"os\")\nfunc b() { fmt.Printf(\"x\"); w := os.Stdout; _ = w }\nfunc Run(any, any) {}\n",
		"internal/surface/tui/t.go": "package tui\nimport \"fmt\"\nfunc c() { fmt.Print(\"x\") }\n",
		"internal/app/a_test.go":    "package app\nimport \"fmt\"\nfunc d() { fmt.Print(\"x\") }\n",
	})
	reach := mcpReach(files, m)
	if !reach["internal/app"] || reach["internal/surface/tui"] {
		t.Errorf("reach = %v", reach)
	}
	expect(t, "stdio", stdioProblems(files, reach, nil, map[string]bool{}), 6)
	allow := map[string]string{"internal/cmds/c.go a println": "x", "internal/app/a.go b os.Stdout": "y", "internal/app/a.go gone fmt.Print": "z"}
	used := map[string]bool{}
	expect(t, "allowed stdio", stdioProblems(files, reach, allow, used), 4)
	expect(t, "stale stdio exception", staleProblems("stdioAllowed", allow, used), 1)
}
