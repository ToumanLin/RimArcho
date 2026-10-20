package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 09 §8: a SteamCMD mod can be enabled; save links it into Mods/ and removes the link once
// it is disabled, never touching the target.
func TestLinkDeployment(t *testing.T) {
	e := loadoutEnv(t)
	cmdDir := filepath.Join(e.SteamCMD, "8888")
	e.WriteMod(cmdDir, testkit.Sample("tester.cmd", "CmdOnly"))
	scanNow(t, e)
	ref := refOf(t, e, "CmdOnly", "")
	link := filepath.Join(e.Game, "Mods", "_Archo_steamcmd_8888")

	e.MustRun("enable", ref)
	out := e.MustRun("save", "--dry-run")
	if !strings.Contains(out, "+ _Archo_steamcmd_8888") {
		t.Fatalf("dry run does not list the link:\n%s", out)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("dry run created the link")
	}
	e.MustRun("save")
	if !source.IsLink(link) {
		t.Fatal("save did not create the link")
	}
	if got, want := mustReal(t, link), mustReal(t, cmdDir); !strings.EqualFold(got, want) {
		t.Fatalf("link → %s, want %s", got, want)
	}
	if !strings.Contains(e.ReadModsConfig(), "<li>tester.cmd</li>") {
		t.Fatalf("ModsConfig.xml:\n%s", e.ReadModsConfig())
	}

	// A rescan sees the link as an alias of the same instance: no new instance, no events,
	// still enabled, and a second save changes no links.
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	if ms := named(t, e, "CmdOnly"); len(ms) != 1 || ms[0].Kind != source.SteamCMD {
		t.Fatalf("after rescan: %d instances", len(ms))
	}
	if out := e.MustRun("save", "--dry-run"); strings.Contains(out, "links") {
		t.Fatalf("second save would change links:\n%s", out)
	}

	e.MustRun("disable", ref)
	out = e.MustRun("save")
	if !strings.Contains(out, "- _Archo_steamcmd_8888") {
		t.Fatalf("save does not report the removal:\n%s", out)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("link still there after disable + save")
	}
	if !source.HasAbout(cmdDir) {
		t.Fatal("the SteamCMD folder was touched")
	}
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	if m := one(t, e, "CmdOnly"); m.Status != identity.Live {
		t.Fatalf("instance status %s", m.Status)
	}
}

// RimArcho never deletes or replaces a non-link entry, even with its prefix.
func TestLinkDeploymentRefusesRealFolder(t *testing.T) {
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "8888"), testkit.Sample("tester.cmd", "CmdOnly"))
	e.WriteFile(filepath.Join(e.Game, "Mods", "_Archo_steamcmd_8888", "keep.txt"), "mine")
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "CmdOnly", source.SteamCMD))
	if out, code := e.Run("save"); code == 0 || !strings.Contains(out, "not a link") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "_Archo_steamcmd_8888", "keep.txt")); err != nil {
		t.Fatal("the real folder was touched")
	}
}

// A deployed copy follows the game's registration rules: a Mods/ mod with the same
// packageId wins, so enabling the SteamCMD copy is refused; next to an enabled workshop
// copy it can only be swapped in.
func TestLinkDeploymentSamePackage(t *testing.T) {
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "1001"), testkit.Sample("tester.a", "A"))
	e.WriteMod(filepath.Join(e.SteamCMD, "5555"), testkit.Sample("tester.b", "B"))
	scanNow(t, e)
	if out, code := e.Run("enable", refOf(t, e, "B", source.SteamCMD)); code == 0 || !strings.Contains(out, "duplicate-installed") {
		t.Fatalf("same packageId as a Mods/ folder: exit %d\n%s", code, out)
	}
	cmdA := refOf(t, e, "A", source.SteamCMD)
	if out, code := e.Run("enable", cmdA); code == 0 || !strings.Contains(out, "archo swap") {
		t.Fatalf("next to the enabled workshop copy: exit %d\n%s", code, out)
	}
	e.MustRun("swap", refOf(t, e, "A", source.Workshop), cmdA)
	e.MustRun("save")
	if cfg := e.ReadModsConfig(); !strings.Contains(cfg, "<li>tester.a</li>") || strings.Contains(cfg, "tester.a_steam") {
		t.Fatalf("the deployed copy should take the plain id:\n%s", cfg)
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := source.RealPath(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
