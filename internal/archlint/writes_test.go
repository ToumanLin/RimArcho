package archlint

import (
	"fmt"
	"go/ast"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// writeFuncs are the standard library calls that change the file system.
var writeFuncs = map[string]map[string]bool{
	"os": {"WriteFile": true, "Create": true, "CreateTemp": true, "MkdirTemp": true, "OpenFile": true,
		"Remove": true, "RemoveAll": true, "Rename": true, "Mkdir": true, "MkdirAll": true, "Chtimes": true,
		"Chmod": true, "Chown": true, "Lchown": true, "Symlink": true, "Link": true, "Truncate": true},
	"io/ioutil": {"WriteFile": true, "TempFile": true, "TempDir": true},
}

var openWriteFlags = map[string]bool{"O_WRONLY": true, "O_RDWR": true, "O_CREATE": true, "O_APPEND": true, "O_TRUNC": true}

// writes reports whether r is a call that writes: os.OpenFile counts only with a write flag
// (a flag RimArcho cannot read statically counts as one).
func writes(r ref) bool {
	if !writeFuncs[r.pkg][r.name] || r.call == nil {
		return false
	}
	if r.pkg != "os" || r.name != "OpenFile" || len(r.call.Args) < 2 {
		return true
	}
	write, unknown := false, false
	ast.Inspect(r.call.Args[1], func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if openWriteFlags[n.Sel.Name] {
				write = true
			} else if n.Sel.Name != "O_RDONLY" && n.Sel.Name != "O_EXCL" && n.Sel.Name != "O_SYNC" {
				unknown = true
			}
			return false
		case *ast.Ident:
			unknown = true
		}
		return true
	})
	return write || unknown
}

// writeKey names a write site for the exception lists: "file function pkg.Name". Line
// numbers are left out so that edits around a listed site do not make it stale.
func writeKey(g *goFile, r ref) string { return g.path + " " + r.fn + " " + r.String() }

// readOnlyDirs read the player's files and RimArcho's data but never write (17 §2, 02 §1).
var readOnlyDirs = map[string]string{
	"internal/identity": "reconciliation, Lineage and change events are pure logic (17 §2)",
	"internal/env":      "environments and read-only ModsConfig parsing (17 §2)",
	"internal/vdf":      "Valve KeyValues reader",
	"internal/scan":     "scanning reads Mods; the scan pipeline that stores results is in app",
	"internal/content":  "effective content",
	"internal/analyze":  "Def facts, Patch simulation and findings; results are stored by app and store",
	"internal/logs":     "Player.log parsing and clustering",
	"internal/order":    "constraint graph and sorting",
	"internal/deps":     "discovery does not download or modify files (17 §2.1); installing is app.InstallDep",
	"internal/savefile": "save reading; save actions are in app/saveactions.go (12 §4)",
	"internal/looks":    "the looks index reads Mods; its header cache is written by fsscan.Cache, and _Archo_Looks by app (21 §2, §4)",
}

// readOnlyAllowed: existing writes in read-only code. Key: writeKey.
var readOnlyAllowed = map[string]string{
	"internal/app/savechoice.go (*App).CopySaveTo os.MkdirAll": "existing; open question: CopySaveTo (12 §4.4) creates the target Saves folder and copies a save, a write that lives in savechoice.go instead of saveactions.go; move it there",
}

// readOnly reports whether a file is read-only code: a read-only package, or a save file
// of app other than saveactions.go.
func readOnly(g *goFile) bool {
	if _, ok := readOnlyDirs[g.dir]; ok {
		return true
	}
	base := path.Base(g.path)
	return g.dir == "internal/app" && strings.HasPrefix(base, "save") && base != "saveactions.go"
}

func readOnlyProblems(files []*goFile, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test || !readOnly(g) {
			continue
		}
		for _, r := range g.refs() {
			if !writes(r) {
				continue
			}
			if k := writeKey(g, r); allow[k] != "" {
				used[k] = true
				continue
			}
			out = append(out, fmt.Sprintf("17 §2, 02 §1: %s in %s: %s writes to disk, but %s is read-only code; writes belong in internal/app (02 §1 lists each)", loc(r.pos, g.path), r.fn, r, g.dir))
		}
		if g.dir == "internal/deps" {
			for _, s := range g.f.Imports {
				if p, _ := strconv.Unquote(s.Path.Value); p == "net/http" {
					out = append(out, fmt.Sprintf("17 §2.1: %s imports net/http; dependency discovery does not use the network (downloads are app.InstallDep)", loc(g.fset.Position(s.Pos()), g.path)))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestArchReadOnlyPackages(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	report(t, readOnlyProblems(repoSource(t), readOnlyAllowed, used))
	report(t, staleProblems("readOnlyAllowed", readOnlyAllowed, used))
}

// surfaceWritesAllowed: files surfaces and cmds write themselves. Each reason says what is
// written. Key: writeKey.
var surfaceWritesAllowed = map[string]string{
	"internal/surface/gui/api.go (*API).Ready os.MkdirAll":                 "creates <data>/logs for gui.log, the start-up time log (02 §1)",
	"internal/surface/gui/api.go (*API).Ready os.OpenFile":                 "appends the start-up time to <data>/logs/gui.log (02 §1)",
	"internal/surface/gui/gui.go logToFile os.MkdirAll":                    "creates <data>/logs for gui.log, which also takes the standard log (02 §1)",
	"internal/surface/gui/gui.go logToFile os.OpenFile":                    "appends the standard log (task panics with their stack) to <data>/logs/gui.log (02 §1)",
	"internal/surface/gui/api.go (*API).SaveRefNums os.MkdirAll":           "creates <data>/refnums for the reference-number files (ctrl+\\)",
	"internal/surface/gui/api.go (*API).SaveRefNums os.WriteFile":          "writes <data>/refnums/<prefix>.txt.tmp, the page's reference numbers",
	"internal/surface/gui/api.go (*API).SaveRefNums os.Rename":             "replaces <data>/refnums/<prefix>.txt with the .tmp it wrote",
	"internal/surface/gui/api.go (*API).PackRemoveUnfinished os.RemoveAll": "existing; open question: deletes an unfinished pack folder that a stopped build left in the pack directory, permanently and from the surface; move it into app (with the build lock) and decide whether it goes through the recycle bin (02 §3)",
	"internal/surface/gui/accounts.go (*API).beginAccount os.Remove":       "existing; open question: deletes a LoversLab download staged in accounts/downloads when the window closes mid-operation (11 §4); the cleanup belongs in app/accounts",
	"internal/surface/gui/accounts.go (*API).accountAction os.Remove":      "existing; open question: deletes staged LoversLab downloads when the account is forgotten (11 §4); the cleanup belongs in app/accounts",
	"internal/surface/gui/accounts.go (*API).LoverFollow os.Remove":        "existing; open question: deletes the previous staged LoversLab download for the same file (11 §4); the cleanup belongs in app/accounts",
	"internal/surface/gui/accounts.go (*API).LoverInstall os.Remove":       "existing; open question: deletes the staged LoversLab download after installing it (11 §4); the cleanup belongs in app/accounts",
	"internal/surface/gui/readers.go (*API).Close os.Remove":               "existing; open question: deletes staged LoversLab downloads when the window closes (11 §4); the cleanup belongs in app/accounts",
	"internal/cmds/exchange.go loadoutExport os.WriteFile":                 "existing; open question: `loadout export --out` writes the exported list to a new file the player names (never overwrites); an app.ExportLoadoutTo would keep cmds write-free",
}

// surfaceWriteProblems: surfaces and the tool layer call app and view, which own every
// write (02 §1, 17 §2: business rules and writes do not live in surfaces).
func surfaceWriteProblems(files []*goFile, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test || !(under(g.dir, surfaceDirs) || g.dir == "internal/cmds") {
			continue
		}
		for _, r := range g.refs() {
			if !writes(r) {
				continue
			}
			if k := writeKey(g, r); allow[k] != "" {
				used[k] = true
				continue
			}
			out = append(out, fmt.Sprintf("02 §1, 17 §2: %s in %s: %s writes to disk from %s; surfaces and cmds call internal/app or internal/view for every write (02 §1 lists them)", loc(r.pos, g.path), r.fn, r, g.dir))
		}
	}
	sort.Strings(out)
	return out
}

func TestArchSurfaceWrites(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	report(t, surfaceWriteProblems(repoSource(t), surfaceWritesAllowed, used))
	report(t, staleProblems("surfaceWritesAllowed", surfaceWritesAllowed, used))
}

// testTrashAllowed: test files that call source.Trash itself. Outside Windows the
// internal/app tests recycle into their own XDG data folder (17 §4); on Windows a test
// binary's Trash refuses (source.ErrTestRecycle), so only internal/source tests it there.
var testTrashAllowed = map[string]bool{"internal/app/main_other_test.go": true}

// testTrashProblems: a test outside internal/source recycles through its App's TrashFunc
// (testkit.Env.Recycle, a fake bin in the fixture), never source.Trash: on a developer's
// machine that is the real recycle bin (17 §4).
func testTrashProblems(g *goFile, mod string) []string {
	var out []string
	if g.dir == "internal/source" || testTrashAllowed[g.path] {
		return nil
	}
	for _, r := range g.refs() {
		if r.pkg == mod+"/internal/source" && r.name == "Trash" {
			out = append(out, fmt.Sprintf("17 §4: %s in %s uses source.Trash; a test recycles through its App's TrashFunc (testkit.Env.Recycle), never the real recycle bin", loc(r.pos, g.path), r.fn))
		}
	}
	return out
}

// trashProblems: one recycle bin entry point (02 §3). source.Trash is reached only through
// App.ToTrash in internal/app/trash.go; only source/trash_windows.go calls SHFileOperationW;
// the trash implementations never delete permanently.
func trashProblems(files []*goFile, mod string) []string {
	var out []string
	for _, g := range files {
		if g.test {
			out = append(out, testTrashProblems(g, mod)...)
			continue
		}
		trashImpl := g.dir == "internal/source" && strings.HasPrefix(path.Base(g.path), "trash_")
		for _, r := range g.refs() {
			if r.pkg == mod+"/internal/source" && r.name == "Trash" && g.path != "internal/app/trash.go" {
				out = append(out, fmt.Sprintf("02 §3: %s in %s uses source.Trash; every recycle goes through App.ToTrash (internal/app/trash.go), which enforces the allowlist", loc(r.pos, g.path), r.fn))
			}
			if trashImpl && r.pkg == "os" && r.name == "RemoveAll" {
				out = append(out, fmt.Sprintf("02 §3: %s in %s: os.RemoveAll in the recycle bin; trash never falls back to permanent deletion", loc(r.pos, g.path), r.fn))
			}
		}
		ast.Inspect(g.f, func(n ast.Node) bool {
			var text string
			switch n := n.(type) {
			case *ast.Ident:
				text = n.Name
			case *ast.BasicLit:
				text = n.Value
			case *ast.CallExpr:
				// Inside source, a call to Trash outside trash_*.go is a second entry point.
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "Trash" && g.dir == "internal/source" && !trashImpl {
					out = append(out, fmt.Sprintf("02 §3: %s calls Trash; the recycle bin is reached only through App.ToTrash", loc(g.fset.Position(n.Pos()), g.path)))
				}
				return true
			default:
				return true
			}
			if strings.Contains(text, "SHFileOperation") && g.path != "internal/source/trash_windows.go" {
				out = append(out, fmt.Sprintf("02 §3: %s names SHFileOperation; only internal/source/trash_windows.go calls it, behind App.ToTrash", loc(g.fset.Position(n.Pos()), g.path)))
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

func TestArchTrash(t *testing.T) {
	t.Parallel()
	_, mod := repoRoot(t)
	report(t, trashProblems(repoSource(t), mod))
}

func TestArchWritesDetection(t *testing.T) {
	t.Parallel()
	const writer = `package p
import (
	"io/ioutil"
	"os"
)
func Read(p string) { os.ReadFile(p); os.Open(p); os.OpenFile(p, os.O_RDONLY, 0); os.Stat(p) }
func Write(p string, flag int) {
	os.WriteFile(p, nil, 0o644)
	os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0)
	os.OpenFile(p, flag, 0)
	defer os.Remove(p)
	go func() { os.RemoveAll(p) }()
	ioutil.WriteFile(p, nil, 0)
	os.Rename(p, p); os.Chtimes(p, t, t); os.MkdirAll(p, 0); os.Mkdir(p, 0); os.Create(p)
}
`
	for _, tc := range []struct {
		file string
		want int
	}{
		{"internal/scan/x.go", 11},
		{"internal/scan/x_test.go", 0},
		{"internal/app/savegrid.go", 11},
		{"internal/app/saveactions.go", 0},
		{"internal/app/launch.go", 0},
		{"internal/store/x.go", 0},
	} {
		expect(t, "read-only "+tc.file, readOnlyProblems(synthetic(t, map[string]string{tc.file: writer}), nil, map[string]bool{}), tc.want)
	}
	expect(t, "deps net/http", readOnlyProblems(synthetic(t, map[string]string{"internal/deps/d.go": "package deps\nimport \"net/http\"\nvar _ = http.Get\n"}), nil, map[string]bool{}), 1)

	for _, tc := range []struct {
		file string
		want int
	}{
		{"internal/surface/gui/x.go", 11},
		{"internal/surface/tui/x.go", 11},
		{"internal/cmds/x.go", 11},
		{"internal/view/x.go", 0},
		{"internal/app/x.go", 0},
	} {
		expect(t, "surface "+tc.file, surfaceWriteProblems(synthetic(t, map[string]string{tc.file: writer}), nil, map[string]bool{}), tc.want)
	}
	allow := map[string]string{"internal/cmds/x.go Write os.Remove": "x", "internal/cmds/x.go Gone os.Remove": "y"}
	used := map[string]bool{}
	expect(t, "allowed surface write", surfaceWriteProblems(synthetic(t, map[string]string{"internal/cmds/x.go": writer}), allow, used), 10)
	expect(t, "stale surface exception", staleProblems("surfaceWritesAllowed", allow, used), 1)

	const m = "example.com/x"
	got := trashProblems(synthetic(t, map[string]string{
		"internal/app/trash.go":            "package app\nimport \"example.com/x/internal/source\"\nvar trash = source.Trash\n",
		"internal/app/mods.go":             "package app\nimport \"example.com/x/internal/source\"\nfunc f() { source.Trash(\"x\") }\n",
		"internal/source/trash_windows.go": "package source\nimport \"os\"\nvar p = \"SHFileOperationW\"\nfunc Trash(p string) error { return os.RemoveAll(p) }\n",
		"internal/source/link.go":          "package source\nfunc g() { Trash(\"x\") }\n",
		"internal/surface/gui/x.go":        "package gui\nvar procSHFileOperationW = 1\n",
		"internal/source/trash_test.go":    "package source\nimport \"os\"\nfunc h() { os.RemoveAll(\"x\") }\n",
	}), m)
	expect(t, "trash", got, 4)
}
