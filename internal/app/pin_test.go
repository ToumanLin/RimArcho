package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// pinFixture: one enabled workshop mod and one installed but not enabled.
func pinFixture(t *testing.T) (*testkit.Env, string) {
	e := testkit.New(t)
	m := testkit.Sample("tester.lock", "Lockable")
	m.Files = map[string]string{"Defs/a.xml": "<Defs><ThingDef><defName>A</defName></ThingDef></Defs>"}
	ws := e.AddWorkshop("5100", m)
	e.AddWorkshop("5200", testkit.Sample("tester.idle", "Idle"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lock")
	e.AddEnv(false)
	scanNow(t, e)
	return e, ws
}

func library(e *testkit.Env) string {
	return filepath.Join(e.Root, "LocalAppData", "RimArcho", "locked")
}

func inPlan(t *testing.T, e *testkit.Env, id string) bool {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	return st.List.Index(id) >= 0
}

// lockedAt is the instance in the library folder named folder.
func lockedAt(t *testing.T, e *testkit.Env, folder string) string {
	t.Helper()
	ix, _ := e.App.Index()
	if m := ix.ModAt(filepath.Join(library(e), folder)); m != nil {
		return m.ID
	}
	t.Fatalf("no instance at %s", folder)
	return ""
}

// archoLinks lists the _Archo_ links in Mods/ with their targets.
func archoLinks(t *testing.T, e *testkit.Env) map[string]string {
	t.Helper()
	out := map[string]string{}
	ents, _ := os.ReadDir(filepath.Join(e.Game, "Mods"))
	for _, d := range ents {
		if strings.HasPrefix(d.Name(), app.DeployPrefix) {
			out[d.Name()] = mustReal(t, filepath.Join(e.Game, "Mods", d.Name()))
		}
	}
	return out
}

// 04 §7.2: pin copies into the lock library (registered as src:locked), swaps the copy in
// for the enabled original as one undoable edit, and the game gets it only through an
// _Archo_ link on save. A dry run writes nothing; a mod that is not enabled leaves the plan.
func TestPinLibrary(t *testing.T) {
	e, _ := pinFixture(t)
	before := e.ReadModsConfig()
	up := refID(t, e, "Lockable", source.Workshop)

	out := e.MustRun("pin", refOf(t, e, "Lockable", ""), "--dry-run")
	if !strings.Contains(out, "dry run, nothing changed") || !strings.Contains(out, "would replace") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(library(e)); err == nil {
		t.Fatal("dry run created the library")
	}

	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	ix, _ := e.App.Index()
	var lib *source.Source
	for _, s := range ix.Sources {
		if s.Label == app.LockLabel {
			lib = s
		}
	}
	if lib == nil || lib.Kind != source.Extra || !strings.EqualFold(lib.Root, library(e)) {
		t.Fatalf("lock library source: %+v", lib)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "5100_local")); err == nil {
		t.Fatal("the copy must not be made in Mods/")
	}
	cp := lockedAt(t, e, "5100_local")
	if inPlan(t, e, up) || !inPlan(t, e, cp) {
		t.Fatal("the current plan must use the copy instead of the original")
	}
	if e.ReadModsConfig() != before {
		t.Fatal("pin wrote ModsConfig.xml")
	}
	got := problemCodes(t, e)
	if got["mod.pinned"] != "info" || got["mod.unloadable"] != "" {
		t.Fatalf("a RimArcho-made copy is a locked version: %v", got)
	}
	ps, _ := e.App.Problems(false, nil)
	for _, p := range ps.List {
		if p.Code == "pkg.duplicate-installed" {
			t.Fatalf("a locked copy is not a duplicate: %s", p.Claim)
		}
	}

	e.MustRun("save")
	links := archoLinks(t, e)
	if len(links) != 1 || !strings.EqualFold(links["_Archo_locked_5100_local"], mustReal(t, filepath.Join(library(e), "5100_local"))) {
		t.Fatalf("save must link exactly the plan's copy: %v", links)
	}

	e.MustRun("undo")
	if !inPlan(t, e, up) || inPlan(t, e, cp) {
		t.Fatal("undo must bring the original back in one step")
	}

	out = e.MustRun("pin", refOf(t, e, "Idle", ""))
	if !strings.Contains(out, "plan: unchanged") || inPlan(t, e, lockedAt(t, e, "5200_local")) {
		t.Fatalf("a mod that is not enabled leaves the plan alone:\n%s", out)
	}
	// A taken folder name gets a suffix instead of failing.
	if out := e.MustRun("pin", refOf(t, e, "Idle", source.Workshop)); !strings.Contains(out, "5200_local2") {
		t.Fatalf("name collision:\n%s", out)
	}
}

// 04 §7.3, §7.4: the upstream's snapshot differing from origin_snapshot raises mod.pinned to
// a warning; pin --update makes a new library folder and swaps it in; both copies coexist
// and save links only the one the plan uses.
func TestPinUpdate(t *testing.T) {
	e, ws := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	oldID := lockedAt(t, e, "5100_local")
	ix, _ := e.App.Index()
	old := ix.Ref(oldID)
	if out, code := e.Run("pin", old, "--update"); code == 0 || !strings.Contains(out, "nothing to update") {
		t.Fatalf("update with no upstream change: exit %d\n%s", code, out)
	}

	// No ACF time needed: the origin marker records what was copied.
	e.WriteFile(filepath.Join(ws, "Defs", "b.xml"), "<Defs><ThingDef><defName>B</defName></ThingDef></Defs>")
	scanNow(t, e)
	if got := problemCodes(t, e); got["mod.pinned"] != "warning" {
		t.Fatalf("upstream update: %v", got)
	}

	out := e.MustRun("pin", refOf(t, e, "Lockable", source.Workshop), "--update")
	for _, want := range []string{"new locked version copied to", "5100_local2", "+ defs/b.xml", "old copy: " + old + " stays", "replaces " + old} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	fresh := lockedAt(t, e, "5100_local2")
	if !inPlan(t, e, fresh) || inPlan(t, e, oldID) {
		t.Fatal("the new copy must replace the old one in the current plan")
	}
	if _, err := os.Stat(filepath.Join(library(e), "5100_local", "About", "About.xml")); err != nil {
		t.Fatal("the old copy must stay in the library")
	}
	e.MustRun("save")
	links := archoLinks(t, e)
	if len(links) != 1 || !strings.EqualFold(links["_Archo_locked_5100_local2"], mustReal(t, filepath.Join(library(e), "5100_local2"))) {
		t.Fatalf("only the plan's copy is linked: %v", links)
	}
	// Going back to the old copy is an ordinary swap; save relinks.
	ix, _ = e.App.Index()
	e.MustRun("swap", ix.Ref(fresh), ix.Ref(oldID))
	e.MustRun("save")
	if links := archoLinks(t, e); len(links) != 1 || links["_Archo_locked_5100_local"] == "" {
		t.Fatalf("after swapping back: %v", links)
	}
}

// 04 §7: a copy RimArcho made in Mods/ before the library existed moves into the library on
// pin --move-to-library (and on the first pin --update); nothing else leaves Mods/.
func TestPinLegacyMove(t *testing.T) {
	e, ws := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	// Make it look like an old copy: the folder sits in Mods/ (origin marker included).
	legacy := filepath.Join(e.Game, "Mods", "5100_local")
	e.Move(filepath.Join(library(e), "5100_local"), legacy)
	scanNow(t, e)
	ix, _ := e.App.Index()
	m := ix.ModAt(legacy)
	if m == nil || m.Kind != source.Local {
		t.Fatal("fixture: no legacy copy in Mods/")
	}
	if !inPlan(t, e, m.ID) {
		e.MustRun("swap", refOf(t, e, "Lockable", source.Workshop), ix.Ref(m.ID))
	}
	if got := problemCodes(t, e); got["mod.pinned"] != "info" {
		t.Fatalf("a legacy copy is a locked version too: %v", got)
	}

	out := e.MustRun("pin", ix.Ref(m.ID), "--move-to-library", "--dry-run")
	if !strings.Contains(out, "would move") || !strings.Contains(out, "renamed (same volume)") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("dry run moved the folder")
	}
	// The first pin --update moves it before copying.
	e.WriteFile(filepath.Join(ws, "Defs", "b.xml"), "<Defs><ThingDef><defName>B</defName></ThingDef></Defs>")
	scanNow(t, e)
	out = e.MustRun("pin", ix.Ref(m.ID), "--update")
	if !strings.Contains(out, "made before the lock library") || !strings.Contains(out, "5100_local2") {
		t.Fatalf("update with a legacy copy:\n%s", out)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Fatal("the legacy copy is still in Mods/")
	}
	if _, err := os.Stat(filepath.Join(library(e), "5100_local", ".rimarcho", "origin.json")); err != nil {
		t.Fatal("the legacy copy did not arrive in the library with its marker")
	}
	if !inPlan(t, e, lockedAt(t, e, "5100_local2")) {
		t.Fatal("the new copy must be in the current plan")
	}

	// Only RimArcho-made copies qualify.
	e.WriteMod(filepath.Join(e.Game, "Mods", "Mine"), testkit.Sample("tester.mine", "Mine"))
	scanNow(t, e)
	if out, code := e.Run("pin", refOf(t, e, "Mine", ""), "--move-to-library"); code == 0 || !strings.Contains(out, "only folders with .rimarcho/origin.json") {
		t.Fatalf("a plain Mods/ folder must not move: exit %d\n%s", code, out)
	}
}

// 04 §7.5: unpinning a library copy swaps the upstream back; the copy stays.
func TestUnpinLocked(t *testing.T) {
	e, _ := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	cp, up := lockedAt(t, e, "5100_local"), refID(t, e, "Lockable", source.Workshop)
	ix, _ := e.App.Index()
	out := e.MustRun("unpin", ix.Ref(cp), "--dry-run")
	if !strings.Contains(out, "dry run") || !inPlan(t, e, cp) {
		t.Fatalf("dry run:\n%s", out)
	}
	// The upstream ref works too.
	e.MustRun("unpin", ix.Ref(up))
	if !inPlan(t, e, up) || inPlan(t, e, cp) {
		t.Fatal("unpin must swap the upstream back into the current plan")
	}
	if _, err := os.Stat(filepath.Join(library(e), "5100_local")); err != nil {
		t.Fatal("the copy stays in the library")
	}
	if out := e.MustRun("unpin", ix.Ref(cp)); !strings.Contains(out, "nothing changed") {
		t.Fatalf("second unpin:\n%s", out)
	}
	if out, code := e.Run("unpin", refOf(t, e, "Idle", "")); code == 0 || !strings.Contains(out, "not pinned") {
		t.Fatalf("unpin of an unlocked mod: exit %d\n%s", code, out)
	}
}

// 08 §3: on a Mods/ volume without junctions (FAT, exFAT) save refuses with the existing
// reason and writes nothing; there is no copy fallback.
func TestPinNoJunctions(t *testing.T) {
	e, _ := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	before := e.ReadModsConfig()
	prev := app.CreateLink
	app.CreateLink = func(link, target string) error {
		return &os.PathError{Op: "junction", Path: link, Err: errors.New("the file system does not support reparse points")}
	}
	t.Cleanup(func() { app.CreateLink = prev })
	out, code := e.Run("save")
	if code == 0 || !strings.Contains(out, "must support junctions") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if e.ReadModsConfig() != before || len(archoLinks(t, e)) != 0 {
		t.Fatal("a refused save must change nothing")
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "5100_local")); err == nil {
		t.Fatal("no copy fallback into Mods/")
	}
}
