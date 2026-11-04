package mcp_test

import (
	"context"
	"slices"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// coreTools is the default `archo mcp` set (05 §5.2): what agents use to diagnose, edit,
// save, launch and test.
var coreTools = []string{"guide", "status", "find_mods", "get_mod", "get_mod_settings", "get_description", "get_def", "get_patches",
	"get_harmony", "explain_relation", "list_problems", "list_changes", "analyze_log", "search_files", "read_source",
	"edit_loadout", "save_loadout", "launch_game", "test_run", "pin_mod", "unpin_mod", "manage_plans", "plan_history",
	"manage_rules", "ignore_problem", "scan", "list_library"}

func listed(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	return names
}

func TestToolGroups(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	want := slices.Clone(coreTools)
	slices.Sort(want)
	if got := listed(t, connectGroup(t, e, mcp.Core)); !slices.Equal(got, want) {
		t.Errorf("core tools:\n got %v\nwant %v", got, want)
	}
	all := listed(t, connect(t, e))
	if len(all) != 42 || len(mcp.Tools()) != 42 {
		t.Errorf("all: %d tools listed, table %d; want 42", len(all), len(mcp.Tools()))
	}
	// The table, the catalog and the guide's marking agree on every tool's group.
	for _, d := range mcp.Catalog() {
		wantGroup := mcp.All
		if slices.Contains(coreTools, d.Name) {
			wantGroup = mcp.Core
		}
		if d.Group != wantGroup {
			t.Errorf("%s: group %s, want %s", d.Name, d.Group, wantGroup)
		}
		if cmds.AllOnlyTools[d.Name] != (d.Group == mcp.All) {
			t.Errorf("%s: cmds.AllOnlyTools says %v, table says %s", d.Name, cmds.AllOnlyTools[d.Name], d.Group)
		}
		if !slices.Contains(all, d.Name) {
			t.Errorf("%s: not served by --tools all", d.Name)
		}
	}
	if len(cmds.AllOnlyTools) != 42-len(coreTools) {
		t.Errorf("cmds.AllOnlyTools has %d tools, want %d", len(cmds.AllOnlyTools), 42-len(coreTools))
	}
	// A core client still calls a core tool and cannot call an all-only one.
	cs := connectGroup(t, e, mcp.Core)
	if out, isErr := call(t, cs, "guide", map[string]any{"topic": "manage_packs"}); isErr {
		t.Errorf("guide on core:\n%s", out)
	}
	if _, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "manage_packs", Arguments: map[string]any{"action": "list"}}); err == nil {
		t.Error("core server served manage_packs")
	}
}

func TestParseGroup(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]mcp.Group{"": mcp.Core, "core": mcp.Core, "all": mcp.All} {
		if g, err := mcp.ParseGroup(in); err != nil || g != want {
			t.Errorf("ParseGroup(%q) = %q, %v; want %q", in, g, err, want)
		}
	}
	for _, in := range []string{"ALL", "full", "core,all"} {
		if _, err := mcp.ParseGroup(in); err == nil {
			t.Errorf("ParseGroup(%q): no error", in)
		}
	}
}

// The aliases copy_local and add_rule are gone (05 §6): no group serves them.
func TestRemovedAliases(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	cs := connect(t, e)
	for _, name := range []string{"copy_local", "add_rule"} {
		if _, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: map[string]any{"ref": "#x"}}); err == nil {
			t.Errorf("%s: still a tool", name)
		}
		if _, err := mcp.ArgvFor(name, []byte(`{}`)); err == nil {
			t.Errorf("ArgvFor(%s): still a tool", name)
		}
		if _, ok := cmds.Guide(name); ok {
			t.Errorf("guide topic %s still exists", name)
		}
	}
}
