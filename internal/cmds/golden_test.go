package cmds_test

// Golden tests: every M1 command's outline text is compared with testdata/golden/*.txt.
// Regenerate with: go test ./internal/cmds -update

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rimarcho/rimarcho/internal/store"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

var update = flag.Bool("update", false, "rewrite golden files")

type step func(e *testkit.Env) []string

func cmd(args ...string) step { return func(*testkit.Env) []string { return args } }

// act changes the fixture between commands; it prints nothing.
func act(f func(e *testkit.Env)) step {
	return func(e *testkit.Env) []string { f(e); return nil }
}

func ref(e *testkit.Env, name string) string {
	ix, err := e.App.Index()
	if err != nil {
		e.T.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.Name == name {
			return ix.Ref(m.ID)
		}
	}
	e.T.Fatalf("no mod named %q", name)
	return ""
}

func byName(e *testkit.Env, name string, pick func(*store.Mod) bool) *store.Mod {
	ix, _ := e.App.Index()
	for _, m := range ix.Mods {
		if m.Name == name && (pick == nil || pick(m)) {
			return m
		}
	}
	return nil
}

func goldenDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "golden")
}

func runGolden(t *testing.T, name string, e *testkit.Env, steps []step) {
	t.Helper()
	var sb strings.Builder
	for _, s := range steps {
		args := s(e)
		if args == nil {
			continue
		}
		out, code := e.Run(args...)
		fmt.Fprintf(&sb, "$ archo %s\n", e.Scrub(strings.Join(args, " ")))
		sb.WriteString(e.Scrub(out))
		if code != 0 {
			fmt.Fprintf(&sb, "[exit %d]\n", code)
		}
		sb.WriteString("\n")
	}
	got := sb.String()
	path := filepath.Join(goldenDir(), name+".txt")
	if *update {
		os.MkdirAll(goldenDir(), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/cmds -update)", err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s differs from golden (go test ./internal/cmds -update):\n%s", name, diff(string(want), got))
	}
}

func diff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	var out []string
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			out = append(out, fmt.Sprintf("line %d\n  want: %s\n  got:  %s", i+1, w, g))
			if len(out) > 10 {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

// populate creates a small but varied mod collection.
func populate(e *testkit.Env) {
	e.CopyTemplate("harmony", filepath.Join(e.Workshop, "2009463077"))
	e.Subscribe("2009463077")
	e.CopyTemplate("versioned", filepath.Join(e.Workshop, "3000000001"))
	e.Subscribe("3000000001")
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Workshop, "3000000009"), testkit.Sample("tester.leftover", "Leftover")) // unsubscribed
	e.WriteFile(filepath.Join(e.Workshop, "3000000010", "Textures", "x.png"), "x")                     // residue without About.xml
	e.CopyTemplate("legacy", filepath.Join(e.Game, "Mods", "OldMod"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Twin"), testkit.Sample("tester.alpha", "Alpha Fork"))
	e.MkdirAll(filepath.Join(e.Game, "Mods", "NotAMod"))
}

func TestGoldenBasics(t *testing.T) {
	e := testkit.New(t)
	populate(e)
	runGolden(t, "basics", e, []step{
		cmd("help"),
		cmd("status"),
		cmd("env", "detect"),
		cmd("env", "add", "main", "--detected"),
		cmd("env", "list"),
		cmd("scan"),
		cmd("status"),
		cmd("list"),
		cmd("list", "--filter", "residue"),
		cmd("list", "--filter", "source:local"),
		cmd("find", "harmony"),
		cmd("find", "nothing-like-this"),
		cmd("mod", "pkg:brrainz.harmony"),
		cmd("mod", "ws:3000000001"),
		func(e *testkit.Env) []string { return []string{"mod", ref(e, "Old Mod")} },
		cmd("mod", "pkg:tester.alpha"),
		cmd("mod", "pkg:nope"),
		cmd("mod", "Alpha"),
		cmd("changes"),
		cmd("scan"),
		cmd("changes"),
		cmd("identity", "list"),
		cmd("frobnicate"),
		cmd("list", "--bogus"),
	})
}

func TestGoldenLifecycle(t *testing.T) {
	e := testkit.New(t)
	populate(e)
	e.AddEnv(true)
	e.CopyTemplate("harmony", filepath.Join(e.SteamCMD, "2009463077"))
	e.MustRun("scan")
	runGolden(t, "lifecycle", e, []step{
		cmd("status"),
		act(func(e *testkit.Env) {
			e.Advance(26 * time.Hour)
			// Beta's author renames the package and the mod; Gamma's folder is renamed;
			// Alpha is copied into Mods/; Leftover is deleted.
			b := testkit.Sample("newauthor.beta", "Beta Reborn")
			b.Author = "New Author"
			b.Supported = []string{"1.5", "1.6"}
			b.Deps = []string{"brrainz.harmony"}
			e.WriteFile(filepath.Join(e.Workshop, "3000000003", "About", "About.xml"), b.AboutXML())
			e.Move(filepath.Join(e.Game, "Mods", "Gamma"), filepath.Join(e.Game, "Mods", "Gamma (renamed)"))
			e.CopyTree(filepath.Join(e.Workshop, "3000000002"), filepath.Join(e.Game, "Mods", "Alpha Local"))
			e.Remove(filepath.Join(e.Workshop, "3000000009"))
		}),
		cmd("scan"),
		cmd("changes"),
		func(e *testkit.Env) []string { return []string{"changes", ref(e, "Beta Reborn")} },
		cmd("mod", "pkg:tester.alpha"),
		act(func(e *testkit.Env) {
			e.Advance(24 * time.Hour)
			// Leftover comes back in a new folder under a new package id: RimArcho asks.
			m := testkit.Sample("tester.leftover2", "Leftover")
			e.WriteMod(filepath.Join(e.Game, "Mods", "Leftover Again"), m)
		}),
		cmd("scan"),
		cmd("identity", "list"),
		cmd("identity", "resolve", "q:1", "same"),
		cmd("identity", "list", "--all"),
		func(e *testkit.Env) []string { return []string{"changes", ref(e, "Leftover")} },
		func(e *testkit.Env) []string {
			m := byName(e, "Alpha", func(m *store.Mod) bool { return m.SourceLabel == "local" })
			ix, _ := e.App.Index()
			return []string{"lineage", "split", ix.Ref(m.ID)}
		},
		func(e *testkit.Env) []string {
			ix, _ := e.App.Index()
			a := byName(e, "Alpha", func(m *store.Mod) bool { return m.SourceLabel == "workshop" })
			l := byName(e, "Alpha", func(m *store.Mod) bool { return m.SourceLabel == "local" })
			return []string{"lineage", "merge", ix.LRef(a.LineageID), ix.Ref(l.ID)}
		},
		cmd("mod", "pkg:brrainz.harmony"),
		cmd("changes", "--since", "scan:2", "--all"),
		cmd("gc", "--dry-run"),
		act(func(e *testkit.Env) { e.Advance(120 * 24 * time.Hour) }),
		cmd("gc"),
		cmd("status"),
	})
}

func TestGoldenRebind(t *testing.T) {
	e := testkit.New(t)
	populate(e)
	e.AddEnv(false)
	e.MustRun("scan")
	runGolden(t, "rebind", e, []step{
		act(func(e *testkit.Env) { e.Move(e.Game, filepath.Join(e.Root, "NewDrive", "RimWorld")) }),
		cmd("scan"),
		cmd("status"),
		func(e *testkit.Env) []string {
			return []string{"env", "set", "main", "--game", filepath.Join(e.Root, "NewDrive", "RimWorld")}
		},
		cmd("scan"),
		act(func(e *testkit.Env) {
			other := filepath.Join(e.Root, "Unrelated", "RimWorld")
			e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
			e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
			e.WriteMod(filepath.Join(other, "Mods", "Different"), testkit.Sample("tester.different", "Different"))
		}),
		func(e *testkit.Env) []string {
			return []string{"env", "set", "main", "--game", filepath.Join(e.Root, "Unrelated", "RimWorld")}
		},
		cmd("env", "list"),
		cmd("env", "rebind", "src:local", "no"),
		cmd("scan"),
		cmd("env", "list"),
	})
}
