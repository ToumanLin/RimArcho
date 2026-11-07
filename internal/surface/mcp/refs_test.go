package mcp_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Agents pass packageIds without pkg: (MCP review 2026-11-04 §2.3). Every tool that takes a
// mod reference accepts them in any case, and a reference matching nothing points to
// find_mods on MCP and to archo find on the CLI (13 §2, §3.3).
func TestBarePackageIDTools(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	cs := connect(t, e)

	for _, c := range []struct {
		tool string
		args func(ref string) map[string]any
	}{
		{"get_mod", func(r string) map[string]any { return map[string]any{"ref": r} }},
		{"get_patches", func(r string) map[string]any { return map[string]any{"mod": r} }},
		{"get_harmony", func(r string) map[string]any { return map[string]any{"mod": r} }},
		{"list_changes", func(r string) map[string]any { return map[string]any{"ref": r} }},
		{"explain_relation", func(r string) map[string]any { return map[string]any{"a": r, "b": "pkg:tester.modb"} }},
	} {
		want, isErr := call(t, cs, c.tool, c.args("pkg:tester.moda"))
		if isErr {
			t.Fatalf("%s pkg:tester.moda: %s", c.tool, want)
		}
		got, isErr := call(t, cs, c.tool, c.args("Tester.ModA"))
		want = strings.ReplaceAll(want, "pkg:tester.moda", "Tester.ModA") // headings echo the ref
		if isErr || got != want {
			t.Errorf("%s Tester.ModA differs from pkg:tester.moda:\n%s\nwant\n%s", c.tool, got, want)
		}

		out, isErr := call(t, cs, c.tool, c.args("khamenman.outfitstandsplus"))
		if !isErr || !strings.Contains(out, "khamenman.outfitstandsplus matches nothing") ||
			!strings.Contains(out, "search with find_mods query=outfitstandsplus") || strings.Contains(out, "archo find") {
			t.Errorf("%s no match on MCP: isError=%v\n%s", c.tool, isErr, out)
		}
	}

	if out, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "enable", "refs": []string{"Tester.ModB"}, "after": "TESTER.MODA"}); isErr {
		t.Fatalf("edit_loadout enable by bare packageId: %s", out)
	}
	out, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "enable", "refs": []string{"OskarPotocki.VanillaFactionsExpanded.Core"}})
	want := "error: OskarPotocki.VanillaFactionsExpanded.Core matches nothing (no mod has packageId oskarpotocki.vanillafactionsexpanded.core; search with find_mods query=vanillafactionsexpanded)\n"
	if !isErr || out != want {
		t.Errorf("edit_loadout no match: isError=%v\n%s\nwant\n%s", isErr, out, want)
	}
	// The CLI keeps naming its own command.
	cli, code := e.Run("enable", "OskarPotocki.VanillaFactionsExpanded.Core")
	if code != 1 || cli != strings.Replace(want, "find_mods query=", "archo find ", 1) {
		t.Errorf("CLI no match: exit %d\n%s", code, cli)
	}
	// Other errors are the same text on both surfaces.
	out, _ = call(t, cs, "get_mod", map[string]any{"ref": "#zzzz"})
	if cli, _ := e.Run("mod", "#zzzz"); !strings.Contains(out, "find_mods query=<words>") || cli != strings.Replace(out, "find_mods query=", "archo find ", 1) {
		t.Errorf("#zzzz: MCP\n%s\nCLI\n%s", out, cli)
	}
}
