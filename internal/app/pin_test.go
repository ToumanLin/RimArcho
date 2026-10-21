package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func inPlan(t *testing.T, e *testkit.Env, id string) bool {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	return st.List.Index(id) >= 0
}

// 04 §7.2 step 6: pin swaps the copy in for the enabled original as one undoable edit; a
// dry run writes nothing; a mod that is not enabled leaves the plan alone.
func TestPinSwapAndUndo(t *testing.T) {
	e, _ := pinFixture(t)
	before := e.ReadModsConfig()
	up := refID(t, e, "Lockable", source.Workshop)
	dst := filepath.Join(e.Game, "Mods", "5100_local")

	out := e.MustRun("pin", refOf(t, e, "Lockable", ""), "--dry-run")
	if !strings.Contains(out, "dry run, nothing changed") || !strings.Contains(out, "would replace") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("dry run created the copy")
	}

	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	cp := refID(t, e, "Lockable", source.Local)
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

	e.MustRun("undo")
	if !inPlan(t, e, up) || inPlan(t, e, cp) {
		t.Fatal("undo must bring the original back in one step")
	}

	out = e.MustRun("pin", refOf(t, e, "Idle", ""))
	if !strings.Contains(out, "plan: unchanged") || inPlan(t, e, refID(t, e, "Idle", source.Local)) {
		t.Fatalf("a mod that is not enabled leaves the plan alone:\n%s", out)
	}
}

// 04 §7.3, §7.4: the upstream's snapshot differing from origin_snapshot raises mod.pinned to
// a warning; pin --update makes a new folder, swaps it in, and leaves the old copy.
func TestPinUpdate(t *testing.T) {
	e, ws := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	old, oldID := refOf(t, e, "Lockable", source.Local), refID(t, e, "Lockable", source.Local)
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
	for _, want := range []string{"new locked version copied to", "5100_local2", "+ defs/b.xml", "old copy: " + old + " stays"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "5100_local2", "Defs", "b.xml")); err != nil {
		t.Fatal("the new copy has the upstream's current content")
	}
	var fresh string
	for _, m := range named(t, e, "Lockable") {
		if strings.HasSuffix(m.Rel, "5100_local2") {
			fresh = m.ID
		}
	}
	if fresh == "" || fresh == oldID {
		t.Fatal("no new copy instance")
	}
	// The game registers one Mods/ folder per packageId, the first by name: while the old
	// copy is there the new one would be ignored, so the plan is left alone and says why.
	if !strings.Contains(out, "plan: unchanged: the game ignores") || !inPlan(t, e, oldID) || inPlan(t, e, fresh) {
		t.Fatalf("swap must wait for the old copy to leave Mods/:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "5100_local", "About", "About.xml")); err != nil {
		t.Fatal("the old copy must stay")
	}
	// Once the player moves the old copy away, the suggested swap works.
	e.Move(filepath.Join(e.Game, "Mods", "5100_local"), filepath.Join(e.Root, "kept-copy"))
	scanNow(t, e)
	ix, _ := e.App.Index()
	e.MustRun("swap", ix.Ref(oldID), ix.Ref(fresh))
	if !inPlan(t, e, fresh) {
		t.Fatal("the new copy must be usable after the old one is moved")
	}
	if got := problemCodes(t, e); got["mod.pinned"] != "info" {
		t.Fatalf("the new copy matches its upstream: %v", got)
	}
}

// 04 §7.5: unpinning a RimArcho-made copy swaps the upstream back; the copy stays.
func TestUnpinLocked(t *testing.T) {
	e, _ := pinFixture(t)
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	cp, up := refID(t, e, "Lockable", source.Local), refID(t, e, "Lockable", source.Workshop)
	out := e.MustRun("unpin", refOf(t, e, "Lockable", source.Local), "--dry-run")
	if !strings.Contains(out, "dry run") || !inPlan(t, e, cp) {
		t.Fatalf("dry run:\n%s", out)
	}
	// The upstream ref works too.
	e.MustRun("unpin", refOf(t, e, "Lockable", source.Workshop))
	if !inPlan(t, e, up) || inPlan(t, e, cp) {
		t.Fatal("unpin must swap the upstream back into the current plan")
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "5100_local")); err != nil {
		t.Fatal("the copy stays on disk")
	}
	if out := e.MustRun("unpin", refOf(t, e, "Lockable", source.Local)); !strings.Contains(out, "nothing changed") {
		t.Fatalf("second unpin:\n%s", out)
	}
	if out, code := e.Run("unpin", refOf(t, e, "Idle", "")); code == 0 || !strings.Contains(out, "not pinned") {
		t.Fatalf("unpin of an unlocked mod: exit %d\n%s", code, out)
	}
}
