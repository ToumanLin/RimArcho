package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
)

// --open-plan names the list a command acts on without making it current (04 §1.3).
func TestOpenPlanFlag(t *testing.T) {
	for _, c := range []struct {
		argv       []string
		plan, rest string
	}{
		{[]string{"analyze", "--open-plan", "B"}, "B", "analyze"},
		{[]string{"--open-plan=Two words", "pack", "build", "p"}, "Two words", "pack build p"},
		{[]string{"textures", "optimize", "--plan"}, "", "textures optimize --plan"},
		{[]string{"x", "--", "--open-plan", "B"}, "", "x -- --open-plan B"},
	} {
		plan, rest, err := openPlanFlag(c.argv)
		if err != nil || plan != c.plan || strings.Join(rest, " ") != c.rest {
			t.Fatalf("%q: %q %q %v", c.argv, plan, rest, err)
		}
	}
	if _, _, err := openPlanFlag([]string{"analyze", "--open-plan"}); err == nil {
		t.Fatal("--open-plan without a name")
	}
}

func TestHereCompatibility(t *testing.T) {
	for _, tc := range []struct {
		argv      []string
		env, rest string
	}{
		{[]string{"--here"}, "", ""},
		{[]string{"--here", "tui"}, "", "tui"},
		{[]string{"--env", "main", "--here", "status"}, "main", "status"},
		{[]string{"list", "--here"}, "", "list --here"},
		{[]string{"--", "--here"}, "", "-- --here"},
	} {
		env, rest, err := globalFlags(tc.argv)
		if err != nil || env != tc.env || strings.Join(rest, " ") != tc.rest {
			t.Fatalf("%q: env=%q rest=%q err=%v", tc.argv, env, rest, err)
		}
	}
}

// archo mcp [--tools core|all] (05 §5.1): core by default; a bad flag or value is a usage
// error before the data folder is opened.
func TestMCPToolsFlag(t *testing.T) {
	for argv, want := range map[string]mcp.Group{"mcp": mcp.Core, "mcp --tools core": mcp.Core, "mcp --tools all": mcp.All, "mcp --tools=all": mcp.All} {
		if g, err := mcpGroup(strings.Fields(argv)); err != nil || g != want {
			t.Errorf("%s: %q %v, want %q", argv, g, err, want)
		}
	}
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("RIMARCHO_HOME", home)
	for _, argv := range []string{"mcp --tools full", "mcp --tools", "mcp --tool all", "mcp extra"} {
		var out, errOut strings.Builder
		if code := Run(strings.Fields(argv), &out, &errOut); code != cmds.ExitUsage || !strings.Contains(errOut.String(), "usage: archo mcp [--tools core|all]") {
			t.Errorf("%s: exit %d, stderr %q", argv, code, errOut.String())
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("a usage error opened the data folder: %v", err)
	}
	var out, errOut strings.Builder
	if code := Run([]string{"mcp", "--help"}, &out, &errOut); code != cmds.ExitOK || !strings.Contains(out.String(), "--tools core|all") {
		t.Errorf("mcp --help: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}
