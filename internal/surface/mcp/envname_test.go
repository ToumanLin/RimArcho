package mcp_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestEnvByGameVersion: env= takes a game version when one Game Config has it; a name that
// selects none lists the Game Configs with their versions; status names them all (13 §2, 14 §1).
func TestEnvByGameVersion(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	cs := connect(t, e)
	if out, isErr := call(t, cs, "status", map[string]any{"env": "1.6"}); isErr || !strings.Contains(out, "env:main") {
		t.Fatalf("env=1.6: %s", out)
	}
	out, isErr := call(t, cs, "status", map[string]any{"env": "1.9"})
	if !isErr || !strings.Contains(out, `no Game Config named "1.9"; the Game Configs are main (RimWorld 1.6)`) || strings.Contains(out, "archo env list") {
		t.Fatalf("env=1.9: %s", out)
	}
	if out, _ := call(t, cs, "status", map[string]any{"show": "envs"}); !strings.Contains(out, "main  1.6.4633") {
		t.Fatalf("show=envs: %s", out)
	}
	// Two Game Configs of one game: the version is ambiguous, and status lists both.
	e.MustRun("env", "add", "other", "--game", e.Game, "--saves", "own")
	if out, isErr := call(t, cs, "status", map[string]any{"env": "1.6"}); !isErr || !strings.Contains(out, "several Game Configs have RimWorld 1.6: main, other") {
		t.Fatalf("ambiguous env=1.6: %s", out)
	}
	out, _ = call(t, cs, "status", map[string]any{"env": "other"})
	if !strings.Contains(out, "game configs: 2 (env=<name> or env=<game version> in other calls)") ||
		!strings.Contains(out, "main  RimWorld 1.6.4633 rev1261  ") || !strings.Contains(out, "  [this call]") {
		t.Fatalf("status lists the Game Configs: %s", out)
	}
}
