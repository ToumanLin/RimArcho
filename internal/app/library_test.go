package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// libFixture: an enabled Workshop mod with a spare copy in Mods/, an idle one, one for an old
// game version, an unsubscribed leftover and a subscription with no folder.
func libFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	m := testkit.Sample("tester.lock", "Lockable")
	ws := e.AddWorkshop("5100", m)
	e.AddWorkshop("5200", testkit.Sample("tester.idle", "Idle"))
	old := testkit.Sample("tester.old", "Oldie")
	old.Supported = []string{"1.4"}
	e.AddWorkshop("5400", old)
	e.WriteMod(filepath.Join(e.Workshop, "5500"), testkit.Sample("tester.gone", "Gone")) // not subscribed
	e.Subscribe("5300")                                                                  // no folder
	e.CopyTree(ws, filepath.Join(e.Game, "Mods", "Lockable Copy"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lock")
	e.AddEnv(false)
	scanNow(t, e)
	return e
}

func mustLib(t *testing.T, e *testkit.Env) *app.Library {
	t.Helper()
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func libLineage(t *testing.T, lib *app.Library, name string) *app.LibLineage {
	t.Helper()
	for _, l := range lib.Lineages {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no lineage %q", name)
	return nil
}

func filtered(t *testing.T, lib *app.Library, q app.LibQuery) []string {
	t.Helper()
	r, err := lib.Filter(q)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range r.Matches {
		out = append(out, m.Name)
	}
	return out
}

func TestLibraryStates(t *testing.T) {
	e := libFixture(t)
	lib := mustLib(t, e)
	lock := libLineage(t, lib, "Lockable")
	if len(lock.Instances) != 2 || lock.Instances[0].Source != app.LibWorkshop || lock.Instances[1].Source != app.LibLocal {
		t.Fatalf("Lockable: want the workshop copy, then the Mods/ copy: %+v", lock.Instances)
	}
	if !lock.Instances[1].Spare || lock.Instances[0].Spare {
		t.Error("only the Mods/ copy beside the workshop copy is spare")
	}
	if n, ok := lock.Bytes(); !ok || n <= 0 {
		t.Errorf("sizes come from the scan: %d %v", n, ok)
	}
	// "does not support the game" and "in no plan" are not states (06 §2.9)
	want := map[string]string{"Idle": "", "Oldie": "", "Gone": app.LibResidue, "Workshop item 5300": app.LibMissing}
	for name, s := range want {
		if got := libLineage(t, lib, name).State(); got != s {
			t.Errorf("%s: state %q, want %q", name, got, s)
		}
	}
	// The game registers the Mods/ copy, so the plan uses it; the workshop copy is ignored
	// by the game and in no plan.
	ws, cp := lock.Instances[0], lock.Instances[1]
	if !cp.InPlan || len(cp.UsedIn) != 1 || !cp.UsedIn[0].Current || cp.UsedIn[0].Value() != "plan:default" {
		t.Errorf("Mods/ copy used in: %+v", cp.UsedIn)
	}
	if len(ws.States) != 0 || len(ws.UsedIn) != 0 {
		t.Errorf("workshop copy states: %v", ws.States)
	}
	// A second copy in Mods/: the game registers one of the two, the other is ignored.
	e.CopyTree(filepath.Join(e.Game, "Mods", "Lockable Copy"), filepath.Join(e.Game, "Mods", "Lockable Copy 2"))
	scanNow(t, e)
	lock = libLineage(t, mustLib(t, e), "Lockable")
	ignored := 0
	for _, i := range lock.Instances {
		if i.State() == app.LibIgnored {
			ignored++
		}
	}
	if len(lock.Instances) != 3 || ignored != 1 {
		t.Errorf("two copies in Mods/: want one ignored, got %d of %d", ignored, len(lock.Instances))
	}
	if libLineage(t, lib, "Oldie").Instances[0].Versions[0] != "1.4" {
		t.Error("supported versions")
	}
}

func TestLibraryFilter(t *testing.T) {
	e := libFixture(t)
	e.MustRun("tag", "add", "pkg:tester.idle", "QoL")
	lib := mustLib(t, e)
	cases := []struct {
		q    app.LibQuery
		want []string
	}{
		{app.LibQuery{}, []string{"Gone", "Idle", "Lockable", "Oldie", "Workshop item 5300"}},
		{app.LibQuery{Search: "tester.idle"}, []string{"Idle"}},
		{app.LibQuery{Search: "qol"}, []string{"Idle"}},
		{app.LibQuery{Include: []string{"used:none"}}, []string{"Gone", "Idle", "Lockable", "Oldie", "Workshop item 5300"}},
		// one key: either value; two keys: both
		{app.LibQuery{Include: []string{"state:residue", "state:missing"}}, []string{"Gone", "Workshop item 5300"}},
		{app.LibQuery{Include: []string{"used:none", "source:workshop"}}, []string{"Gone", "Idle", "Lockable", "Oldie", "Workshop item 5300"}},
		{app.LibQuery{Include: []string{"used:none", "source:local"}}, nil},
		{app.LibQuery{Exclude: []string{"source:workshop"}}, []string{"Lockable"}},
		{app.LibQuery{Include: []string{"tag:qol"}}, []string{"Idle"}},
		{app.LibQuery{Include: []string{"used:plan:default"}}, []string{"Lockable"}},
		{app.LibQuery{Include: []string{"version:1.4"}}, []string{"Oldie"}},
	}
	for _, c := range cases {
		if got := filtered(t, lib, c.q); !slices.Equal(got, c.want) {
			t.Errorf("%+v: got %v, want %v", c.q, got, c.want)
		}
	}
	// Excluding Lockable's workshop copy leaves its Mods/ copy only.
	r, _ := lib.Filter(app.LibQuery{Exclude: []string{"source:workshop"}})
	if len(r.Matches[0].Shown) != 1 || r.Matches[0].Shown[0].Source != app.LibLocal {
		t.Errorf("shown instances: %+v", r.Matches[0].Shown)
	}
	// A key's counts ignore that key's own choices.
	r, _ = lib.Filter(app.LibQuery{Include: []string{"source:local"}})
	for _, f := range r.Facets {
		if f.Key != "source" {
			continue
		}
		for _, v := range f.Values {
			if v.Value == "workshop" && v.Count != 5 || v.Value == "local" && (v.Count != 1 || v.On != 1) {
				t.Errorf("source facet %+v", v)
			}
		}
	}
	if _, err := lib.Filter(app.LibQuery{Include: []string{"colour:red"}}); err == nil {
		t.Error("an unknown key is an error")
	}
	if got := filtered(t, lib, app.LibQuery{Sort: "size"}); got[len(got)-1] != "Workshop item 5300" {
		t.Errorf("a missing item has no size: last by size, got %v", got)
	}
}

// 06 §2.9: removing from every plan is one undo step in each plan that had it.
func TestLibraryRemoveFromPlans(t *testing.T) {
	e := libFixture(t)
	e.MustRun("plan", "new", "other", "--from", "current")
	ref := refOf(t, e, "Lockable", source.Local)
	rs, err := e.App.RemoveFromPlans([]string{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Removed != 1 || rs[1].Removed != 1 {
		t.Fatalf("removals: %+v", rs)
	}
	id := refID(t, e, "Lockable", source.Local)
	if !rs[0].Current || rs[1].Plan != "other" || inPlan(t, e, id) {
		t.Fatalf("still in the current plan: %+v", rs)
	}
	e.MustRun("undo")
	if !inPlan(t, e, id) {
		t.Fatal("undo brings it back in the current plan")
	}
	e.MustRun("plan", "use", "other")
	if inPlan(t, e, id) {
		t.Fatal("the other plan's undo is its own")
	}
	e.MustRun("undo")
	if !inPlan(t, e, id) {
		t.Fatal("the other plan's undo brings it back")
	}
}

// 06 §2.9, 01 §4: a spare copy in Mods/ moves into the lock library as the same instance.
func TestLibraryMoveToLocked(t *testing.T) {
	e := libFixture(t)
	cp := refID(t, e, "Lockable", source.Local)
	e.MustRun("plan", "new", "other", "--from", "current")
	from := filepath.Join(e.Game, "Mods", "Lockable Copy")

	r, err := e.App.MoveToLocked("#"+cp[:6], true)
	if err != nil {
		t.Fatal(err)
	}
	if !r.DryRun || !r.Origin || !slices.Equal(r.Plans, []string{"plan:default", "plan:other"}) {
		t.Fatalf("dry run: %+v", r)
	}
	if _, err := os.Stat(from); err != nil {
		t.Fatal("the dry run moved it")
	}
	if r, err = e.App.MoveToLocked("#"+cp[:6], false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); err == nil {
		t.Fatal("still in Mods/")
	}
	to := filepath.Join(library(e), "Lockable Copy")
	if r.To != to || r.After == nil || r.After.ID != cp {
		t.Fatalf("the instance must stay the same: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(to, ".rimarcho", "origin.json")); err != nil {
		t.Fatal("no origin.json written")
	}
	if !inPlan(t, e, cp) {
		t.Fatal("the plan lost it")
	}
	lib := mustLib(t, e)
	if i := lib.Instance(r.After.ID); i == nil || i.Source != app.LibLocked || i.Locked == nil || i.Created.IsZero() {
		t.Fatalf("now a locked version with its upstream: %+v", i)
	}

	// The only copy of a mod, and a folder with a link out of it, stay.
	e.WriteMod(filepath.Join(e.Game, "Mods", "Mine"), testkit.Sample("tester.mine", "Mine"))
	scanNow(t, e)
	if _, err := e.App.MoveToLocked(refOf(t, e, "Mine", ""), true); err == nil || !strings.Contains(err.Error(), "only copy") {
		t.Fatalf("only copy: %v", err)
	}
	e.CopyTree(filepath.Join(e.Workshop, "5200"), filepath.Join(e.Game, "Mods", "Idle Copy"))
	e.MkdirAll(filepath.Join(e.Root, "outside"))
	e.Junction(filepath.Join(e.Game, "Mods", "Idle Copy", "Textures", "ext"), filepath.Join(e.Root, "outside"))
	scanNow(t, e)
	if _, err := e.App.MoveToLocked(refOf(t, e, "Idle", source.Local), true); err == nil || !strings.Contains(err.Error(), "links that point outside") {
		t.Fatalf("link outside: %v", err)
	}
}

// 06 §2.9: trash names the plans that still use the folder.
func TestLibraryTrashNamesPlans(t *testing.T) {
	e := libFixture(t)
	gone := refOf(t, e, "Gone", "")
	if _, err := e.App.Trash(gone, true); err != nil {
		t.Fatalf("unused residue: %v", err)
	}
	// Another plan still lists it (enable refuses residue: write the plan's list directly).
	e.MustRun("plan", "new", "other")
	p, err := e.App.FindPlan("other")
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := e.App.Store.Workspace(p.ID)
	ws.Entries = `[{"i":"` + refID(t, e, "Gone", "") + `"}]`
	if err := e.App.Store.SetWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Trash(gone, true); err == nil || !strings.Contains(err.Error(), "plan:other") {
		t.Fatalf("want a refusal naming plan:other, got %v", err)
	}
}

// A SteamCMD download may go like a copy of the player's own: no subscription brings it
// back.
func TestTrashSteamCMDCopy(t *testing.T) {
	e := setup(t, true)
	e.WriteMod(filepath.Join(e.SteamCMD, "5001"), testkit.Sample("tester.cmd", "Via SteamCMD"))
	scanNow(t, e)
	if _, err := e.App.Trash(refOf(t, e, "Via SteamCMD", ""), true); err != nil {
		t.Fatalf("a SteamCMD copy: %v", err)
	}
}
