package archlint

import (
	"fmt"
	"go/ast"
	"sort"
	"testing"
)

// hideHelpers start a child without a console window (CREATE_NO_WINDOW, or DETACHED_PROCESS
// for the game): app/deps/selfupdate hideWindow, accounts.hideLoginHost, gametest.detach
// and app.startDetached.
var hideHelpers = map[string]bool{"hideWindow": true, "hideLoginHost": true, "detach": true, "startDetached": true}

// childWindowAllowed: children that may show a window, or are hidden another way. Key:
// "file function". Each reason says why.
var childWindowAllowed = map[string]string{
	"internal/app/launch.go ExecLauncher":                      "starts the game (or Steam), a GUI program with its own window (09 §1)",
	"internal/app/firstrun.go (*App).RelocateWorkshop":         "starts the relocated RimArcho.exe, a GUI program (17 §5.1 Workshop startup)",
	"internal/surface/gui/selfupdate.go (*API).restartUpdated": "restarts the updated RimArcho.exe, a GUI program (17 §5.1 step 7)",
	"internal/surface/cli/cli.go restartTUI":                   "runs the updated archo.exe TUI in this same terminal (17 §5.1 step 7)",
	"internal/surface/cli/gui_handover.go handOver":            "`archo gui` starts the sibling RimArcho.exe or rimarcho, a GUI program",
	"internal/surface/upgradewin/upgradewin_linux.go Open":     "zenity's progress dialog, a GUI program (16 §8.1)",
	"internal/surface/upgradewin/error_linux.go ShowError":     "zenity, kdialog or notify-send show the startup error, GUI programs (16 §8.1)",
	"internal/surface/gui/gui.go var openURL":                  "hands a link to the system browser (rundll32 / open / xdg-open, GUI programs)",
	"internal/surface/gui/library.go var openDir":              "opens a folder in explorer / open / xdg-open (GUI programs)",
	"internal/surface/tui/detail.go var openURL":               "hands a link to the system browser (rundll32 / open / xdg-open, GUI programs)",
	"internal/surface/tui/shortcuts.go var openDir":            "opens a folder in explorer / open / xdg-open (GUI programs)",
	"internal/surface/tui/feedback.go var copyText":            "`clip` from the TUI shares the TUI's console; no window opens",
	"internal/app/agent.go (*App).RunAgent":                    "`archo agent run` runs Pi in the caller's terminal, which the player types into (15 §7.2); the GUI passes console and gets startAgentConsole, a console window of its own",
	"internal/app/console_other.go startWorkshopCMD":           "Linux and macOS have no console windows; an interactive run uses the caller's terminal (only when it is one), a hidden one its own process group",
	"internal/textures/todds.go (*Todds).Version":              "hidden through Todds.Prepare, which every textures.Todds sets to hideWindow (TestArchChildWindows checks)",
	"internal/textures/todds.go (*Todds).Encode":               "hidden through Todds.Prepare, which every textures.Todds sets to hideWindow (TestArchChildWindows checks)",
}

// childWindowProblems: every exec.Command / exec.CommandContext in non-test code is hidden
// in the same function (17 §5: RimArcho.exe is a GUI program, so a console child of it
// would open a console window; 16 §8).
func childWindowProblems(files []*goFile, mod string, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test || g.dir == "internal/testkit" {
			continue
		}
		for _, d := range g.f.Decls {
			var starts []ast.Node
			hidden := false
			ast.Inspect(d, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					switch f := n.Fun.(type) {
					case *ast.Ident:
						hidden = hidden || hideHelpers[f.Name]
					case *ast.SelectorExpr:
						if id, ok := f.X.(*ast.Ident); ok && g.imports()[id.Name] == "os/exec" && (f.Sel.Name == "Command" || f.Sel.Name == "CommandContext") {
							starts = append(starts, n)
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						s, ok := l.(*ast.SelectorExpr)
						if !ok || s.Sel.Name != "SysProcAttr" || i >= len(n.Rhs) {
							continue
						}
						hidden = hidden || setsNoWindow(n.Rhs[i])
					}
				}
				return true
			})
			if len(starts) == 0 || hidden {
				continue
			}
			key := g.path + " " + declName(d)
			if allow[key] != "" {
				used[key] = true
				continue
			}
			for _, s := range starts {
				out = append(out, fmt.Sprintf("17 §5: %s in %s starts a child without hiding its console window; call hideWindow (internal/app/hide_windows.go) in the same function, or list it in childWindowAllowed with the reason it shows a window", loc(g.fset.Position(s.Pos()), g.path), declName(d)))
			}
		}
		// Todds runs hidden only because its Prepare is hideWindow.
		ast.Inspect(g.f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			s, ok := cl.Type.(*ast.SelectorExpr)
			if !ok || s.Sel.Name != "Todds" {
				return true
			}
			if id, ok := s.X.(*ast.Ident); !ok || g.imports()[id.Name] != mod+"/internal/textures" {
				return true
			}
			for _, e := range cl.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Prepare" {
						if v, ok := kv.Value.(*ast.Ident); ok && hideHelpers[v.Name] {
							return true
						}
					}
				}
			}
			out = append(out, fmt.Sprintf("17 §5: %s builds a textures.Todds without Prepare: hideWindow; todds would open a console window from the GUI", loc(g.fset.Position(cl.Pos()), g.path)))
			return true
		})
	}
	sort.Strings(out)
	return out
}

// setsNoWindow reports whether a SysProcAttr value sets HideWindow or CreationFlags.
func setsNoWindow(e ast.Expr) bool {
	if u, ok := e.(*ast.UnaryExpr); ok {
		e = u.X
	}
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if k, ok := kv.Key.(*ast.Ident); ok && (k.Name == "HideWindow" || k.Name == "CreationFlags") {
				return true
			}
		}
	}
	return false
}

func TestArchChildWindows(t *testing.T) {
	t.Parallel()
	_, mod := repoRoot(t)
	used := map[string]bool{}
	report(t, childWindowProblems(repoSource(t), mod, childWindowAllowed, used))
	report(t, staleProblems("childWindowAllowed", childWindowAllowed, used))
}

func TestArchChildWindowsDetection(t *testing.T) {
	t.Parallel()
	const m = "example.com/x"
	src := `package app
import (
	"os/exec"
	"syscall"
	"example.com/x/internal/textures"
)
func hidden() { c := exec.Command("x"); hideWindow(c); c.Run() }
func flags() { c := exec.CommandContext(ctx, "x"); c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}; c.Run() }
func closure() error { run := func() error { return exec.Command("git").Run() }; return run() }
func two() { exec.Command("a").Run(); exec.CommandContext(ctx, "b").Run() }
func group() { c := exec.Command("x"); c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func detached() { startDetached(func() *exec.Cmd { return exec.Command("x") }) }
var good = &textures.Todds{Path: p, Prepare: hideWindow}
var bad = &textures.Todds{Path: p}
`
	got := childWindowProblems(synthetic(t, map[string]string{"internal/app/x.go": src}), m, nil, map[string]bool{})
	expect(t, "child windows", got, 5) // closure, two (2), group, bad Todds
	allow := map[string]string{"internal/app/x.go two": "x", "internal/app/x.go gone": "y"}
	used := map[string]bool{}
	got = childWindowProblems(synthetic(t, map[string]string{"internal/app/x.go": src}), m, allow, used)
	expect(t, "allowed child", got, 3)
	expect(t, "stale child exception", staleProblems("childWindowAllowed", allow, used), 1)
	expect(t, "tests are exempt", childWindowProblems(synthetic(t, map[string]string{"internal/app/x_test.go": src}), m, nil, map[string]bool{}), 0)
}
