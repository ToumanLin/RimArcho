package archlint

import (
	"go/ast"
	"go/token"
	"slices"
	"testing"
)

// Detect a slow call in the lexical scope of an API mutex acquisition (16 §8.4).
// The GUI uses direct Lock and Unlock calls or a deferred Unlock.
func guiLockedActions(g *goFile) []token.Pos {
	var violations []token.Pos
	if g.dir != "internal/surface/gui" || g.test {
		return violations
	}
	ast.Inspect(g.f, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		held := false
		for _, stmt := range block.List {
			switch guiMutexCall(stmt) {
			case "Lock":
				held = true
			case "Unlock":
				held = false
			}
			if held {
				ast.Inspect(stmt, func(n ast.Node) bool {
					if _, ok := n.(*ast.FuncLit); ok {
						return false
					}
					if call, ok := n.(*ast.CallExpr); ok && guiSlowAction(call) {
						violations = append(violations, call.Pos())
					}
					return true
				})
			}
		}
		return true
	})
	return slices.Compact(violations)
}

func guiMutexCall(stmt ast.Stmt) string {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return ""
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	mu, ok := method.X.(*ast.SelectorExpr)
	if ok && mu.Sel.Name == "mu" {
		if recv, ok := mu.X.(*ast.Ident); ok && recv.Name == "s" {
			return method.Sel.Name
		}
	}
	return ""
}

func guiSlowAction(call *ast.CallExpr) bool {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch method.Sel.Name {
	case "Run", "RunAsking", "RunMoving", "DryRun":
		pkg, ok := method.X.(*ast.Ident)
		return ok && pkg.Name == "view"
	case "Launch", "LaunchWith", "LaunchPack", "LaunchPackWith", "SaveWith",
		"BackupSaves", "TrashSaves", "CopySave", "CopySaveTo", "RenameSave":
		return true
	}
	return false
}

func TestArchGUIFileActionsOutsideMainLock(t *testing.T) {
	t.Parallel()
	for _, g := range repoSource(t) {
		for _, pos := range guiLockedActions(g) {
			t.Errorf("%s: run game and save file actions on a detached session outside s.mu (16 §8.4)", g.fset.Position(pos))
		}
	}
}

func TestArchDetectsGUIFileActionsUnderMainLock(t *testing.T) {
	t.Parallel()
	code := `package gui
func (s *API) run() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if yes { view.RunAsking(s.a, what) }
	s.a.BackupSaves(saves, yes, token)
}
func (s *API) copy() {
	s.mu.Lock()
	s.a.CopySave(save)
	s.mu.Unlock()
	view.DryRun(r, what)
}
func (s *API) actions() {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	r.TrashSaves(saves, yes, token)
}`
	files := synthetic(t, map[string]string{"internal/surface/gui/api.go": code})
	if got := guiLockedActions(files[0]); len(got) != 3 {
		t.Fatalf("found %d calls, want 3", len(got))
	}
}
