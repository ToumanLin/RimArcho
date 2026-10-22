package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// No two copies of one packageId in Mods/ (08 §3, §5.3): the game logs "Tried loading mod
// with the same packageId multiple times" and ignores one.

// dupEnv is the player's 2026-10-21 game: a RimCrow-style local copy Mods/_2888837973_
// (hair.Kawaii) and RimCrow's companion reached through Mods/_Link_RimCrowCompanion, both in
// the plan, which is compiled into the pack 测试.
func dupEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := setup(t, true)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "_2888837973_"), testkit.Sample("hair.Kawaii", "Kawaii Hair"))
	companion := filepath.Join(e.Root, "RimCrow", "Companion")
	e.WriteMod(companion, testkit.Sample("rimcrow.companion", "RimCrow Companion"))
	e.Junction(filepath.Join(mods, "_Link_RimCrowCompanion"), companion)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "hair.kawaii", "rimcrow.companion")
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("pack", "build", "测试")
	return e
}

func packLinksIn(t *testing.T, e *testkit.Env) []string {
	t.Helper()
	ls, _ := filepath.Glob(filepath.Join(e.Game, "Mods", "!ArchoPack_*"))
	return ls
}

// The replay: launching the pack no longer links its copies next to the same mods in Mods/;
// the copies already there (the same content) stand in for the pack's.
func TestPackLaunchNoDuplicates(t *testing.T) {
	e := dupEnv(t)
	out := e.MustRun("pack", "launch", "测试")
	if !strings.Contains(out, "from Mods/: 2 mods") {
		t.Fatalf("launch:\n%s", out)
	}
	if ls := packLinksIn(t, e); len(ls) != 0 {
		t.Fatalf("pack links next to the same mods: %v", ls)
	}
	if dups := app.ModsDuplicates(e.App.ModsCopies(mustIndex(t, e))); len(dups) != 0 {
		t.Fatalf("duplicates in Mods/: %+v", dups)
	}
	cfg, _ := os.ReadFile(filepath.Join(e.Home, "packs", "测试", "userdata", "Config", "ModsConfig.xml"))
	if !strings.Contains(strings.ToLower(string(cfg)), "<li>hair.kawaii</li>") || !strings.Contains(string(cfg), "<li>rimcrow.companion</li>") {
		t.Fatalf("the pack's list:\n%s", cfg)
	}

	// The local copy changed since the build: the pack's copy is not the same mod any more,
	// and linking it would put two in Mods/: refused, naming the folder.
	e.WriteFile(filepath.Join(e.Game, "Mods", "_2888837973_", "Defs", "New.xml"), "<Defs><ThingDef><defName>N</defName></ThingDef></Defs>")
	scanNow(t, e)
	out, code := e.Run("pack", "launch", "测试")
	if code == 0 || !strings.Contains(out, "Mods/_2888837973_") || !strings.Contains(out, "other content") {
		t.Fatalf("a changed local copy: exit %d\n%s", code, out)
	}
	if ls := packLinksIn(t, e); len(ls) != 0 {
		t.Fatalf("a refused launch made links: %v", ls)
	}
}

// What the old pack launch left (local _X_ and !ArchoPack____X_): one item in 会让游戏出错
// whose one action removes only the pack's links.
func TestPackLinksDuplicateReported(t *testing.T) {
	e := dupEnv(t)
	mods := filepath.Join(e.Game, "Mods")
	copyDir := filepath.Join(e.Home, "packs", "测试", "mods")
	ents, _ := os.ReadDir(copyDir)
	if len(ents) == 0 {
		t.Fatal("the pack carries nothing")
	}
	for _, de := range ents {
		e.Junction(filepath.Join(mods, "!ArchoPack____"+de.Name()), filepath.Join(copyDir, de.Name()))
	}
	out := e.MustRun("problems", "--code", "pack")
	if !strings.Contains(out, "error pack.links-left") || !strings.Contains(out, "hair.kawaii") || !strings.Contains(out, ", Mods/_2888837973_") || !strings.Contains(out, "Mods/!ArchoPack____") {
		t.Fatalf("problems:\n%s", out)
	}
	if out := e.MustRun("problems", "--code", "pkg.duplicate-mods"); strings.Contains(out, "error pkg.duplicate-mods") {
		t.Fatalf("the pack's duplicates are reported twice:\n%s", out)
	}
	e.MustRun("pack", "unlink")
	if ls := packLinksIn(t, e); len(ls) != 0 {
		t.Fatalf("links left: %v", ls)
	}
	if _, err := os.Stat(filepath.Join(mods, "_2888837973_", "About", "About.xml")); err != nil {
		t.Fatal("unlink touched the player's folder")
	}
	if out := e.MustRun("problems", "--code", "pack"); strings.Contains(out, "pack.links-left") {
		t.Fatalf("still reported:\n%s", out)
	}
}

// _Archo_ links (08 §3): writing a plan never links a mod next to another copy of its
// packageId in Mods/; a pair that is already there is reported with a fix that does not move
// the player's folder; a pack drops a plan's link that would duplicate its own copy.
func TestDeployNoDuplicates(t *testing.T) {
	e := setup(t, true)
	cmdDir := filepath.Join(e.SteamCMD, "8888")
	e.WriteMod(cmdDir, testkit.Sample("tester.cmd", "CmdOnly"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "CmdOnly", ""))
	e.MustRun("save")
	mods := filepath.Join(e.Game, "Mods")

	// Another copy of the same package appears in Mods/ (copied by hand, say).
	e.WriteMod(filepath.Join(mods, "CmdCopy"), testkit.Sample("tester.cmd", "CmdOnly"))
	scanNow(t, e)
	out := e.MustRun("problems", "--code", "pkg.duplicate-mods")
	if !strings.Contains(out, "error pkg.duplicate-mods") || !strings.Contains(out, "Mods/_Archo_steamcmd_8888") || !strings.Contains(out, "Mods/CmdCopy") {
		t.Fatalf("problems:\n%s", out)
	}
	if !strings.Contains(out, "fix: archo swap") && !strings.Contains(out, "fix: archo disable") {
		t.Fatalf("no fix:\n%s", out)
	}
	// Writing the plan again keeps the existing link (it is replaced only when it changes),
	// but a new link next to a copy is refused: drop the link and write.
	os.Remove(filepath.Join(mods, "_Archo_steamcmd_8888"))
	scanNow(t, e)
	if out, code := e.Run("save"); code == 0 || !strings.Contains(out, "Mods/CmdCopy") {
		t.Fatalf("save next to a copy: exit %d\n%s", code, out)
	}
	if _, err := os.Lstat(filepath.Join(mods, "_Archo_steamcmd_8888")); err == nil {
		t.Fatal("the refused save made the link")
	}
}

// Two folders of the player's with one packageId: reported, and the fix says what to do.
func TestPlayerDuplicatesReported(t *testing.T) {
	e := setup(t, false)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "A1"), testkit.Sample("tester.same", "Same"))
	e.WriteMod(filepath.Join(mods, "A2"), testkit.Sample("tester.same", "Same"))
	scanNow(t, e)
	out := e.MustRun("problems", "--code", "pkg.duplicate-mods")
	if !strings.Contains(out, "Mods/A1, Mods/A2") || !strings.Contains(out, "move all but one") {
		t.Fatalf("problems:\n%s", out)
	}
}

// A pack whose copy shares a packageId with a plan's _Archo_ link: the link goes for the run.
func TestPackDropsDuplicateDeploy(t *testing.T) {
	e := setup(t, true)
	cmdDir := filepath.Join(e.SteamCMD, "8888")
	e.WriteMod(cmdDir, testkit.Sample("tester.cmd", "CmdOnly"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "CmdOnly", ""))
	e.MustRun("save")
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("pack", "build", "p")
	e.MustRun("pack", "launch", "p")
	if _, err := os.Lstat(filepath.Join(e.Game, "Mods", "_Archo_steamcmd_8888")); err == nil {
		t.Fatal("the plan's link stayed next to the pack's copy")
	}
	if dups := app.ModsDuplicates(e.App.ModsCopies(mustIndex(t, e))); len(dups) != 0 {
		t.Fatalf("duplicates: %+v", dups)
	}
	if ls := packLinksIn(t, e); len(ls) != 1 {
		t.Fatalf("pack links: %v", ls)
	}
}
