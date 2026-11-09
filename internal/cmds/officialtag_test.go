package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Core and the DLCs get the RimWorld tag once, and it is kept on top (16 §3.8): untagging
// survives the next scan, and a stored pinned list gets it in front once.
func TestOfficialTag(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("6100", testkit.Sample("tester.a", "Alpha"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("config", "set", "tags.pinned", "a,b")
	e.MustRun("scan")

	out := e.MustRun("tag", "list")
	if !strings.Contains(out, "RimWorld") {
		t.Fatalf("Core has no RimWorld tag:\n%s", out)
	}
	if v := cmds.Setting(e.App, "tags.pinned"); v != "RimWorld,a,b" {
		t.Fatalf("stored pinned list: %q", v)
	}
	// Core only: the Workshop Mod is not the game's
	if out := e.MustRun("list", "--filter", "tag:RimWorld"); !strings.Contains(out, "mods: 1 ") || !strings.Contains(out, "official Core") {
		t.Fatalf("Mods with the RimWorld tag:\n%s", out)
	}

	e.MustRun("tag", "remove", "pkg:ludeon.rimworld", "RimWorld")
	e.MustRun("config", "set", "tags.pinned", "a,b")
	e.MustRun("scan", "--full")
	if out := e.MustRun("tag", "list"); strings.Contains(out, "RimWorld") {
		t.Fatalf("the RimWorld tag came back after the player removed it:\n%s", out)
	}
	if v := cmds.Setting(e.App, "tags.pinned"); v != "a,b" {
		t.Fatalf("the pinned list changed again: %q", v)
	}
}
