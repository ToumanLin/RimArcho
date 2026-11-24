package archlint

import (
	"go/ast"
	"sort"
	"strings"
	"testing"
)

// SteamCMD's password and Steam Guard code (11 §3 "Password prompt") travel as
// app.SteamSecret from the surface that read it to SteamCMD's console input, nowhere else.
// secretFiles are the only non-test files that may name the type or build one, each with its
// part; secretBuffer the only ones that may reach its UTF-16 buffer (the field b). None of
// them may import a logger, the operation log or the store.
var secretFiles = map[string]string{
	"internal/app/steamsecret.go":             "the type, its constructors and wiping",
	"internal/app/steamprompts.go":            "asks the prompter and types the answer, then wipes it",
	"internal/app/steamconsole_windows.go":    "writes it as console key input",
	"internal/app/steamconsole_other.go":      "no console to type into",
	"internal/surface/gui/steamprompt.go":     "the window's answer to the waiting run",
	"internal/surface/cli/steampw_windows.go": "the CLI's no-echo read",
	"internal/surface/cli/steampw_other.go":   "no CLI read",
}

var secretBuffer = map[string]bool{
	"internal/app/steamsecret.go":          true,
	"internal/app/steamconsole_windows.go": true,
}

var secretForbiddenImports = []string{"log", "log/slog", "/internal/oplog", "/internal/store", "/internal/perftrace", "/internal/logs"}

func steamSecretProblems(files []*goFile) []string {
	var out []string
	for _, g := range files {
		if g.test {
			continue
		}
		uses, buffer := false, false
		ast.Inspect(g.f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if strings.Contains(x.Name, "SteamSecret") {
					uses = true
				}
			case *ast.SelectorExpr:
				if strings.Contains(x.Sel.Name, "SteamSecret") {
					uses = true
				}
				if x.Sel.Name == "b" {
					if id, ok := x.X.(*ast.Ident); ok && (id.Name == "s" || id.Name == "secret") {
						buffer = true
					}
				}
			}
			return true
		})
		if !uses {
			continue
		}
		if _, ok := secretFiles[g.path]; !ok {
			out = append(out, g.path+": names app.SteamSecret outside the files allowed to carry a SteamCMD password (11 §3)")
			continue
		}
		if buffer && !secretBuffer[g.path] {
			out = append(out, g.path+": reads a SteamSecret's buffer; only steamsecret.go and the console typist may (11 §3)")
		}
		for p := range g.imports() {
			for _, bad := range secretForbiddenImports {
				if p == bad || strings.HasSuffix(p, bad) {
					out = append(out, g.path+": carries a SteamCMD password and imports "+p+"; keep it out of logs and stores (11 §3)")
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestArchSteamSecret(t *testing.T) {
	t.Parallel()
	report(t, steamSecretProblems(repoSource(t)))
}

func TestArchSteamSecretDetection(t *testing.T) {
	t.Parallel()
	got := steamSecretProblems(synthetic(t, map[string]string{
		"internal/app/steamsecret.go":         "package app\ntype SteamSecret struct{ b []uint16 }\nfunc (s *SteamSecret) Wipe() { clear(s.b) }\n",
		"internal/view/x.go":                  "package view\nimport \"x/internal/app\"\nvar s *app.SteamSecret\n",
		"internal/surface/gui/steamprompt.go": "package gui\nimport (\"log\"; \"x/internal/app\")\nfunc f(s *app.SteamSecret) { log.Print(s.b) }\n",
	}))
	// view names it; the window's file reads the buffer and imports log
	expect(t, "steam secret", got, 3)
}
