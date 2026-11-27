package archlint

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// opPrimitives are the functions that change the player's files and must write the operation
// log (13 §1.3, 02 §1): file → function names (methods by name). A new write path of the
// same kind joins this table, or it silently skips the log.
var opPrimitives = map[string][]string{
	"internal/app/trash.go":          {"toTrash", "recycleTargetIn", "deleteChecked", "deleteTree"},
	"internal/app/subscribe.go":      {"RemoveSubscriptionsWith"},
	"internal/app/workshopsub.go":    {"Workshop", "resubscribe"},
	"internal/app/loadout.go":        {"writeGame"},
	"internal/app/linkdeployment.go": {"applyLinks"},
	"internal/app/linkhealth.go":     {"applyLinkRepair"},
	"internal/app/saveactions.go":    {"RenameSave", "CopySave", "BackupSaves", "TrashSaves"},
	"internal/app/savelaunchcopy.go": {"launchCopy"},
	"internal/app/selfupdate.go":     {"installSelfUpdate", "resumeSelfSwap"},
	"internal/app/uninstall.go":      {"Uninstall", "uninstallItem"},
	"internal/app/launch.go":         {"LaunchWith"},
	"internal/app/pack.go":           {"LaunchPackWith", "launchPackWith"},
	"internal/app/planfollow.go":     {"syncCollection"},
}

// permanentDeletes are the functions that delete a player's folder for good, skipping the
// recycle bin, each with the only function allowed to call it: a logged primitive of
// opPrimitives. A new caller (another quick-delete path) must go through that primitive, or
// it deletes the player's Mods with no row.
var permanentDeletes = map[string]string{
	"removeModTree": "internal/app/trash.go deleteTree",
}

// opLogFile holds the App's log helpers: a primitive calls one of them or App.Ops itself.
const opLogFile = "internal/app/oplog.go"

// opLogProblems: every primitive's body reaches the log (a selector .Ops, a helper declared
// in opLogFile, or an oplog handle method), and only Archo's launcher sets the agent origin.
func opLogProblems(files []*goFile) []string {
	helpers := map[string]bool{}
	for _, g := range files {
		if g.path != opLogFile {
			continue
		}
		for _, d := range g.f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				helpers[fd.Name.Name] = true
			}
		}
	}
	found := map[string]bool{}
	var out []string
	for _, g := range files {
		if g.test {
			continue
		}
		want := opPrimitives[g.path]
		for _, d := range g.f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !contains(want, fd.Name.Name) {
				continue
			}
			found[g.path+" "+fd.Name.Name] = true
			logs := false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				s, ok := n.(*ast.SelectorExpr)
				if ok && (s.Sel.Name == "Ops" || helpers[s.Sel.Name] || s.Sel.Name == "begin" || s.Sel.Name == "end" || s.Sel.Name == "parent") {
					logs = true
				}
				return !logs
			})
			if !logs {
				out = append(out, fmt.Sprintf("13 §1.3: %s %s changes the player's files but writes no operation-log row (App.Ops or a helper in %s)", g.path, fd.Name.Name, opLogFile))
			}
		}
		// The agent origin is set (SetOrigin, Setenv, EnvVar+"=", an env map) only by Archo's launcher.
		if g.dir != "internal/oplog" && g.path != "internal/app/agent.go" {
			isSel := func(e ast.Expr, name string) bool {
				s, ok := e.(*ast.SelectorExpr)
				id, ok2 := func() (*ast.Ident, bool) {
					if !ok {
						return nil, false
					}
					i, ok := s.X.(*ast.Ident)
					return i, ok
				}()
				return ok && ok2 && id.Name == "oplog" && s.Sel.Name == name
			}
			setsAgent := func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					if s, ok := n.Fun.(*ast.SelectorExpr); ok && (s.Sel.Name == "SetOrigin" || s.Sel.Name == "Setenv") {
						for _, a := range n.Args {
							if isSel(a, "Agent") {
								return true
							}
						}
					}
				case *ast.BinaryExpr:
					if b, ok := n.X.(*ast.BinaryExpr); ok && isSel(b.X, "EnvVar") && isSel(n.Y, "Agent") {
						return true
					}
				case *ast.KeyValueExpr:
					return isSel(n.Key, "EnvVar") && isSel(n.Value, "Agent")
				}
				return false
			}
			ast.Inspect(g.f, func(n ast.Node) bool {
				if setsAgent(n) {
					out = append(out, fmt.Sprintf("13 §1.3: %s sets the agent origin; only Archo's launcher (internal/app/agent.go) does", loc(g.fset.Position(n.Pos()), g.path)))
					return false
				}
				if b, ok := n.(*ast.BasicLit); ok {
					if v, err := strconv.Unquote(b.Value); err == nil && strings.Contains(v, "ARCHO_ORIGIN") {
						out = append(out, fmt.Sprintf("13 §1.3: %s spells ARCHO_ORIGIN; use oplog.EnvVar, and only Archo's launcher sets it to agent", loc(g.fset.Position(b.Pos()), g.path)))
					}
				}
				return true
			})
		}
	}
	for _, g := range files {
		if g.test {
			continue
		}
		for _, d := range g.f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch f := c.Fun.(type) {
				case *ast.Ident:
					name = f.Name
				case *ast.SelectorExpr:
					name = f.Sel.Name
				}
				if want, ok := permanentDeletes[name]; ok && g.path+" "+fd.Name.Name != want {
					out = append(out, fmt.Sprintf("13 §1.3: %s %s calls %s, which deletes a player's folder for good with no operation-log row; call %s instead", loc(g.fset.Position(c.Pos()), g.path), fd.Name.Name, name, want))
				}
				return true
			})
		}
	}
	for file, fns := range opPrimitives {
		for _, fn := range fns {
			if !found[file+" "+fn] && len(files) > 0 && hasFile(files, file) {
				out = append(out, fmt.Sprintf("13 §1.3: %s has no function %s; update opPrimitives in internal/archlint", file, fn))
			}
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func hasFile(files []*goFile, path string) bool {
	for _, g := range files {
		if g.path == path {
			return true
		}
	}
	return false
}

func TestArchOpLog(t *testing.T) {
	t.Parallel()
	report(t, opLogProblems(repoSource(t)))
}

func TestArchOpLogDetection(t *testing.T) {
	t.Parallel()
	got := opLogProblems(synthetic(t, map[string]string{
		"internal/app/oplog.go":       "package app\nfunc (a *App) logTrash() { a.Ops.Add() }\nfunc (a *App) logDelete() { a.Ops.Add() }\n",
		"internal/app/trash.go":       "package app\nfunc (a *App) toTrash() { a.logTrash() }\nfunc (a *App) recycleTargetIn() { trash() }\nfunc (a *App) deleteChecked() { a.logDelete() }\nfunc (a *App) deleteTree() { removeModTree(); a.logDelete() }\n",
		"internal/app/subscribe.go":   "package app\nfunc (a *App) RemoveSubscriptionsWith() { a.Ops.Add() }\nfunc (a *App) quick() { removeModTree() }\n",
		"internal/app/workshopsub.go": "package app\nimport \"x/internal/oplog\"\nvar o = map[string]string{oplog.EnvVar: oplog.Agent}\nfunc (a *App) Workshop() { a.Ops.SetOrigin(oplog.Agent) }\n",
		"internal/app/agent.go":       "package app\nimport \"x/internal/oplog\"\nvar o = oplog.EnvVar + \"=\" + oplog.Agent\n",
		"internal/view/x.go":          "package view\nimport \"x/internal/oplog\"\nvar label = map[string]string{oplog.Agent: \"Archo\"}\n",
		"internal/cmds/x.go":          "package cmds\nvar e = \"ARCHO_ORIGIN=agent\"\n",
	}))
	// recycleTargetIn logs nothing; quick calls removeModTree outside deleteTree; the agent
	// origin set twice outside agent.go (a reading in view is fine); ARCHO_ORIGIN spelled;
	// resubscribe missing.
	expect(t, "op log", got, 6)
}

// TestArchOpRowNilSafe: an action's pending row (opRun.opRow) is read and written only in
// oplog.go, whose methods all accept a nil run; elsewhere it goes through opRun.note. A run is
// nil whenever the App has no operation log, and 1.1.0's window update panicked on a direct
// o.row.Target after the download (17 §5.1, 13 §1.3).
func TestArchOpRowNilSafe(t *testing.T) {
	t.Parallel()
	root, _ := repoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "internal", "app", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, f := range files {
		rel := filepath.ToSlash(strings.TrimPrefix(f, root+string(filepath.Separator)))
		if rel == opLogFile || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, ".opRow") {
				problems = append(problems, fmt.Sprintf("13 §1.3: %s:%d uses opRun.opRow directly; use o.note(func(r *oplog.Row) {…}), which skips a nil run", rel, i+1))
			}
		}
	}
	report(t, problems)
}
