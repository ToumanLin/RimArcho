package archlint

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// importRule forbids imports between package directories. bad reports whether file (in
// package directory from) may not import package directory to; both are module-relative.
type importRule struct {
	spec, why string
	bad       func(from, file, to string) bool
}

var surfaceDirs = "internal/surface"

var importRules = []importRule{
	{"17 §2, 00a §2", "only surfaces (internal/surface/**) and cmd/** import surface packages", func(from, _, to string) bool {
		return under(to, surfaceDirs) && !under(from, surfaceDirs) && !under(from, "cmd")
	}},
	{"16 §1, 17 §2", "only surfaces and internal/cmdline import internal/view; view models serve GUI and TUI", func(from, _, to string) bool {
		return to == "internal/view" && !under(from, surfaceDirs) && !under(from, "cmd") && from != "internal/cmdline"
	}},
	{"17 §5.1", "internal/selfupdate is independent of other packages and imports no other internal package", func(from, _, to string) bool {
		return from == "internal/selfupdate" && under(to, "internal")
	}},
	{"17 §2, 00a §2", "internal/store is the facts layer: it imports no application, view, command or surface package, nor the analysis and identity layers above it", func(from, _, to string) bool {
		if from != "internal/store" {
			return false
		}
		for _, up := range []string{"internal/app", "internal/view", "internal/cmds", "internal/cmdline", surfaceDirs, "internal/analyze", "internal/identity", "internal/order", "internal/logs"} {
			if under(to, up) {
				return true
			}
		}
		return false
	}},
	{"13 §1.1, 17 §2", "internal/app is below the tool layer and the surfaces: it never imports cmds, cmdline, view or a surface", func(from, _, to string) bool {
		if from != "internal/app" {
			return false
		}
		return to == "internal/cmds" || to == "internal/cmdline" || to == "internal/view" || under(to, surfaceDirs)
	}},
	{"11 §4, 15 §7.2", "the tool layer (internal/cmds) and MCP never import accounts, secret or lanzou: account sessions and keys stay out of agent-reachable code", func(from, _, to string) bool {
		if from != "internal/cmds" && from != "internal/surface/mcp" {
			return false
		}
		return to == "internal/accounts" || to == "internal/secret" || to == "internal/lanzou"
	}},
	{"15 §7.2, 02 §2", "internal/secret (the Archo Agent API key) is imported only by internal/app/agent.go and internal/app/uninstall.go", func(_, file, to string) bool {
		return to == "internal/secret" && file != "internal/app/agent.go" && file != "internal/app/uninstall.go"
	}},
	{"14 §1, 13 §1.1, 17 §2", "MCP depends only on internal/cmds: each tool builds a CLI argv and returns cmds' text", func(from, _, to string) bool {
		return from == "internal/surface/mcp" && under(to, "internal") && to != "internal/cmds"
	}},
	{"17 §2, §5", "product code never imports spikes/ or scripts/ (excluded from releases)", func(_, _, to string) bool {
		return under(to, "spikes") || under(to, "scripts")
	}},
}

// importAllowed: existing imports that break a rule. Key "from -> to".
var importAllowed = map[string]string{
	"internal/store -> internal/analyze":         "existing; open question: 00a §2 puts Facts (SQLite) below Analysis, but store reads and writes analyze's fact and finding types (store/analysis.go); move the types down or redraw 00a",
	"internal/store -> internal/identity":        "existing; open question: 00a §2 puts Facts below Identity, but store persists identity's reconciliation types; move the types down or redraw 00a",
	"internal/surface/mcp -> internal/app":       "existing; open question: MCP opens the App, switches environments per call and waits for analysis (14 §1) itself instead of through cmds",
	"internal/surface/mcp -> internal/subscribe": "existing; open question: MCP uses subscribe's types for manage_subscriptions instead of a cmds wrapper",
	"internal/surface/mcp -> internal/buildinfo": "existing; open question: the server reports the release version (14 §1); a cmds accessor would remove it",
}

// importProblems checks every non-test import between the module's packages.
func importProblems(files []*goFile, mod string, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test {
			continue
		}
		for _, s := range g.f.Imports {
			p, _ := strconv.Unquote(s.Path.Value)
			to, ok := strings.CutPrefix(p, mod+"/")
			if !ok {
				continue
			}
			for _, r := range importRules {
				if !r.bad(g.dir, g.path, to) {
					continue
				}
				key := g.dir + " -> " + to
				if _, ok := allow[key]; ok {
					used[key] = true
					continue
				}
				out = append(out, fmt.Sprintf("%s: %s imports %s; %s", r.spec, loc(g.fset.Position(s.Pos()), g.path), to, r.why))
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestArchImports(t *testing.T) {
	t.Parallel()
	_, mod := repoRoot(t)
	used := map[string]bool{}
	report(t, importProblems(repoImports(t), mod, importAllowed, used))
	report(t, staleProblems("importAllowed", importAllowed, used))
}

// cgoProblems: no import "C" anywhere (17 §1). RimArcho's own packages are pure Go; only the
// Linux GUI binary, rimarcho, is built with CGO_ENABLED=1, for Wails' Linux code (decision D7).
func cgoProblems(files []*goFile) []string {
	var out []string
	for _, g := range files {
		for _, s := range g.f.Imports {
			if s.Path.Value == `"C"` {
				out = append(out, fmt.Sprintf(`17 §1: %s imports "C"; RimArcho's packages stay pure Go; cgo is allowed only inside Wails, for the Linux GUI binary (D7)`, loc(g.fset.Position(s.Pos()), g.path)))
			}
		}
	}
	return out
}

func TestArchNoCgo(t *testing.T) {
	t.Parallel()
	report(t, cgoProblems(repoImports(t)))
}

func TestArchImportsDetection(t *testing.T) {
	t.Parallel()
	const m = "example.com/x"
	imp := func(paths ...string) string {
		s := "package p\nimport (\n"
		for _, p := range paths {
			s += strconv.Quote(p) + "\n"
		}
		return s + ")\n"
	}
	for _, tc := range []struct {
		file, src string
		want      int
	}{
		{"internal/app/a.go", imp(m+"/internal/store", m+"/internal/secret", "os"), 1}, // secret outside agent.go/uninstall.go
		{"internal/app/agent.go", imp(m + "/internal/secret"), 0},
		{"internal/app/a_test.go", imp(m + "/internal/surface/gui"), 0}, // tests are not layered
		{"internal/app/b.go", imp(m+"/internal/cmds", m+"/internal/surface/gui", m+"/internal/view"), 5},
		{"internal/store/s.go", imp(m+"/internal/analyze", m+"/internal/app", m+"/internal/scan"), 2},
		{"internal/selfupdate/u.go", imp(m+"/internal/env", "net/http"), 1},
		{"internal/cmds/c.go", imp(m+"/internal/accounts", m+"/internal/lanzou", m+"/internal/app"), 2},
		{"internal/surface/mcp/s.go", imp(m+"/internal/cmds", m+"/internal/app", m+"/internal/secret"), 4},
		{"internal/surface/tui/t.go", imp(m+"/internal/view", m+"/internal/surface/gui", m+"/spikes/x"), 1},
		{"internal/cmdline/c.go", imp(m + "/internal/view"), 0},
		{"cmd/archo/main.go", imp(m+"/internal/surface/cli", m+"/scripts/vm"), 1},
		{"internal/render/r.go", imp(m + "/internal/view"), 1},
	} {
		got := importProblems(synthetic(t, map[string]string{tc.file: tc.src}), m, nil, map[string]bool{})
		expect(t, tc.file, got, tc.want)
	}
	allow := map[string]string{"internal/store -> internal/analyze": "x", "internal/store -> internal/order": "y"}
	used := map[string]bool{}
	got := importProblems(synthetic(t, map[string]string{"internal/store/s.go": imp(m + "/internal/analyze")}), m, allow, used)
	expect(t, "allowed import", got, 0)
	expect(t, "stale import exception", staleProblems("importAllowed", allow, used), 1)
	got = importProblems(synthetic(t, map[string]string{"internal/store/s.go": imp(m + "/internal/app")}), m, nil, map[string]bool{})
	if len(got) != 1 || !strings.HasPrefix(got[0], "17 §2, 00a §2: internal/store/s.go:3 imports internal/app;") {
		t.Errorf("wrong message: %v", got)
	}
	expect(t, "cgo", cgoProblems(synthetic(t, map[string]string{"internal/x/c.go": "package x\nimport \"C\"\n"})), 1)
}
