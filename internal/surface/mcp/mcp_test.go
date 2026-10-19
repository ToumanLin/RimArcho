package mcp_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// connect serves e.App over in-memory transports and returns a client session.
func connect(t *testing.T, e *testkit.Env) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := sdk.NewInMemoryTransports()
	srv := mcp.New(e.App)
	if _, err := srv.SDK().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		sb.WriteString(c.(*sdk.TextContent).Text)
	}
	return sb.String(), res.IsError
}

func TestToolsAndAnnotations(t *testing.T) {
	e := testkit.New(t)
	cs := connect(t, e)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{ // name → expected annotation: ro, write, destructive
		"status": "ro", "find_mods": "ro", "get_mod": "ro", "get_def": "ro", "get_patches": "ro", "get_harmony": "ro",
		"explain_relation": "ro", "list_problems": "ro", "list_changes": "ro", "analyze_log": "ro", "search_files": "ro",
		"read_source": "ro", "edit_loadout": "write", "copy_local": "write", "save_loadout": "destructive", "add_rule": "write",
		"get_description": "ro", "set_player_data": "write",
	}
	seen := map[string]bool{}
	for _, tl := range res.Tools {
		seen[tl.Name] = true
		kind, ok := want[tl.Name]
		if !ok {
			t.Errorf("unexpected tool %s", tl.Name)
			continue
		}
		a := tl.Annotations
		switch {
		case a == nil:
			t.Errorf("%s: no annotations", tl.Name)
		case kind == "ro" && !a.ReadOnlyHint:
			t.Errorf("%s: want readOnlyHint", tl.Name)
		case kind != "ro" && (a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint != (kind == "destructive")):
			t.Errorf("%s: annotations %+v, want %s", tl.Name, a, kind)
		}
		// 05 §4.3: every description gives an example call with output.
		if !strings.Contains(tl.Description, "Example: "+tl.Name+" {") {
			t.Errorf("%s: description has no example call", tl.Name)
		}
	}
	for n := range want {
		if !seen[n] {
			t.Errorf("tool %s missing", n)
		}
	}
	if r := cs.InitializeResult(); r == nil || !strings.Contains(r.Instructions, "analyze_log") {
		t.Error("server instructions missing the diagnosis workflow")
	}
}

// I1: the same command through the CLI and through MCP gives identical text. I5: default
// output is at most 40 lines.
func TestCLIParity(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	cs := connect(t, e)
	modA := ref(t, e, "Mod A")
	cases := []struct {
		tool string
		args map[string]any
		cli  []string
	}{
		{"status", nil, []string{"status"}},
		{"find_mods", map[string]any{"query": "mod"}, []string{"find", "mod"}},
		{"find_mods", map[string]any{"active": true}, []string{"list", "--active"}},
		{"get_mod", map[string]any{"ref": modA}, []string{"mod", modA}},
		{"get_def", map[string]any{"def": "ThingDef/Lib_Gun"}, []string{"def", "ThingDef/Lib_Gun"}},
		{"get_patches", map[string]any{"failed": true}, []string{"patches", "--failed"}},
		{"get_harmony", map[string]any{"target": "Verse.Pawn.Tick"}, []string{"harmony", "--target", "Verse.Pawn.Tick"}},
		{"get_harmony", map[string]any{"mod": modA}, []string{"harmony", "--mod", modA}},
		{"explain_relation", map[string]any{"a": modA, "b": "pkg:tester.modb"}, []string{"why", modA, "pkg:tester.modb"}},
		{"list_problems", nil, []string{"problems", "--min-severity", "warning"}},
		{"list_problems", map[string]any{"code": "harmony", "min_severity": "info"}, []string{"problems", "--code", "harmony", "--min-severity", "info"}},
		{"list_changes", nil, []string{"changes"}},
		{"analyze_log", nil, []string{"log"}},
		{"analyze_log", map[string]any{"cluster": "E3"}, []string{"log", "--cluster", "E3"}},
		{"analyze_log", map[string]any{"text": testkit.FixtureLog(`C:\x\Compat.xml`)}, []string{"log", "--text", testkit.FixtureLog(`C:\x\Compat.xml`)}},
		{"analyze_log", map[string]any{"cluster": "E1", "game_version": "1.6"}, []string{"log", "--cluster", "E1", "--game-version", "1.6"}},
		{"search_files", map[string]any{"pattern": "Lib_Gun"}, []string{"files", "search", "Lib_Gun"}},
		{"save_loadout", map[string]any{"dry_run": true}, []string{"save", "--dry-run"}},
		{"edit_loadout", map[string]any{"action": "sort", "dry_run": true}, []string{"sort", "--dry-run"}},
		{"get_description", map[string]any{"ref": modA, "offline": true}, []string{"describe", modA, "--offline"}},
	}
	for _, c := range cases {
		got, isErr := call(t, cs, c.tool, c.args)
		want, code := e.Run(c.cli...)
		if got != want {
			t.Errorf("%s %v: MCP text differs from `archo %s`:\n--- mcp\n%s\n--- cli\n%s", c.tool, c.args, strings.Join(c.cli[:1], " "), got, want)
		}
		if isErr != (code == cmds.ExitError || code == cmds.ExitUsage) {
			t.Errorf("%s: isError=%v, exit %d", c.tool, isErr, code)
		}
		if n := strings.Count(strings.TrimRight(got, "\n"), "\n") + 1; n > 40 {
			t.Errorf("%s %v: %d lines (I5: at most 40)", c.tool, c.args, n)
		}
	}
}

// I2: references printed by one tool are accepted by the others.
func TestRefsRoundTrip(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	cs := connect(t, e)
	refRe := regexp.MustCompile(`(?:^|\s)(#[0-9a-z]{4}|p:[0-9a-f]{6}|log:E\d+|def:\w+/\w+|pkg:[\w.]+)\b`)
	seen := map[string]bool{}
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"get_harmony", map[string]any{"target": "Verse.Pawn.Tick"}},
		{"analyze_log", nil},
		{"list_problems", map[string]any{"min_severity": "info"}},
		{"get_patches", map[string]any{"failed": true}},
		{"analyze_log", map[string]any{"cluster": "E2"}},
	} {
		out, _ := call(t, cs, c.tool, c.args)
		for _, m := range refRe.FindAllStringSubmatch(out, -1) {
			seen[m[1]] = true
		}
	}
	if len(seen) < 8 {
		t.Fatalf("only %d refs found: %v", len(seen), seen)
	}
	for r := range seen {
		var tool string
		var args map[string]any
		switch {
		case strings.HasPrefix(r, "#"), strings.HasPrefix(r, "pkg:"):
			tool, args = "get_mod", map[string]any{"ref": r}
		case strings.HasPrefix(r, "p:"):
			tool, args = "list_problems", map[string]any{"ref": r}
		case strings.HasPrefix(r, "log:"):
			tool, args = "analyze_log", map[string]any{"cluster": r}
		case strings.HasPrefix(r, "def:"):
			tool, args = "get_def", map[string]any{"def": r}
		}
		if out, isErr := call(t, cs, tool, args); isErr {
			t.Errorf("%s %s: %s", tool, r, out)
		}
	}
}

// I3: a packageId with several instances lists the candidates instead of picking one.
func TestAmbiguousPackage(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.WriteMod(filepath.Join(e.Game, "Mods", "ModA_copy"), testkit.Mod{PackageID: "tester.moda", Name: "Mod A (local copy)", Author: "Tester"})
	e.MustRun("scan")
	cs := connect(t, e)
	out, _ := call(t, cs, "get_mod", map[string]any{"ref": "pkg:tester.moda"})
	for _, name := range []string{"Mod A", "Mod A (local copy)"} {
		if r := ref(t, e, name); !strings.Contains(out, r) {
			t.Errorf("candidate %s (%s) not listed:\n%s", name, r, out)
		}
	}
	if strings.Contains(out, "status:") {
		t.Errorf("picked one instance instead of listing candidates:\n%s", out)
	}
}

// I5: a long list is cut at 40 lines with the command that shows everything.
func TestTruncation(t *testing.T) {
	e := testkit.New(t)
	var active []string
	for i := 0; i < 60; i++ {
		pkg := fmt.Sprintf("tester.m%02d", i)
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("M%02d", i)), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Many %02d", i)})
		active = append(active, pkg)
	}
	e.WriteModsConfig("1.6.4633 rev1261", append([]string{"ludeon.rimworld"}, active...)...)
	e.AddEnv(false)
	e.MustRun("scan")
	cs := connect(t, e)
	for _, c := range []struct {
		tool string
		args map[string]any
	}{{"find_mods", map[string]any{"active": true}}, {"find_mods", map[string]any{"query": "many"}}, {"find_mods", nil}} {
		out, _ := call(t, cs, c.tool, c.args)
		n := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
		if n > 40 || !strings.Contains(out, "--all") {
			t.Errorf("%s %v: %d lines, expansion hint present=%v\n%s", c.tool, c.args, n, strings.Contains(out, "--all"), out)
		}
		all := map[string]any{"all": true}
		for k, v := range c.args {
			all[k] = v
		}
		full, _ := call(t, cs, c.tool, all)
		if !strings.Contains(full, "Many 59") {
			t.Errorf("%s all=true does not list everything", c.tool)
		}
	}
}

func ref(t *testing.T, e *testkit.Env, name string) string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.Name == name {
			return ix.Ref(m.ID)
		}
	}
	t.Fatalf("no mod %q", name)
	return ""
}

// Write tools only ever run against synthetic environments: enable, save, rule, undo.
func TestWriteTools(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	cs := connect(t, e)
	lib := ref(t, e, "Lib")
	if out, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "enable", "refs": []string{lib}, "before": "pkg:tester.compat"}); isErr || !strings.Contains(out, "1 change unsaved") {
		t.Fatalf("enable:\n%s", out)
	}
	if strings.Contains(e.ReadModsConfig(), "tester.lib") {
		t.Fatal("edit_loadout wrote ModsConfig.xml")
	}
	if out, _ := call(t, cs, "save_loadout", map[string]any{"dry_run": true}); strings.Contains(e.ReadModsConfig(), "tester.lib") {
		t.Fatalf("dry run wrote:\n%s", out)
	}
	if out, isErr := call(t, cs, "save_loadout", nil); isErr || !strings.Contains(e.ReadModsConfig(), "tester.lib") {
		t.Fatalf("save:\n%s", out)
	}
	if out, isErr := call(t, cs, "add_rule", map[string]any{"kind": "after", "a": "pkg:tester.modc", "b": "pkg:tester.modb"}); isErr || !strings.Contains(out, "r:1") {
		t.Fatalf("add_rule:\n%s", out)
	}
	if out, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "bogus"}); !isErr {
		t.Fatalf("unknown action is an error:\n%s", out)
	}
}

// 05 §4.1: a tool can name another environment.
func TestEnvParameter(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("env", "add", "other", "--game", e.Game, "--user-data", e.UserData, "--workshop", e.Workshop)
	e.MustRun("env", "use", "main")
	cs := connect(t, e)
	if out, _ := call(t, cs, "status", nil); !strings.HasPrefix(out, "env:main") {
		t.Fatalf("default env:\n%s", out)
	}
	if out, _ := call(t, cs, "status", map[string]any{"env": "other"}); !strings.HasPrefix(out, "env:other") {
		t.Fatalf("env=other:\n%s", out)
	}
	if out, _ := call(t, cs, "status", nil); !strings.HasPrefix(out, "env:main") {
		t.Fatalf("env is per call:\n%s", out)
	}
	if out, isErr := call(t, cs, "status", map[string]any{"env": "nope"}); !isErr || !strings.Contains(out, `no environment named "nope"`) {
		t.Fatalf("unknown env:\n%s", out)
	}
}

// 05 §4.1: when the analysis takes longer than the wait, the call returns the declaration
// problems with a note, and a later call gets the finished analysis.
func TestStillComputing(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	inner := e.App.AnalyzeRunner
	release := make(chan bool)
	e.App.AnalyzeRunner = func(env string, p func(string)) error { <-release; return inner(env, p) }
	e.App.AnalyzeWait = 50 * time.Millisecond
	cs := connect(t, e)
	out, isErr := call(t, cs, "list_problems", nil)
	if isErr || !strings.Contains(out, "analysis: still computing in the background") {
		t.Fatalf("first call:\n%s", out)
	}
	close(release)
	time.Sleep(500 * time.Millisecond)
	if out, _ := call(t, cs, "list_problems", map[string]any{"code": "harmony"}); !strings.Contains(out, "analysis: a:1") || !strings.Contains(out, "harmony.multi-transpiler") {
		t.Fatalf("second call:\n%s", out)
	}
}
