package mcp_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestForeignLogCLIParity(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	other := filepath.Join(e.Root, "other-game")
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.MustRun("env", "add", "other", "--game", other, "--user-data", e.UserData)
	e.MustRun("env", "use", "main")
	e.MustRun("analyze")
	path := filepath.Join(e.UserData, "Player.log")
	// Same game version, different game directory: version equality is not ownership.
	e.WriteFile(path, "Mono path[0] = '"+filepath.ToSlash(filepath.Join(other, "RimWorldWin64_Data", "Managed"))+"'\n"+text)
	runs := 0
	e.App.AnalyzeRunner = func(string, func(string)) error { runs++; return fmt.Errorf("unexpected analysis") }
	cs := connect(t, e)
	for _, tc := range []struct {
		name string
		in   map[string]any
		argv []string
	}{
		{"analyze_log", map[string]any{}, []string{"log"}},
		{"analyze_log", map[string]any{"file": path, "game_version": "1.7"}, []string{"log", "--file", path, "--game-version", "1.7"}},
		{"list_problems", map[string]any{"all": true}, []string{"problems", "--all"}},
		{"status", map[string]any{}, []string{"status"}},
	} {
		want, code := e.Run(tc.argv...)
		got, bad := call(t, cs, tc.name, tc.in)
		if got != cmds.MCPHints(want) || bad != (code != 0) {
			t.Fatalf("%s: CLI (%d) %s\nMCP (%v) %s", tc.name, code, want, bad, got)
		}
		if tc.name != "analyze_log" && (strings.Contains(got, "stale log") || strings.Contains(got, "log:E")) {
			t.Fatal(got)
		}
	}
	if runs != 0 {
		t.Fatalf("foreign log started %d analyses", runs)
	}
}
