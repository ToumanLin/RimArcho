package mcp_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestVersionMuteTool(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	cs := connect(t, e)
	got, isErr := call(t, cs, "mute_version_warnings", map[string]any{"refs": []string{"pkg:tester.a"}, "game_version": "1.7"})
	if isErr || !strings.Contains(got, "ignored for 1.7") {
		t.Fatalf("mute: %s %v", got, isErr)
	}
	got, isErr = call(t, cs, "list_problems", nil)
	if isErr || !strings.Contains(got, "ignored-version-warnings: 1 version warnings") || strings.Contains(got, "Alpha") {
		t.Fatalf("list: %s %v", got, isErr)
	}
	got, isErr = call(t, cs, "list_problems", map[string]any{"include_muted": true})
	if isErr || !strings.Contains(got, "Alpha") || !strings.Contains(got, "[version warning ignored]") {
		t.Fatalf("include: %s %v", got, isErr)
	}
	got, isErr = call(t, cs, "mute_version_warnings", map[string]any{"refs": []string{"pkg:tester.a"}, "undo": true})
	if isErr || !strings.Contains(got, "restored for 1.7") {
		t.Fatalf("unmute: %s %v", got, isErr)
	}
	got, isErr = call(t, cs, "list_problems", nil)
	if isErr || strings.Contains(got, "ignored-version-warnings: ") || !strings.Contains(got, "version.unsupported") {
		t.Fatalf("restored list: %s %v", got, isErr)
	}
}

func TestVersionMuteArgv(t *testing.T) {
	got := mcp.VersionMuteArgv(mcp.VersionMuteIn{Refs: []string{"pkg:tester.a", "#abcd"}, GameVersion: "1.7", Undo: true})
	want := []string{"problems", "unmute-version", "pkg:tester.a", "#abcd", "--game-version", "1.7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v != %v", got, want)
	}
	if got := mcp.ProblemsArgv(mcp.ProblemsIn{IncludeMuted: true}); !reflect.DeepEqual(got, []string{"problems", "--include-muted"}) {
		t.Fatal(got)
	}
}

func TestIncludeMutedOlderVersionTool(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.old", Name: "Old mod", Supported: []string{"1.5"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.old")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("problems", "mute-version", "pkg:tester.old")
	cs := connect(t, e)
	got, isErr := call(t, cs, "list_problems", map[string]any{"include_muted": true})
	if isErr || !strings.Contains(got, "Old mod") || !strings.Contains(got, "[version warning ignored]") {
		t.Fatalf("info mute missing: %s %v", got, isErr)
	}
}
